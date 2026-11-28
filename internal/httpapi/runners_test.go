package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentflow/internal/domain"
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

func newRunnerTestServer(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{ID: 42, LoginName: "alice", ChineseName: "爱丽丝"}); err != nil {
		t.Fatal(err)
	}
	return New(s, tracker.NewService(), "", "")
}

func registerTestRunner(t *testing.T, h http.Handler) (runnerID, session string) {
	t.Helper()
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-secret")
	body := map[string]any{
		"name":           "test-mac",
		"os":             "darwin",
		"arch":           "arm64",
		"version":        "0.1.0",
		"maxConcurrency": 1,
		"runtimes": []domain.RunnerRuntime{
			{Provider: "mock", BinaryPath: "(builtin)", Status: "online"},
		},
	}
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", body, map[string]string{"X-Bootstrap-Token": "boot-secret"})
	if rec.Code != http.StatusOK {
		t.Fatalf("register: status %d body %s", rec.Code, rec.Body.String())
	}
	var out struct {
		RunnerID     string `json:"runnerId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.RunnerID == "" || out.SessionToken == "" {
		t.Fatalf("register response missing fields: %s", rec.Body.String())
	}
	return out.RunnerID, out.SessionToken
}

func TestRunnerBindCodeRegistersUnderIssuingUser(t *testing.T) {
	h := newRunnerTestServer(t)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "")

	// The bind code requires an interactive login; use the local acceptance
	// login with a freshly created user. Local login is loopback-only.
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", "alice")
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/local", bytes.NewReader([]byte("{}")))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://example.com")
	loginReq.RemoteAddr = "127.0.0.1:60000"
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("local login: %d %s", loginRec.Code, loginRec.Body.String())
	}
	rec := loginRec
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("login cookie missing")
	}

	// Issue a one-time bind code.
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/bind-codes", map[string]any{}, map[string]string{"Cookie": authCookieName + "=" + cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("issue bind code: %d %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil || issued.Code == "" {
		t.Fatalf("bind code response: %s", rec.Body.String())
	}

	// Register the machine with the bind code; it lands under the user.
	body := map[string]any{"name": "bind-code-mac", "os": "darwin", "arch": "arm64", "maxConcurrency": 1}
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", body, map[string]string{"X-Bootstrap-Token": issued.Code})
	if rec.Code != http.StatusOK {
		t.Fatalf("register with bind code: %d %s", rec.Code, rec.Body.String())
	}
	runnersRec := requestWithHeaders(t, h, http.MethodGet, "/api/runners?mine=1", nil, map[string]string{"Cookie": authCookieName + "=" + cookie})
	if runnersRec.Code != http.StatusOK {
		t.Fatalf("mine=1: %d %s", runnersRec.Code, runnersRec.Body.String())
	}
	var runners []domain.Runner
	if err := json.Unmarshal(runnersRec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].Name != "bind-code-mac" {
		t.Fatalf("mine=1 should list the bound machine: %+v", runners)
	}

	// The code is strictly one-time: a replay is rejected.
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", body, map[string]string{"X-Bootstrap-Token": issued.Code})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed bind code register status = %d, want 401", rec.Code)
	}
}

func TestRunnerDeregisterRemovesMachine(t *testing.T) {
	h := newRunnerTestServer(t)

	// Log in as alice, issue a bind code, and register her machine with it.
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", "alice")
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/local", bytes.NewReader([]byte("{}")))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://example.com")
	loginReq.RemoteAddr = "127.0.0.1:60000"
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("local login: %d %s", loginRec.Code, loginRec.Body.String())
	}
	var cookie string
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == authCookieName {
			cookie = c.Value
		}
	}
	userAuth := map[string]string{"Cookie": authCookieName + "=" + cookie}
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/bind-codes", map[string]any{}, userAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("issue bind code: %d %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil || issued.Code == "" {
		t.Fatalf("bind code response: %s", rec.Body.String())
	}
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{
		"name": "dereg-mac", "os": "darwin", "arch": "arm64", "version": domain.RunnerVersionDefaultSelection, "maxConcurrency": 1,
	}, map[string]string{"X-Bootstrap-Token": issued.Code})
	if rec.Code != http.StatusOK {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	var registered struct {
		RunnerID     string `json:"runnerId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"Authorization": "Bearer " + registered.SessionToken}

	// Graceful deregister removes the machine.
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/deregister", map[string]any{}, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("deregister: %d %s", rec.Code, rec.Body.String())
	}
	listRec := requestWithHeaders(t, h, http.MethodGet, "/api/runners", nil, nil)
	var runners []domain.Runner
	if err := json.Unmarshal(listRec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 0 {
		t.Fatalf("machine still listed after deregister: %+v", runners)
	}

	// The stale session is rejected (the record is gone).
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/heartbeat", map[string]any{}, auth)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-deregister heartbeat status = %d, want 401", rec.Code)
	}
}

