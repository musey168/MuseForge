package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

func TestDeleteIssueEndpoint(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := New(st, tracker.NewService(), "", "")
	issue := domain.Issue{ID: "delete-api", ProjectID: "platform", Title: "delete", Status: "backlog"}
	issue.Clarification.Status = "analyzing"
	if err := st.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?force=true"} {
		conflict := requestEmptyJSON[struct {
			Block *repo.IssueDeletionBlock `json:"deletionBlock"`
		}](t, h, http.MethodDelete, "/api/issues/delete-api"+query, http.StatusConflict)
		if conflict.Block == nil || conflict.Block.Code != "clarifying" {
			t.Fatalf("missing clarification block: %+v", conflict)
		}
	}
	issue.Clarification.Status = ""
	if err := st.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/issues/delete-api", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	requestEmptyJSON[map[string]string](t, h, http.MethodGet, "/api/issues/delete-api", http.StatusNotFound)
}


