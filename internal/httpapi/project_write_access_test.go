package httpapi

import (
	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The platform runs one account. A project write goes to the account that
// holds the project; every other account is refused.
func TestProjectContentWritesRequireTheAccountHoldingTheProject(t *testing.T) {
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "true")
	t.Setenv("AGENTFLOW_SCHEDULER_ENABLED", "false")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	h := New(s, nil, "", "")
	session := func(login string) *http.Cookie {
		r := httptest.NewRequest("POST", "http://localhost/api/auth/register", bytes.NewBufferString(`{"login":"`+login+`","password":"secret1"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.RemoteAddr = "192.0.2.10:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || len(w.Result().Cookies()) != 1 {
			t.Fatalf("register %s: %d %s", login, w.Code, w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	request := func(cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	call := func(cookie *http.Cookie, method, path, body string) int {
		return request(cookie, method, path, body).Code
	}
	session("admin")
	owner, outsider, other := session("owner"), session("outsider"), session("other")
	ownerUser, _ := s.UserByLoginName("owner")
	if err := s.PutProject(domain.Project{ID: "p1", Name: "P1", OwnerID: ownerUser.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProject(domain.Project{ID: "shared", Name: "Shared"}); err != nil {
		t.Fatal(err)
	}

	issue := fmt.Sprintf(`{"id":"i1","projectId":"p1","title":"Fix login","assigneeUserId":%d}`, ownerUser.ID)
	if code := call(other, "POST", "/api/issues", issue); code != 403 {
		t.Fatalf("another account create issue: %d", code)
	}
	if code := call(outsider, "POST", "/api/issues", issue); code != 403 {
		t.Fatalf("another account create issue: %d", code)
	}
	if code := call(owner, "POST", "/api/issues", issue); code != 200 {
		t.Fatalf("the account holding the project creates the issue: %d", code)
	}
	if code := call(other, "PUT", "/api/issues", `{"id":"i1","projectId":"p1","title":"Hijacked"}`); code != 403 {
		t.Fatalf("non-member edit issue: %d", code)
	}
	if code := call(other, "POST", "/api/issues/i1/close", `{}`); code != 403 {
		t.Fatalf("non-member close issue: %d", code)
	}
	if saved, _ := s.Issue("i1"); saved.Title != "Fix login" || saved.Status == "closed" {
		t.Fatalf("issue changed by a non-member: %+v", saved)
	}
	if code := call(other, "POST", "/api/issues", `{"projectId":"shared","title":"Shared idea"}`); code != 200 {
		t.Fatalf("ownerless projects stay shared: %d", code)
	}
	if err := s.PutProject(domain.Project{ID: "ownerless-team", Name: "Ownerless team"}); err != nil {
		t.Fatal(err)
	}
	if code := call(other, "POST", "/api/issues", `{"projectId":"ownerless-team","title":"Outsider idea"}`); code != 200 {
		t.Fatalf("ownerless projects stay open to the account: %d", code)
	}

	// The project response carries no member roster, no member roles and no
	// roster version any more.
	list := request(owner, "GET", "/api/projects", "")
	if list.Code != 200 {
		t.Fatalf("project list: %d", list.Code)
	}
	for _, field := range []string{`"members"`, `"membersVersion"`, `"businessDomainId"`} {
		if bytes.Contains(list.Body.Bytes(), []byte(field)) {
			t.Fatalf("the project response still carries %s: %s", field, list.Body.String())
		}
	}

	agent := `{"id":"a1","projectId":"p1","name":"Dev","instructions":"build"}`
	if code := call(other, "PUT", "/api/agents", agent); code != 403 {
		t.Fatalf("non-member write agent: %d", code)
	}
	if code := call(owner, "PUT", "/api/agents", agent); code != 200 {
		t.Fatalf("owner write agent: %d", code)
	}
	if code := call(other, "DELETE", "/api/agents/a1", ""); code != 403 {
		t.Fatalf("non-member delete agent: %d", code)
	}
	if code := call(other, "POST", "/api/agents/a1/clone", `{"id":"a1","name":"Mine","projectId":"shared"}`); code != 403 {
		t.Fatalf("clone must not overwrite another project's agent: %d", code)
	}
	if saved, _ := s.Agent("a1"); saved.ProjectID != "p1" || saved.Name != "Dev" {
		t.Fatalf("agent changed by a non-member: %+v", saved)
	}

	if err := s.PutRun(domain.Run{ID: "r1", Status: "succeeded", ExecutionContext: domain.TelemetryContext{ProjectID: "p1"}, Nodes: map[string]*domain.NodeRun{}}); err != nil {
		t.Fatal(err)
	}
	if code := call(other, "POST", "/api/runs/r1/cancel", `{}`); code != 403 {
		t.Fatalf("non-member cancel run: %d", code)
	}
	if code := call(other, "POST", "/api/runs/r1/workspace/terminal", `{"command":"id"}`); code != 403 {
		t.Fatalf("non-member terminal: %d", code)
	}
	if code := call(other, "DELETE", "/api/runs/r1", ""); code != 403 {
		t.Fatalf("non-member delete run: %d", code)
	}
	if code := call(owner, "DELETE", "/api/runs/r1", ""); code != 200 {
		t.Fatalf("the account holding the project deletes the run: %d", code)
	}

	rule := `{"id":"rule1","projectId":"p1","name":"Style","content":"gofmt"}`
	if code := call(other, "PUT", "/api/rules", rule); code != 403 {
		t.Fatalf("non-member write rule: %d", code)
	}
	if code := call(owner, "PUT", "/api/rules", rule); code != 200 {
		t.Fatalf("owner write rule: %d", code)
	}
	if code := call(other, "DELETE", "/api/rules?id=rule1", ""); code != 403 {
		t.Fatalf("non-member delete rule: %d", code)
	}
	if err := s.PutSandbox(domain.SandboxProfile{ID: "sb-global", Name: "Base", Driver: domain.SandboxDriverDocker, Dockerfile: &domain.SandboxDockerfile{Content: "FROM alpine"}}); err != nil {
		t.Fatal(err)
	}
	if code := call(owner, "PUT", "/api/sandboxes", `{"id":"sb-global","name":"Base","driver":"docker","dockerfile":{"content":"FROM alpine"},"privileged":true}`); code != 403 {
		t.Fatalf("non-admin edit platform sandbox: %d", code)
	}
	const sandboxBody = `{"id":"sb-project","projectId":"p1","name":"Docker","driver":"docker","dockerfile":{"content":"FROM alpine"},"privileged":%t}`
	enable := fmt.Sprintf(sandboxBody, true)
	if code := call(outsider, "PUT", "/api/sandboxes", enable); code != 403 {
		t.Fatalf("another account create privileged sandbox: %d", code)
	}
	if code := call(owner, "PUT", "/api/sandboxes", enable); code != 200 {
		t.Fatalf("the account holding the project creates a privileged sandbox: %d", code)
	}
	if saved, ok := s.Sandbox("sb-project"); !ok || !saved.Privileged {
		t.Fatalf("privileged mode was not saved: %+v", saved)
	}
	disable := fmt.Sprintf(sandboxBody, false)
	if code := call(other, "PUT", "/api/sandboxes", disable); code != 403 {
		t.Fatalf("another account edit project sandbox: %d", code)
	}
	if code := call(owner, "PUT", "/api/sandboxes", disable); code != 200 {
		t.Fatalf("the account holding the project disables privileged mode: %d", code)
	}
	if saved, ok := s.Sandbox("sb-project"); !ok || saved.Privileged {
		t.Fatalf("disabling privileged mode was not saved: %+v", saved)
	}
	if code := call(other, "PUT", "/api/sync/config", `{}`); code != 403 {
		t.Fatalf("non-admin sync config: %d", code)
	}
	if code := call(other, "POST", "/api/mcp-servers/custom", `{"scope":"project","projectId":"p1","server":{"id":"m1","name":"M","transport":"http","url":"https://example.com/mcp"}}`); code != 403 {
		t.Fatalf("non-member install project MCP: %d", code)
	}
}
