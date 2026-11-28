// Package config resolves process-wide settings once at startup so the rest
// of the code never re-derives them from the environment.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Endpoints is every address this process listens on or hands to others.
// Keeping them together makes the api/worker/all differences explicit: a
// worker serves no API plane, so it only knows platform addresses that the
// deployment configures.
type Endpoints struct {
	Role string
	// ListenAddr is this process's own HTTP listener.
	ListenAddr string
	// APIBase reaches the API plane from the same host (loopback by default).
	APIBase string
	// PlatformBase reaches the API plane from agent sandboxes. Empty when the
	// deployment did not configure one for a worker; see AGENTFLOW_PLATFORM_MCP_URL.
	PlatformBase     string
	PlatformMCPToken string
	// ControlURL is where the API plane reaches this worker to interrupt runs.
	ControlURL        string
	ControlToken      string
	WorkerFallbackURL string
	// AgentToolGateway is the base the assistant's host-side CLI uses for
	// native agent tools.
	AgentToolGateway string
	// CollectorBase receives OTLP telemetry from agent CLIs.
	CollectorBase string
}

const (
	defaultAPIPort    = "8080"
	defaultWorkerAddr = ":8081"
)

// LoadEndpoints resolves Endpoints for role from the -addr flag and env.
func LoadEndpoints(role, addrFlag string) (Endpoints, error) {
	ep := Endpoints{
		Role:              role,
		PlatformMCPToken:  env("AGENTFLOW_PLATFORM_MCP_TOKEN"),
		ControlToken:      env("AGENTFLOW_CONTROL_TOKEN"),
		WorkerFallbackURL: envURL("AGENTFLOW_WORKER_URL"),
	}
	httpPort, err := httpPortFromEnv()
	if err != nil {
		return Endpoints{}, err
	}
	configuredAPI := envURL("AGENTFLOW_API_URL")
	if role == "worker" {
		ep.ListenAddr = firstNonEmpty(addrFlag, defaultWorkerAddr)
		ep.ControlURL = firstNonEmpty(envURL("AGENTFLOW_CONTROL_URL"), LoopbackURL(ep.ListenAddr))
		ep.APIBase = firstNonEmpty(configuredAPI, "http://127.0.0.1:"+httpPort)
		ep.PlatformBase = firstNonEmpty(envURL("AGENTFLOW_PLATFORM_MCP_URL"), configuredAPI)
	} else {
		// The personal build serves an unauthenticated API, so it listens on
		// the loopback interface only.
		ep.ListenAddr = firstNonEmpty(addrFlag, "127.0.0.1:"+httpPort)
		if err := RequireLoopbackListenAddr(ep.ListenAddr); err != nil {
			return Endpoints{}, err
		}
		ep.APIBase = firstNonEmpty(configuredAPI, LoopbackURL(ep.ListenAddr))
		ep.PlatformBase = firstNonEmpty(envURL("AGENTFLOW_PLATFORM_MCP_URL"), ep.APIBase)
	}
	ep.AgentToolGateway = firstNonEmpty(envURL("AGENTFLOW_AGENT_TOOL_GATEWAY_URL"), ep.APIBase)
	ep.CollectorBase = firstNonEmpty(ep.PlatformBase, ep.APIBase)
	return ep, nil
}

// RequireLoopbackListenAddr refuses a listen address that is not the loopback
// interface. The personal build serves an unauthenticated API: a public
// interface would expose the requirements, the credentials and the cost data to
// the network, so the refusal is the point rather than a warning.
//
// An address with no host ("8080", ":8080") means the wildcard, which is not
// loopback and is refused.
//
// A container deployment opts in with AGENTFLOW_ALLOW_PUBLIC_BIND=1: inside the
// container's own network namespace the wildcard reaches nothing but the
// container, and the published port bounds what the host exposes. A host
// deployment must not set it.
func RequireLoopbackListenAddr(addr string) error {
	if env("AGENTFLOW_ALLOW_PUBLIC_BIND") == "1" {
		return nil
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "127.0.0.1", "::1", "localhost":
		return nil
	}
	return fmt.Errorf("refusing to listen on %q: MuseForge serves an unauthenticated API and binds to the loopback interface only (use 127.0.0.1)", addr)
}

// httpPortFromEnv returns AGENTFLOW_HTTP_PORT, defaulting to defaultAPIPort,
// and rejects values that are not a TCP port number.
func httpPortFromEnv() (string, error) {
	port := firstNonEmpty(env("AGENTFLOW_HTTP_PORT"), defaultAPIPort)
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid AGENTFLOW_HTTP_PORT %q", port)
	}
	return port, nil
}

// WithPrivateAgentToolGateway points native agent-tool callbacks at base, the
// process's own loopback listener: tool sessions live in process memory, so
// after a handover a CLI must keep calling the process that started it.
// AGENTFLOW_AGENT_TOOL_GATEWAY_URL still takes precedence.
func (ep Endpoints) WithPrivateAgentToolGateway(base string) Endpoints {
	if env("AGENTFLOW_AGENT_TOOL_GATEWAY_URL") == "" && trimURL(base) != "" {
		ep.AgentToolGateway = trimURL(base)
	}
	return ep
}

// LoopbackURL turns a listen address into a same-host http URL, mapping
// wildcard hosts to 127.0.0.1. It returns "" for unparseable addresses.
func LoopbackURL(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "http://127.0.0.1" + addr
		}
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

var (
	currentMu sync.RWMutex
	current   *Endpoints
)

// SetCurrent publishes the endpoints main resolved for package-level helpers
// that cannot take them as a parameter.
func SetCurrent(ep Endpoints) {
	currentMu.Lock()
	defer currentMu.Unlock()
	current = &ep
}

// Current returns the published endpoints, or the "all"-role defaults from the
// environment when main never published any (tests, helper binaries).
func Current() Endpoints {
	currentMu.RLock()
	ep := current
	currentMu.RUnlock()
	if ep != nil {
		return *ep
	}
	fallback, err := LoadEndpoints("all", "")
	if err != nil {
		base := "http://127.0.0.1:" + defaultAPIPort
		return Endpoints{Role: "all", ListenAddr: ":" + defaultAPIPort, APIBase: base, PlatformBase: base, AgentToolGateway: base, CollectorBase: base}
	}
	return fallback
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func trimURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }

// envURL reads key as a base URL: trimmed, without trailing slashes.
func envURL(key string) string { return trimURL(env(key)) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
