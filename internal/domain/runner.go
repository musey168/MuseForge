package domain

import (
	"strconv"
	"strings"
	"time"
)

// Runner connection model (docs: cli-connect.md).
//
// A Runner is a long-lived daemon on a user machine that connects to the
// control plane over outbound HTTPS only. The platform never opens an
// inbound port to the machine. A RunnerRuntime is one Agent CLI (claude,
// codex, codebuddy, ...) discovered on that machine. The machine is an
// inventory: it registers, heartbeats and reports its CLIs. It never
// receives work.

const (
	RunnerStatusOnline  = "online"
	RunnerStatusOffline = "offline"
)

// RunnerVersionCLISettings adds chat account login, CLI tests, model discovery
// and updates from the machine card.
const RunnerVersionCLISettings = "0.10.2"

// SupportsCLISettings reports whether the machine offers the account login
// and model catalogue the inventory page shows.
func (r Runner) SupportsCLISettings() bool {
	return versionAtLeast(r.RunnerVersion, RunnerVersionCLISettings)
}

// Runner is one registered execution machine.
type Runner struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	OS            string          `json:"os"`
	Arch          string          `json:"arch"`
	RunnerVersion string          `json:"runnerVersion"`
	Labels        []string        `json:"labels,omitempty"`
	Status        string          `json:"status"`
	Runtimes      []RunnerRuntime `json:"runtimes,omitempty"`
	LastSeenAt    *time.Time      `json:"lastSeenAt,omitempty"`
	// OwnerUserID is the user who bound this machine via bind-cli.sh; the
	// personal 执行器 page lists machines by owner.
	OwnerUserID int64 `json:"ownerUserId,omitempty"`
	// LoginModels contains the machine owner's saved account-login catalogs by CLI.
	LoginModels      map[string][]CLIModel `json:"loginModels,omitempty"`
	SessionTokenHash string                `json:"-"`
	SessionExpiresAt *time.Time            `json:"-"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

// PublicRunner strips credential material before returning runner data to
// management callers. Session hashes never leave the store.
func PublicRunner(r Runner) Runner {
	r.SessionTokenHash = ""
	r.SessionExpiresAt = nil
	return r
}

// PublicRunners strips credential material from a list of machines.
func PublicRunners(items []Runner) []Runner {
	out := make([]Runner, len(items))
	for i, r := range items {
		out[i] = PublicRunner(r)
	}
	return out
}

// versionAtLeast compares dotted numeric versions; malformed input is older.
func versionAtLeast(have, want string) bool {
	parse := func(v string) ([]int, bool) {
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, false
			}
			out[i] = n
		}
		return out, true
	}
	h, ok := parse(have)
	if !ok {
		return false
	}
	w, _ := parse(want)
	for i := range len(w) {
		hv := 0
		if i < len(h) {
			hv = h[i]
		}
		if hv != w[i] {
			return hv > w[i]
		}
	}
	return true
}

// SupportsProvider reports whether the runner last reported a runtime for the
// provider.
func (r Runner) SupportsProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return false
	}
	for _, rt := range r.Runtimes {
		if strings.EqualFold(rt.Provider, provider) && rt.Status != RuntimeStatusDisabled {
			return true
		}
	}
	return false
}

// RuntimeAuthMode returns the configured credential source for a provider.
// CLIs with reusable account authentication default to local login. Other
// CLIs always use the responsible user's personal credential.
func (r Runner) RuntimeAuthMode(provider string) string {
	if IsAgentCLIKind(provider) && !SupportsRunnerLogin(provider) {
		return RuntimeAuthPersonal
	}
	provider = strings.TrimSpace(provider)
	for _, rt := range r.Runtimes {
		if strings.EqualFold(rt.Provider, provider) {
			if rt.AuthMode == RuntimeAuthPersonal {
				return RuntimeAuthPersonal
			}
			if rt.AuthMode == RuntimeAuthLocal {
				return RuntimeAuthLocal
			}
			break
		}
	}
	if strings.EqualFold(provider, "codebuddy") {
		return RuntimeAuthPersonal
	}
	return RuntimeAuthLocal
}

// SupportsRunnerLogin reports whether the provider can use a login state kept
// on the machine itself.
func SupportsRunnerLogin(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "claude-code", "agy":
		return true
	default:
		return false
	}
}

const (
	RuntimeStatusOnline            = "online"
	RuntimeStatusOffline           = "offline"
	RuntimeStatusDisabled          = "disabled"
	RuntimeAuthLocal               = "local"
	RuntimeAuthPersonal            = "personal"
	RuntimeCapabilityManagedUpdate = "managed_update"
	RuntimeCapabilityNativeUpdate  = "native_update"
)

// SupportsCLIUpdate reports whether the machine can update this CLI itself.
func (rt RunnerRuntime) SupportsCLIUpdate() bool {
	for _, capability := range rt.Capabilities {
		if capability == RuntimeCapabilityManagedUpdate || capability == RuntimeCapabilityNativeUpdate {
			return true
		}
	}
	return false
}

// RunnerRuntime is one discovered Agent CLI on the runner machine. Credentials
// for the CLI stay on that machine; the control plane only learns the binary
// path, version, and capabilities.
type RunnerRuntime struct {
	Provider     string   `json:"provider"`
	BinaryPath   string   `json:"binaryPath"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Status       string   `json:"status"`
	// AuthMode is controlled by the platform. "local" uses the CLI login
	// state on this machine; "personal" injects task-scoped user credentials.
	AuthMode string `json:"authMode,omitempty"`
}