func TestRunnerDisconnectKeepsRegistrationAndSession(t *testing.T) {
	h := newRunnerTestServer(t)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-secret")
	runnerID, session := registerTestRunner(t, h)
	auth := map[string]string{"Authorization": "Bearer " + session}

	// A normal daemon stop marks presence offline without destroying the
	// registration. The same session can heartbeat after a service restart.
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/disconnect", map[string]any{}, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", rec.Code, rec.Body.String())
	}
	listRec := requestWithHeaders(t, h, http.MethodGet, "/api/runners", nil, nil)
	var runners []domain.Runner
	if err := json.Unmarshal(listRec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].ID != runnerID || runners[0].Status != domain.RunnerStatusOffline {
		t.Fatalf("disconnect should retain an offline machine: %+v", runners)
	}
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/heartbeat", map[string]any{
		"status": "idle", "runningTasks": 0, "version": "0.2.0",
	}, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat after reconnect: %d %s", rec.Code, rec.Body.String())
	}
	listRec = requestWithHeaders(t, h, http.MethodGet, "/api/runners", nil, nil)
	if err := json.Unmarshal(listRec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].RunnerVersion != "0.2.0" {
		t.Fatalf("runner version after heartbeat: %+v", runners)
	}
}

func TestRunnerRegisterRejectsBadBootstrapToken(t *testing.T) {
	h := newRunnerTestServer(t)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-secret")
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{"name": "x"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token register status = %d, want 401", rec.Code)
	}
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{"name": "x"}, map[string]string{"X-Bootstrap-Token": "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token register status = %d, want 401", rec.Code)
	}
}

func TestRunnerRegisterDisabledWithoutBootstrapToken(t *testing.T) {
	h := newRunnerTestServer(t)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "")
	// No credential at all and registration not configured: 503.
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{"name": "x"}, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("register without credential or bootstrap token status = %d, want 503", rec.Code)
	}
	// A presented-but-invalid credential is always 401.
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{"name": "x"}, map[string]string{"X-Bootstrap-Token": "anything"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("register with invalid token status = %d, want 401", rec.Code)
	}
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

