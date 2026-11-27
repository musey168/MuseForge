package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentflow/internal/domain"
)

// runnerVersion is reported at registration and at every heartbeat. It is the
// build that understands the current control protocol.
const runnerVersion = "0.13.2"

// Daemon is the long-running runner process on a user machine. It is a
// registration client: it connects to the control plane, registers the
// machine, heartbeats the CLIs it found and answers the two control
// operations the platform still has (CLI maintenance and the CLI model
// probe). It never receives work.
type Daemon struct {
	cfg    Config
	client *Client

	mu         sync.Mutex
	state      daemonState
	runtimes   []domain.RunnerRuntime
	registerMu sync.Mutex
	wsOnline   atomic.Bool

	updateMu         sync.RWMutex
	updateReady      atomic.Bool
	updateDownloadMu sync.Mutex

	heartbeatWake chan struct{}
	maintenance   runnerMaintenanceState

	restartOnce sync.Once
	restart     chan struct{}
}

type Config struct {
	ServerURL         string
	BootstrapToken    string
	StatePath         string
	Name              string
	Labels            []string
	HeartbeatInterval time.Duration
	// DisableAutoUpdate keeps the daemon from fetching a newer runner build
	// on its own. A maintenance request can still update it explicitly.
	DisableAutoUpdate bool
	// ExtraBinaries maps a provider kind to a non-default binary name.
	ExtraBinaries map[string]string
	// ManagedCLIRoot explicitly opts platform runners into managed CLI coordination.
	// Empty leaves ordinary user runners unchanged; it is never inferred from PATH.
	ManagedCLIRoot string
}

