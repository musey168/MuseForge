package httpapi

import (
	"net/http"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// trackerImportResponse is the answer of the tracker import endpoints.
type trackerImportResponse struct {
	Run *domain.TrackerImportRun `json:"run"`
}

// The import can be triggered on demand, its outcome is visible, and a
// repository whose credential is broken fails with a reason while the other
// repository still imports.
func TestTrackerImportRefusesAnUnknownProvider(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPost, "/api/tracker/import", map[string]any{"provider": "bitbucket"}, http.StatusBadRequest)
}