func TestRunnerMachinePlaneNeedsNoUserIdentityButKeepsRunnerAuth(t *testing.T) {
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "true")
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-secret")
	t.Setenv("AGENTFLOW_RUNNER_LONG_POLL_TIMEOUT", "10ms")
	dir := t.TempDir()
	binary := filepath.Join(dir, "agentflow-runner-linux-amd64")
	if err := os.WriteFile(binary, []byte("runner-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	gateBinary := filepath.Join(dir, "agentflow-gate-mcp-linux-amd64")
	if err := os.WriteFile(gateBinary, []byte("gate-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTFLOW_RUNNER_BINARY", filepath.Join(dir, "agentflow-runner-{os}-{arch}"))
	t.Setenv("AGENTFLOW_GATE_MCP_BINARY", filepath.Join(dir, "agentflow-gate-mcp-{os}-{arch}"))

	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "preview", "secret")

	for _, path := range []string{"/runners/install.sh", "/runners/health", "/runners/agentflow-runner?os=linux&arch=amd64"} {
		rec := requestWithHeaders(t, h, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("public machine asset %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	body := map[string]any{"name": "isolated-linux", "os": "linux", "arch": "amd64", "maxConcurrency": 1, "runtimes": []domain.RunnerRuntime{{Provider: "terminal", Status: "online"}}}
	registered := requestWithHeaders(t, h, http.MethodPost, "/runners/register", body, map[string]string{"X-Bootstrap-Token": "boot-secret"})
	if registered.Code != http.StatusOK {
		t.Fatalf("machine registration: status=%d body=%s", registered.Code, registered.Body.String())
	}
	var session struct {
		Token string `json:"sessionToken"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("registration response: %s", registered.Body.String())
	}

	unauthorized := requestWithHeaders(t, h, http.MethodPost, "/runners/heartbeat", map[string]any{}, nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("heartbeat without runner token: status=%d", unauthorized.Code)
	}
	auth := map[string]string{"Authorization": "Bearer " + session.Token}
	heartbeat := requestWithHeaders(t, h, http.MethodPost, "/runners/heartbeat", map[string]any{"runtimes": []domain.RunnerRuntime{{Provider: "terminal", Status: "online"}}}, auth)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("session heartbeat: status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
	}
	management := requestWithHeaders(t, h, http.MethodGet, "/api/runners", nil, nil)
	if management.Code != http.StatusUnauthorized {
		t.Fatalf("browser management unexpectedly public: status=%d body=%s", management.Code, management.Body.String())
	}
	for _, path := range []string{"/runners/bind-codes", "/runners/runtimes", "/runners/tasks", "/runners/tasks/task-1/cancel"} {
		rec := requestWithHeaders(t, h, http.MethodPost, path, map[string]any{}, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("management endpoint leaked into machine plane %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

// 直接派发执行机器任务等于在别人的机器上跑任意提示词，只有平台管理员可以用；
// 执行机器也只能读派给自己的任务，且拿不到任务里的凭证。


// 用户可以移除自己已经离线、不再使用的机器；在线机器和别人的机器不能移除。
func TestRunnerRemoveOnlyOwnOfflineMachine(t *testing.T) {
	h := newRunnerTestServer(t)
	t.Setenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN", "boot-secret")
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", "alice")
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/local", bytes.NewReader([]byte("{}")))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://example.com")
	loginReq.RemoteAddr = "127.0.0.1:60000"
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("local login: %d %s", loginRec.Code, loginRec.Body.String())
	}
	var cookie string
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == authCookieName {
			cookie = c.Value
		}
	}
	userAuth := map[string]string{"Cookie": authCookieName + "=" + cookie}
	rec := requestWithHeaders(t, h, http.MethodPost, "/api/runners/bind-codes", map[string]any{}, userAuth)
	var issued struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil || issued.Code == "" {
		t.Fatalf("bind code response: %d %s", rec.Code, rec.Body.String())
	}
	rec = requestWithHeaders(t, h, http.MethodPost, "/api/runners/register", map[string]any{
		"name": "old-mac", "os": "darwin", "arch": "arm64", "maxConcurrency": 1,
	}, map[string]string{"X-Bootstrap-Token": issued.Code})
	var registered struct {
		RunnerID     string `json:"runnerId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &registered); err != nil || registered.RunnerID == "" {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	poolRunnerID, _ := registerTestRunner(t, h)

	if rec = requestWithHeaders(t, h, http.MethodDelete, "/api/runners/"+registered.RunnerID, nil, userAuth); rec.Code != http.StatusConflict {
		t.Fatalf("remove online machine status = %d, want 409", rec.Code)
	}
	if rec = requestWithHeaders(t, h, http.MethodDelete, "/api/runners/"+poolRunnerID, nil, userAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("remove someone else's machine status = %d, want 403", rec.Code)
	}
	requestWithHeaders(t, h, http.MethodPost, "/api/runners/disconnect", map[string]any{}, map[string]string{"Authorization": "Bearer " + registered.SessionToken})
	if rec = requestWithHeaders(t, h, http.MethodDelete, "/api/runners/"+registered.RunnerID, nil, userAuth); rec.Code != http.StatusOK {
		t.Fatalf("remove offline machine: %d %s", rec.Code, rec.Body.String())
	}
	var mine []domain.Runner
	listRec := requestWithHeaders(t, h, http.MethodGet, "/api/runners?mine=1", nil, userAuth)
	if err := json.Unmarshal(listRec.Body.Bytes(), &mine); err != nil {
		t.Fatal(err)
	}
	if len(mine) != 0 {
		t.Fatalf("machine still listed after remove: %+v", mine)
	}
}


