package config

import "testing"

func clearEndpointEnv(t *testing.T) {
	for _, key := range []string{"AGENTFLOW_HTTP_PORT", "AGENTFLOW_API_URL", "AGENTFLOW_PLATFORM_MCP_URL", "AGENTFLOW_PLATFORM_MCP_TOKEN", "AGENTFLOW_CONTROL_URL", "AGENTFLOW_CONTROL_TOKEN", "AGENTFLOW_WORKER_URL", "AGENTFLOW_AGENT_TOOL_GATEWAY_URL"} {
		t.Setenv(key, "")
	}
}

func TestLoopbackURL(t *testing.T) {
	cases := map[string]string{":8081": "http://127.0.0.1:8081", "0.0.0.0:8081": "http://127.0.0.1:8081", "10.0.0.2:9000": "http://10.0.0.2:9000", "": "", "bogus": ""}
	for in, want := range cases {
		if got := LoopbackURL(in); got != want {
			t.Fatalf("LoopbackURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadEndpointsAPIFollowsListenAddr(t *testing.T) {
	clearEndpointEnv(t)
	ep, err := LoadEndpoints("127.0.0.1:18090")
	if err != nil {
		t.Fatal(err)
	}
	// -addr 覆盖端口时，助手工具网关和 OTLP 采集地址要跟着走，不能再假定 8080。
	want := "http://127.0.0.1:18090"
	if ep.ListenAddr != "127.0.0.1:18090" || ep.APIBase != want || ep.PlatformBase != want || ep.AgentToolGateway != want || ep.CollectorBase != want {
		t.Fatalf("endpoints = %#v", ep)
	}
	t.Setenv("AGENTFLOW_HTTP_PORT", "9090")
	if ep, _ = LoadEndpoints(""); ep.ListenAddr != "127.0.0.1:9090" || ep.APIBase != "http://127.0.0.1:9090" {
		t.Fatalf("port from env: %#v", ep)
	}
	t.Setenv("AGENTFLOW_HTTP_PORT", "nope")
	if _, err := LoadEndpoints(""); err == nil {
		t.Fatal("expected invalid AGENTFLOW_HTTP_PORT error")
	}
}

func TestLoadEndpointsAPIURLFallback(t *testing.T) {
	clearEndpointEnv(t)
	t.Setenv("AGENTFLOW_API_URL", "http://api.internal:8080/")
	ep, _ := LoadEndpoints("")
	if ep.PlatformBase != "http://api.internal:8080" || ep.APIBase != "http://api.internal:8080" {
		t.Fatalf("API URL fallback = %#v", ep)
	}
}

func TestCurrentDefaultsAndOverride(t *testing.T) {
	clearEndpointEnv(t)
	currentMu.Lock()
	saved := current
	current = nil
	currentMu.Unlock()
	t.Cleanup(func() { currentMu.Lock(); current = saved; currentMu.Unlock() })
	if got := Current().APIBase; got != "http://127.0.0.1:8080" {
		t.Fatalf("default APIBase = %q", got)
	}
	SetCurrent(Endpoints{APIBase: "http://x"})
	if got := Current().APIBase; got != "http://x" {
		t.Fatalf("published APIBase = %q", got)
	}
}

func TestPrivateAgentToolGatewayUnlessOverridden(t *testing.T) {
	clearEndpointEnv(t)
	ep, err := LoadEndpoints("127.0.0.1:18090")
	if err != nil {
		t.Fatal(err)
	}
	// Tool sessions live in process memory: after a handover the CLI must keep
	// calling the process that started it, not the shared public port.
	private := ep.WithPrivateAgentToolGateway("http://127.0.0.1:41234/")
	if private.AgentToolGateway != "http://127.0.0.1:41234" || private.APIBase != "http://127.0.0.1:18090" {
		t.Fatalf("private gateway = %#v", private)
	}
	t.Setenv("AGENTFLOW_AGENT_TOOL_GATEWAY_URL", "http://gateway.internal:9000")
	ep, _ = LoadEndpoints("127.0.0.1:18090")
	if got := ep.WithPrivateAgentToolGateway("http://127.0.0.1:41234").AgentToolGateway; got != "http://gateway.internal:9000" {
		t.Fatalf("explicit override lost: %s", got)
	}
}

// TestRequireLoopbackListenAddr pins the exposure rule: the platform serves an
// unauthenticated API, so it binds to the loopback interface and refuses
// anything else.
func TestRequireLoopbackListenAddr(t *testing.T) {
	t.Setenv("AGENTFLOW_CONTAINER_BIND", "")
	for _, tc := range []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:8080", false},
		{"127.0.0.1:0", false},
		{"localhost:8080", false},
		{"[::1]:8080", false},
		{"0.0.0.0:8080", true},
		{"[::]:8080", true},
		{":8080", true},
		{"192.168.1.10:8080", true},
		{"example.com:8080", true},
	} {
		err := RequireLoopbackListenAddr(tc.addr)
		if (err != nil) != tc.wantErr {
			t.Errorf("RequireLoopbackListenAddr(%q) error = %v, want error %v", tc.addr, err, tc.wantErr)
		}
	}
}

// TestLoadEndpointsBindsLoopbackOnly asserts the default listen address is the
// loopback interface and that a public address is refused.
func TestLoadEndpointsBindsLoopbackOnly(t *testing.T) {
	clearEndpointEnv(t)
	t.Setenv("AGENTFLOW_CONTAINER_BIND", "")
	t.Setenv("AGENTFLOW_HTTP_PORT", "18099")
	ep, err := LoadEndpoints("")
	if err != nil {
		t.Fatal(err)
	}
	if ep.ListenAddr != "127.0.0.1:18099" {
		t.Fatalf("default listen address = %q, want the loopback interface", ep.ListenAddr)
	}
	if _, err := LoadEndpoints("0.0.0.0:18099"); err == nil {
		t.Fatal("a public listen address must be refused")
	}
}

// TestLoadEndpointsAllowsTheContainerOptIn: a container deployment may bind the
// wildcard inside its own network namespace, where the published port bounds what
// the host exposes.
func TestLoadEndpointsAllowsTheContainerOptIn(t *testing.T) {
	clearEndpointEnv(t)
	t.Setenv("AGENTFLOW_CONTAINER_BIND", "1")
	t.Setenv("AGENTFLOW_HTTP_PORT", "18099")
	if _, err := LoadEndpoints("0.0.0.0:18099"); err != nil {
		t.Fatalf("the container opt-in must be honoured: %v", err)
	}
}
