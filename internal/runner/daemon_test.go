package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agentflow/internal/domain"
)

// fakePlatform is a stand-in control plane that answers the four endpoints a
// registration client uses: register, heartbeat, disconnect and the control
// socket.
type fakePlatform struct {
	server *httptest.Server

	mu          sync.Mutex
	register    map[string]any
	heartbeats  []map[string]any
	disconnects int
	paths       []string
}

func newFakePlatform(t *testing.T) *fakePlatform {
	t.Helper()
	p := &fakePlatform{register: map[string]any{}}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.Path)
		p.mu.Unlock()
		switch r.URL.Path {
		case "/runners/register":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p.mu.Lock()
			p.register = body
			p.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runnerId":     "runner-1",
				"sessionToken": "session-1",
				"expiresAt":    time.Now().Add(24 * time.Hour).UTC(),
			})
		case "/runners/heartbeat":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p.mu.Lock()
			p.heartbeats = append(p.heartbeats, body)
			p.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"expiresAt": time.Now().Add(24 * time.Hour).UTC()})
		case "/runners/disconnect":
			p.mu.Lock()
			p.disconnects++
			p.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakePlatform) seenPaths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.paths...)
}

// The daemon is a registration client: it registers the machine, heartbeats
// the CLIs it found and disconnects on shutdown. It never claims a task.
func TestDaemonRegistersHeartbeatsAndDisconnects(t *testing.T) {
	platform := newFakePlatform(t)
	d, err := New(Config{
		ServerURL:         platform.server.URL,
		BootstrapToken:    "boot",
		StatePath:         filepath.Join(t.TempDir(), "state.json"),
		Name:              "machine",
		Labels:            []string{"gpu"},
		HeartbeatInterval: 20 * time.Millisecond,
		DisableAutoUpdate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		platform.mu.Lock()
		beats := len(platform.heartbeats)
		platform.mu.Unlock()
		if beats > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}

	platform.mu.Lock()
	register := platform.register
	heartbeats := append([]map[string]any(nil), platform.heartbeats...)
	disconnects := platform.disconnects
	paths := append([]string(nil), platform.paths...)
	platform.mu.Unlock()

	if register["name"] != "machine" || register["version"] != runnerVersion {
		t.Fatalf("register = %+v", register)
	}
	if register["os"] == "" || register["arch"] == "" {
		t.Fatalf("register did not report the machine: %+v", register)
	}
	labels, _ := register["labels"].([]any)
	if len(labels) != 1 || labels[0] != "gpu" {
		t.Fatalf("register labels = %+v", register["labels"])
	}
	if len(heartbeats) == 0 {
		t.Fatal("no heartbeat was sent")
	}
	if heartbeats[0]["status"] != "idle" || heartbeats[0]["version"] != runnerVersion {
		t.Fatalf("heartbeat = %+v", heartbeats[0])
	}
	if disconnects != 1 {
		t.Fatalf("disconnects = %d", disconnects)
	}
	for _, path := range paths {
		if strings.Contains(path, "/runners/tasks") {
			t.Fatalf("the runner called the task queue: %v", paths)
		}
	}
}

// A persisted session is reused across restarts: the machine keeps its
// identity and does not register again.
func TestDaemonReusesPersistedSession(t *testing.T) {
	platform := newFakePlatform(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(statePath, []byte(`{"runnerId":"runner-9","sessionToken":"session-9","expiresAt":"`+
		time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{ServerURL: platform.server.URL, BootstrapToken: "boot", StatePath: statePath, DisableAutoUpdate: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ensureRegistered(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.runnerID() != "runner-9" || d.clientSession() != "session-9" {
		t.Fatalf("state = %+v", d.state)
	}
	for _, path := range platform.seenPaths() {
		if path == "/runners/register" {
			t.Fatalf("a persisted session registered again: %v", platform.seenPaths())
		}
	}
}

// A rejected session makes the runner register again instead of going dark.
func TestDaemonReregistersAfterRejectedSession(t *testing.T) {
	var mu sync.Mutex
	registrations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runners/register":
			mu.Lock()
			registrations++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runnerId":     "runner-new",
				"sessionToken": "token-new",
				"expiresAt":    time.Now().Add(24 * time.Hour).UTC(),
			})
		case "/runners/heartbeat":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	d, err := New(Config{ServerURL: server.URL, BootstrapToken: "boot", StatePath: filepath.Join(t.TempDir(), "state.json"), HeartbeatInterval: 20 * time.Millisecond, DisableAutoUpdate: true})
	if err != nil {
		t.Fatal(err)
	}
	d.state.RunnerID = "runner-old"
	if err := d.ensureRegistered(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); d.heartbeatLoop(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := registrations
		mu.Unlock()
		if count > 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	count := registrations
	mu.Unlock()
	if count < 2 {
		t.Fatalf("registrations = %d, want a re-registration after the rejected session", count)
	}
	if d.runnerID() != "runner-new" || d.clientSession() != "token-new" {
		t.Fatalf("state = %+v", d.state)
	}
	cancel()
	<-done // the loop's last state write must land before the temp dir is removed
}

// The session file survives a renewal, so a restart keeps the same identity.
func TestDaemonPersistsRenewedSession(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	d, err := New(Config{ServerURL: "https://runner.example", BootstrapToken: "boot", StatePath: statePath})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.saveState(daemonState{RunnerID: "runner-1", SessionToken: "session-1", ExpiresAt: time.Now().Add(6 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	renewed := time.Now().Add(30 * 24 * time.Hour)
	d.refreshSessionExpiry(renewed)
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved daemonState
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RunnerID != "runner-1" || saved.SessionToken != "session-1" || !saved.ExpiresAt.Equal(renewed) {
		t.Fatalf("saved state = %+v", saved)
	}
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("empty config accepted")
	}
	if _, err := New(Config{ServerURL: "http://x"}); err == nil {
		t.Fatal("missing bootstrap token accepted")
	}
	d, err := New(Config{ServerURL: "http://x///", BootstrapToken: "b", StatePath: filepath.Join(t.TempDir(), "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	if d.cfg.ServerURL != "http://x" {
		t.Fatalf("server URL = %q", d.cfg.ServerURL)
	}
	if Version() == "" {
		t.Fatal("the runner reports no version")
	}
}

// The runner reports the CLIs it found, and only those.
func TestDaemonReportsDetectedCLIs(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "opencode")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf 'opencode 1.0\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	d, err := New(Config{ServerURL: "http://x", BootstrapToken: "b", StatePath: filepath.Join(t.TempDir(), "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	d.refreshRuntimes(context.Background())
	var found bool
	for _, rt := range d.currentRuntimes() {
		if rt.Provider == "opencode" && rt.BinaryPath == tool && rt.Status == domain.RuntimeStatusOnline {
			found = true
		}
	}
	if !found {
		t.Fatalf("detected runtimes = %+v", d.currentRuntimes())
	}
}

// The machine daemon keeps one small state file and nothing else. A runner
// that copied a helper distribution per start filled 37 GB on a real machine;
// this pins the footprint so it cannot come back.
func TestDaemonLeavesNoPerSessionCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	statePath, err := DefaultStatePath()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{
		ServerURL:      "http://127.0.0.1:1", // nothing listens: registration fails at once
		BootstrapToken: "boot",
		StatePath:      statePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = d.Run(ctx) // registration fails; the daemon stops

	var entries []string
	var total int64
	err = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == home {
			return nil
		}
		rel, _ := filepath.Rel(home, path)
		entries = append(entries, rel)
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry != ".agentflow-runner" && !strings.HasPrefix(entry, ".agentflow-runner"+string(filepath.Separator)) {
			t.Fatalf("the daemon wrote outside its state directory: %s (everything: %v)", entry, entries)
		}
	}
	// A copied binary bundle is hundreds of megabytes; a state file is bytes.
	if total > 1<<20 {
		t.Fatalf("the daemon wrote %d bytes of state, want a small file: %v", total, entries)
	}
}
