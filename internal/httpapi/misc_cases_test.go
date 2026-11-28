package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

func cvBMiscRequestAs(t *testing.T, f cvBHTTPFixture, user domain.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(t, req, user)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func cvBMiscContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, rec
}



func TestMiscProjectPlaybooksPermissionsAndAssignments(t *testing.T) {
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "true")
	f := cvBNewHTTPFixture(t)
	member := domain.User{ID: 7102, LoginName: "cvb-member", ChineseName: "成员"}
	outsider := domain.User{ID: 7103, LoginName: "cvb-outsider"}
	if err := f.s.PutUser(member); err != nil {
		t.Fatal(err)
	}
	if err := f.s.PutUser(outsider); err != nil {
		t.Fatal(err)
	}
	if err := f.s.PutWorkflow(domain.WorkflowDefinition{
		ID: "cvb-playbook-wf", Name: "playbook", ProjectID: "platform",
		Status: "published", Version: 1, SandboxProfileID: "test-native",
		Nodes: []domain.WorkflowNode{{ID: "approve", Name: "approve", Type: "approval", MaxAttempts: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	h := &projectsHandlers{deps: &deps{repo: f.s}}
	if err := h.applyProjectPlaybook(nil); err != nil {
		t.Fatal(err)
	}
	emptyBooks := domain.Project{ID: "platform", SOPPlaybooks: []domain.SOPPlaybook{}}
	if err := h.applyProjectPlaybook(&emptyBooks); err != nil {
		t.Fatal(err)
	}
	badBook := domain.Project{ID: "platform", SOPPlaybooks: []domain.SOPPlaybook{{
		ID: "bad", Bindings: []domain.IssueSOPBinding{{WorkflowID: "missing"}},
	}}}
	if err := h.applyProjectPlaybook(&badBook); err == nil {
		t.Fatal("missing playbook workflow accepted")
	}
	goodBook := domain.Project{ID: "platform", SOPPlaybooks: []domain.SOPPlaybook{{
		ID: "good", Name: "Good", Default: true,
		Bindings: []domain.IssueSOPBinding{{WorkflowID: "cvb-playbook-wf"}},
	}}}
	if err := h.applyProjectPlaybook(&goodBook); err != nil || goodBook.WorkflowID != "cvb-playbook-wf" {
		t.Fatalf("good playbook = %+v, %v", goodBook, err)
	}
	legacyEmpty := domain.Project{ID: "platform", SOPBindings: []domain.IssueSOPBinding{}}
	if err := h.applyProjectPlaybook(&legacyEmpty); err != nil || legacyEmpty.SOPBindings != nil {
		t.Fatalf("empty legacy bindings = %+v, %v", legacyEmpty, err)
	}
	legacyBad := domain.Project{ID: "platform", SOPBindings: []domain.IssueSOPBinding{{WorkflowID: "missing"}}}
	if err := h.applyProjectPlaybook(&legacyBad); err == nil {
		t.Fatal("missing legacy workflow accepted")
	}
	legacyGood := domain.Project{ID: "platform", SOPBindings: []domain.IssueSOPBinding{{WorkflowID: "cvb-playbook-wf"}}}
	if err := h.applyProjectPlaybook(&legacyGood); err != nil || legacyGood.WorkflowID != "cvb-playbook-wf" {
		t.Fatalf("legacy binding = %+v, %v", legacyGood, err)
	}

	if err := f.s.PutProject(domain.Project{ID: "cvb-owned", Key: "OWN", Name: "Owned", OwnerID: f.user.ID}); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		user domain.User
		body any
		want int
	}{
		{member, map[string]any{"id": "cvb-owned", "name": "changed"}, http.StatusForbidden},
		{member, map[string]any{"id": "cvb-new-wrong-owner", "name": "new", "ownerId": outsider.ID}, http.StatusForbidden},
		{member, map[string]any{"id": "cvb-new-member", "name": "new"}, http.StatusOK},
	} {
		rec := cvBMiscRequestAs(t, f, tc.user, http.MethodPost, "/api/projects", tc.body)
		if rec.Code != tc.want {
			t.Fatalf("project permission case %d = %d want %d: %s", i, rec.Code, tc.want, rec.Body.String())
		}
	}
	if rec := cvBMiscRequestAs(t, f, member, http.MethodPost, "/api/projects", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed project = %d: %s", rec.Code, rec.Body.String())
	}
	c, rec := cvBMiscContext(http.MethodPost, "/api/projects")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"id":"cvb-direct","name":"direct"}`))
	c.Request.ContentLength = int64(len(`{"id":"cvb-direct","name":"direct"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	(&projectsHandlers{deps: &deps{repo: f.s}}).projects(c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("direct unauthenticated project = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := cvBMiscRequestAs(t, f, f.user, http.MethodPost, "/api/projects", map[string]any{
		"id": "cvb-playbook-bad", "name": "bad",
		"sopBindings": []any{map[string]any{"workflowId": "missing"}},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad project playbook = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := cvBMiscRequestAs(t, f, f.user, http.MethodPost, "/api/projects", map[string]any{
		"id": "cvb-tapd-clear-new", "name": "clear", "tapdClear": true,
	}); rec.Code != http.StatusOK {
		t.Fatalf("new TAPD clear project = %d: %s", rec.Code, rec.Body.String())
	}
}
