package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agentflow/internal/engine"
	"agentflow/internal/repo"
	"agentflow/internal/runtime"
	"agentflow/internal/tracker"
)

// The chat workbench is gone. Its routes must not answer: the home page is
// the requirement list, and there is no bot to ask.
func TestChatWorkbenchRoutesAreGone(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "workspaces"))), tracker.NewService(), "", "")

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/assistant/chat"},
		{http.MethodPost, "/api/assistant/generations"},
		{http.MethodGet, "/api/assistant/sessions"},
		{http.MethodGet, "/api/assistant/sessions/abc"},
		{http.MethodPut, "/api/assistant/sessions/abc"},
		{http.MethodDelete, "/api/assistant/sessions/abc"},
		{http.MethodPost, "/api/assistant/sessions/abc/generation"},
		{http.MethodPost, "/api/assistant/sessions/abc/generation/cancel"},
		{http.MethodPost, "/api/assistant/sessions/abc/generation/steer"},
		{http.MethodGet, "/api/assistant/sessions/abc/generation"},
		{http.MethodGet, "/api/assistant/sessions/abc/generations/gen/diagnostics"},
		{http.MethodPost, "/api/assistant/sessions/abc/proposals"},
		{http.MethodGet, "/api/assistant/preferences"},
		{http.MethodPut, "/api/assistant/preferences"},
		{http.MethodPost, "/api/assistant/approval-review/probe"},
		{http.MethodGet, "/api/assistant/models"},
		{http.MethodPost, "/api/assistant/cli-install"},
		{http.MethodGet, "/api/assistant/cli-install"},
		{http.MethodPost, "/api/assistant/attachments"},
		{http.MethodGet, "/api/assistant/attachments/abc"},
		{http.MethodGet, "/api/assistant/references"},
		{http.MethodGet, "/api/runs/run-1/conversation"},
		{http.MethodGet, "/api/issues/ISSUE-1/conversations"},
		{http.MethodGet, "/api/assistant/tasks"},
		{http.MethodPut, "/api/assistant/tasks"},
		{http.MethodPost, "/api/assistant/tasks/task-1/run"},
		{http.MethodGet, "/api/bot-drafts/draft-1"},
		{http.MethodPost, "/api/chat-clarify"},
		{http.MethodGet, "/api/chat-clarify"},
		{http.MethodPost, "/api/chat-clarify/cancel"},
		{http.MethodPost, "/api/issues/ISSUE-1/chat-clarify"},
		{http.MethodGet, "/api/issues/ISSUE-1/chat-clarify"},
		{http.MethodPost, "/api/issues/ISSUE-1/chat-clarify/cancel"},
		{http.MethodGet, "/api/issues/ISSUE-1/clarify-models"},
		{http.MethodGet, "/api/rooms"},
		{http.MethodPost, "/api/rooms"},
		{http.MethodGet, "/api/rooms/room-1"},
		{http.MethodPost, "/api/rooms/room-1/messages"},
		{http.MethodGet, "/api/spaces"},
		{http.MethodPost, "/api/spaces"},
		{http.MethodGet, "/api/spaces/space-1"},
		{http.MethodGet, "/api/spaces/space-1/messages"},
		{http.MethodGet, "/api/spaces/space-1/artifacts"},
		{http.MethodPost, "/api/runs/run-1/browser"},
		{http.MethodGet, "/api/runs/run-1/browser/stream"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, "http://agentflow.example"+tc.path, nil))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s still answers with %d", tc.method, tc.path, rec.Code)
		}
	}
}