type daemonState struct {
	RunnerID     string    `json:"runnerId"`
	SessionToken string    `json:"sessionToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agentflow-runner", "state.json"), nil
}

func New(cfg Config) (*Daemon, error) {
	cfg.ManagedCLIRoot = strings.TrimSpace(cfg.ManagedCLIRoot)
	cfg.ServerURL = trimTrailingSlash(cfg.ServerURL)
	if cfg.ServerURL == "" {
		return nil, errors.New("server URL is required")
	}
	if cfg.BootstrapToken == "" {
		return nil, errors.New("bootstrap token is required")
	}
	if cfg.StatePath == "" {
		path, err := DefaultStatePath()
		if err != nil {
			return nil, err
		}
		cfg.StatePath = path
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.Name == "" {
		host, _ := os.Hostname()
		cfg.Name = host
	}
	return &Daemon{
		cfg:           cfg,
		client:        NewClient(cfg.ServerURL),
		heartbeatWake: make(chan struct{}, 1),
		restart:       make(chan struct{}),
	}, nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// Run blocks until the context is canceled or the runner has installed a
// newer build of itself. It registers the machine, keeps the heartbeat alive
// and rescans the installed CLIs.
func (d *Daemon) Run(ctx context.Context) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	if err := d.ensureRegistered(ctx); err != nil {
		return err
	}
	d.refreshRuntimes(ctx)
	go d.rescanLoop(ctx, 5*time.Minute)
	heartbeatDone := make(chan struct{})
	go func() { defer close(heartbeatDone); d.heartbeatLoop(ctx) }()
	controlDone := make(chan struct{})
	go func() { defer close(controlDone); d.controlLoop(ctx) }()
	select {
	case <-ctx.Done():
	case <-d.restart:
	}
	stop()
	<-controlDone
	d.maintenance.jobs.Wait()
	<-heartbeatDone
	d.disconnect(context.Background())
	if d.updateReady.Load() {
		return ErrUpdateReady
	}
	return nil
}

// requestRestart asks the daemon to exit so the entry point can exec the
// freshly installed runner build.
func (d *Daemon) requestRestart() {
	d.restartOnce.Do(func() { close(d.restart) })
}

// disconnect updates presence immediately on graceful shutdown but retains
// identity/session/default selection so launchd or systemd restarts do not
// require a fresh one-time bind code.
func (d *Daemon) disconnect(ctx context.Context) {
	d.mu.Lock()
	hasSession := d.state.SessionToken != ""
	d.mu.Unlock()
	if !hasSession {
		return
	}
	deregCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := d.client.Disconnect(deregCtx); err != nil {
		log.Printf("disconnect (server will time out the heartbeat instead): %v", err)
	}
}

// ---------- state persistence ----------

func (d *Daemon) loadState() (daemonState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state.RunnerID != "" {
		return d.state, nil
	}
	raw, err := os.ReadFile(d.cfg.StatePath)
	if err != nil {
		return daemonState{}, err
	}
	var st daemonState
	if err := json.Unmarshal(raw, &st); err != nil {
		return daemonState{}, err
	}
	d.state = st
	return st, nil
}

func (d *Daemon) saveState(st daemonState) error {
	dir := filepath.Dir(d.cfg.StatePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), d.cfg.StatePath); err != nil {
		return err
	}
	d.mu.Lock()
	d.state = st
	d.mu.Unlock()
	return nil
}

// Persist a renewed session well before its old deadline, but avoid rewriting
// state.json on every 30-second heartbeat. Registration uses the same lock so
// a renewal cannot overwrite a newly issued token.
func (d *Daemon) refreshSessionExpiry(expires time.Time) {
	if expires.IsZero() {
		return // older servers do not include the renewal field
	}
	d.registerMu.Lock()
	defer d.registerMu.Unlock()
	d.mu.Lock()
	st := d.state
	d.mu.Unlock()
	if st.SessionToken == "" || !st.ExpiresAt.Before(time.Now().Add(7*24*time.Hour)) {
		return
	}
	st.ExpiresAt = expires
	if err := d.saveState(st); err != nil {
		log.Printf("warning: persist renewed runner session: %v", err)
	}
}

// ---------- registration ----------

func (d *Daemon) ensureRegistered(ctx context.Context) error {
	d.registerMu.Lock()
	defer d.registerMu.Unlock()
	if d.clientSession() != "" {
		return nil
	}
	st, _ := d.loadState()
	if st.SessionToken != "" && (st.ExpiresAt.IsZero() || st.ExpiresAt.After(time.Now().Add(5*time.Minute))) {
		d.client.SetSession(st.SessionToken)
		return nil
	}
	return d.registerLocked(ctx, st.RunnerID)
}

func (d *Daemon) registerLocked(ctx context.Context, runnerID string) error {
	d.refreshRuntimes(ctx)
	name := d.cfg.Name
	if name == "" {
		name = "runner"
	}
	resp, err := d.client.Register(ctx, d.cfg.BootstrapToken, RegisterRequest{
		RunnerID: runnerID,
		Name:     name,
		OS:       goruntime.GOOS,
		Arch:     goruntime.GOARCH,
		Version:  runnerVersion,
		Labels:   d.cfg.Labels,
		Runtimes: d.currentRuntimes(),
	})
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	d.client.SetSession(resp.SessionToken)
	if err := d.saveState(daemonState{RunnerID: resp.RunnerID, SessionToken: resp.SessionToken, ExpiresAt: resp.ExpiresAt}); err != nil {
		log.Printf("warning: persist runner state: %v", err)
	}
	log.Printf("registered as %s (session expires %s)", resp.RunnerID, resp.ExpiresAt.Format(time.RFC3339))
	return nil
}

func (d *Daemon) clientSession() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state.SessionToken
}

func (d *Daemon) runnerID() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state.RunnerID
}

// reregisterWithLock is the recovery path when the server rejects a session.
func (d *Daemon) reregister(ctx context.Context) error {
	d.registerMu.Lock()
	defer d.registerMu.Unlock()
	d.mu.Lock()
	runnerID := d.state.RunnerID
	d.mu.Unlock()
	return d.registerLocked(ctx, runnerID)
}

// ---------- CLI discovery ----------

func (d *Daemon) refreshRuntimes(ctx context.Context) {
	var runtimes []domain.RunnerRuntime
	if d.cfg.ManagedCLIRoot == "" {
		runtimes = DetectRuntimes(ctx, d.cfg.ExtraBinaries)
	} else {
		var err error
		runtimes, err = d.detectManagedRuntimes(ctx)
		if err != nil {
			log.Printf("managed CLI discovery: %v", err)
			return
		}
	}
	d.mu.Lock()
	d.runtimes = runtimes
	d.mu.Unlock()
}

func (d *Daemon) currentRuntimes() []domain.RunnerRuntime {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]domain.RunnerRuntime(nil), d.runtimes...)
}

// rescanLoop refreshes the CLI inventory and, unless auto-update is off,
// keeps the runner build itself current.
func (d *Daemon) rescanLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.refreshRuntimes(ctx)
			d.checkRunnerUpdate(ctx)
		}
	}
}

// ---------- heartbeat ----------

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.HeartbeatInterval)
	defer ticker.Stop()
	// A restart reuses the persisted session without registering again. Publish
	// presence immediately so the machine does not remain offline for a full
	// heartbeat interval.
	for ctx.Err() == nil {
		req := HeartbeatRequest{
			Status:   "idle",
			Version:  runnerVersion,
			Runtimes: d.currentRuntimes(),
		}
		hbCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		response, err := d.client.Heartbeat(hbCtx, req)
		cancel()
		if err == ErrUnauthorized {
			if regErr := d.reregister(ctx); regErr != nil {
				log.Printf("re-register failed: %v", regErr)
			}
		} else if err != nil {
			log.Printf("heartbeat: %v", err)
		} else if response != nil {
			d.refreshSessionExpiry(response.ExpiresAt)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-d.heartbeatWake:
		}
	}
}

// Version returns the runner build version reported at registration.
func Version() string { return runnerVersion }
