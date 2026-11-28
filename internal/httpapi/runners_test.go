package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
		"/runners/tasks/claim",
		"/runners/tasks/rt-1/events",
		"/api/runners/register",
		"/api/runners/disconnect",
		"/api/runners/heartbeat",
		"/api/runners/tasks/claim",
		"/api/runners/tasks/rt-1",
		"/api/runners/tasks/rt-1/start",
		"/api/runners/tasks/rt-1/lease/renew",
		"/api/runners/tasks/rt-1/events",
		"/api/runners/tasks/rt-1/complete",
		"/api/runners/tasks/rt-1/control",
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
