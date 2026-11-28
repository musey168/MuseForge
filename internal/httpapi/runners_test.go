package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// requestWithHeaders issues a JSON request with extra headers (runner session
// or bootstrap tokens) and returns the raw recorder.
func requestWithHeaders(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIsRunnerAgentPath(t *testing.T) {
	agentPaths := []string{
		"/runners/install.sh",
		"/runners/agentflow-runner",
		"/runners/agentflow-gate-mcp",
		"/runners/health",
		"/runners/register",
		"/runners/heartbeat",
		"/api/runners/register",
		"/api/runners/disconnect",
		"/api/runners/heartbeat",
	}
	for _, p := range agentPaths {
		if !isRunnerAgentPath(p) {
			t.Fatalf("isRunnerAgentPath(%q) = false, want true", p)
		}
	}
	managementPaths := []string{
		"/api/runners",
		"/api/runners/tasks",
		"/api/runners/tasks/rt-1/cancel",
		"/api/runners/tasks/rt-1/start",
		"/api/runners/tasks/rt-1/lease/renew",
		"/api/runs",
		"/api/runnersX",
	}
	for _, p := range managementPaths {
		if isRunnerAgentPath(p) {
			t.Fatalf("isRunnerAgentPath(%q) = true, want false", p)
		}
	}
	_ = os.Environ
}

// A machine that registers with the server's bootstrap token belongs to the
// platform's one account, so the console lists it. Registering it ownerless
// would hide it from the only person who can see anything.
func TestBootstrapRegisteredMachineBelongsToTheAccount(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPost, "/api/account", map[string]any{}, http.StatusOK)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-test")

	registered := requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{
		"name": "my machine", "os": "darwin", "arch": "arm64", "runnerVersion": "0.13.2",
	}, map[string]string{"X-Bootstrap-Token": "boot-test"})
	if registered.Code != http.StatusOK {
		t.Fatalf("register = %d %s", registered.Code, registered.Body.String())
	}

	list := requestWithHeaders(t, h, http.MethodGet, "/api/runners", nil, nil)
	var machines []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &machines); err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("the machine list = %d machines, want the registered one", len(machines))
	}
	if machines[0]["name"] != "my machine" {
		t.Fatalf("machine = %+v", machines[0])
	}
	if owner, _ := machines[0]["ownerUserId"].(float64); owner != 1 {
		t.Fatalf("the machine must belong to the account: %+v", machines[0])
	}
}
