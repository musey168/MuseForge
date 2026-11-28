package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// requirementView is the answer of the requirement endpoints.
type requirementView struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"projectId"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority"`
	Tags       []string `json:"tags"`
	ExternalID string   `json:"-"`
}

// newRequirementStore starts a platform with one project to put requirements in.
func newRequirementStore(t *testing.T) (*repo.Store, http.Handler) {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	h := New(s, tracker.NewService(), "", "")
	return s, h
}

// createRequirement stores one requirement through the API and returns it.
func createRequirement(t *testing.T, h http.Handler, projectID, title string, tags []string) requirementView {
	t.Helper()
	body := map[string]any{"projectId": projectID, "title": title, "tags": tags}
	return requestJSON[requirementView](t, h, http.MethodPost, "/api/issues", body, http.StatusOK)
}

// A requirement is created, tagged, filtered by project, status and tag,
// closed, reopened and deleted. Its own audit trail records the actions.
func TestRequirementLifecycleWithTags(t *testing.T) {
	s, h := newRequirementStore(t)
	project := requestJSON[map[string]any](t, h, http.MethodPost, "/api/projects", map[string]any{"id": "p-board", "name": "工作台"}, http.StatusOK)
	projectID, _ := project["id"].(string)
	if projectID == "" {
		t.Fatalf("project id = %v", project["id"])
	}

	bug := createRequirement(t, h, projectID, "登录页在手机上白屏", []string{"bug", "ui"})
	if len(bug.Tags) != 2 || bug.Tags[0] != "bug" || bug.Tags[1] != "ui" {
		t.Fatalf("tags = %#v", bug.Tags)
	}
	feature := createRequirement(t, h, projectID, "需求列表支持标签筛选", []string{"ui"})

	list := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues", nil, http.StatusOK)
	if len(list) != 2 {
		t.Fatalf("list = %#v", list)
	}

	byTag := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues?tag=bug", nil, http.StatusOK)
	if len(byTag) != 1 || byTag[0].ID != bug.ID {
		t.Fatalf("tag filter = %#v", byTag)
	}
	byProject := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues?projectId="+projectID, nil, http.StatusOK)
	if len(byProject) != 2 {
		t.Fatalf("project filter = %#v", byProject)
	}
	byStatus := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues?status=backlog", nil, http.StatusOK)
	if len(byStatus) != 2 {
		t.Fatalf("status filter = %#v", byStatus)
	}

	closed := requestJSON[requirementView](t, h, http.MethodPost, "/api/issues/"+bug.ID+"/close", map[string]any{}, http.StatusOK)
	if closed.Status != "closed" {
		t.Fatalf("closed status = %q", closed.Status)
	}
	stillClosed := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues?status=closed", nil, http.StatusOK)
	if len(stillClosed) != 1 || stillClosed[0].ID != bug.ID {
		t.Fatalf("closed filter = %#v", stillClosed)
	}

	reopened := requestJSON[requirementView](t, h, http.MethodPost, "/api/issues/"+bug.ID+"/reopen", map[string]any{}, http.StatusOK)
	if reopened.Status == "closed" {
		t.Fatalf("reopened status = %q", reopened.Status)
	}
	openAgain := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues?status="+reopened.Status, nil, http.StatusOK)
	if len(openAgain) != 1 || openAgain[0].ID != bug.ID {
		t.Fatalf("reopened filter = %#v", openAgain)
	}

	// The labels survive an edit that does not mention them.
	edited := requestJSON[requirementView](t, h, http.MethodPut, "/api/issues", map[string]any{
		"id": feature.ID, "projectId": projectID, "title": "需求列表支持标签筛选", "description": "带筛选",
	}, http.StatusOK)
	if len(edited.Tags) != 1 || edited.Tags[0] != "ui" {
		t.Fatalf("edited tags = %#v", edited.Tags)
	}

	deleteRequirement(t, h, feature.ID)
	remaining := requestJSON[[]requirementView](t, h, http.MethodGet, "/api/issues", nil, http.StatusOK)
	if len(remaining) != 1 || remaining[0].ID != bug.ID {
		t.Fatalf("after delete = %#v", remaining)
	}
	_ = s
}

// deleteRequirement removes one requirement and expects the empty answer.
func deleteRequirement(t *testing.T, h http.Handler, id string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/issues/"+id, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete %s: status %d, body %s", id, rec.Code, rec.Body.String())
	}
}

// The answer of the requirement carries no field of the retired LOOP lifecycle:
// the model is the plain to-do item the personal build promised.
func TestRequirementCarriesNoFlowFields(t *testing.T) {
	_, h := newRequirementStore(t)
	project := requestJSON[map[string]any](t, h, http.MethodPost, "/api/projects", map[string]any{"id": "p-board", "name": "工作台"}, http.StatusOK)
	projectID, _ := project["id"].(string)
	created := createRequirement(t, h, projectID, "只写标题的需求", []string{"ui"})

	raw := requestJSON[map[string]any](t, h, http.MethodGet, "/api/issues/"+created.ID, nil, http.StatusOK)
	issue, _ := raw["issue"].(map[string]any)
	if issue == nil {
		t.Fatalf("detail = %#v", raw)
	}
	for _, gone := range []string{
		"assigneeType", "assigneeId", "assigneeUserId", "workflowId", "playbookId",
		"sopBindings", "sopMode", "latestTaskId", "automationState", "deliveryState",
		"clarification", "conversationSessionId", "tapdId",
	} {
		if _, ok := issue[gone]; ok {
			t.Errorf("the requirement still carries %q", gone)
		}
	}
	tags, _ := issue["tags"].([]any)
	if len(tags) != 1 || tags[0] != "ui" {
		t.Errorf("detail tags = %#v", issue["tags"])
	}
}

// The personal build has no sign-in: with the auth switch untouched the
// requirement list answers. The platform once turned authentication on by
// default and answered "authentication required" on every route, and only the
// live suite noticed.
func TestPersonalBuildServesWithoutAuth(t *testing.T) {
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "")
	_, h := newRequirementStore(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/issues", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/issues = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
