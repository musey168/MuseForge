package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// The platform serves its own surfaces from a SQLite file: no database
// server, no manual migration step, no seeded rows.
func TestPlatformServesItsSurfacesFromSQLiteFile(t *testing.T) {
	t.Setenv("AGENTFLOW_SESSION_SECRET", "sqlite-platform-session-secret")
	root := t.TempDir()
	s, err := repo.Open(filepath.Join(root, "museforge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")

	if err := s.PutUser(domain.User{StaffID: 11, LoginName: "muse", ChineseName: "缪斯"}); err != nil {
		t.Fatal(err)
	}
	user, ok := s.UserByLoginName("muse")
	if !ok {
		t.Fatal("user was not stored")
	}
	if err := s.PutProject(domain.Project{ID: "atlas", Key: "ATL", Name: "Atlas", OwnerID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "req-1", ProjectID: "atlas", Title: "把用量台接到 SQLite", Description: "写入即持久", Status: "todo", Priority: "high", AssigneeType: "member", AssigneeUserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRunner(domain.Runner{ID: "runner-1", Name: "studio", OS: "darwin", Arch: "arm64", MaxConcurrency: 1, Status: "online", OwnerUserID: user.ID}); err != nil {
		t.Fatal(err)
	}

	issues := requestJSON[[]issueDeletionView](t, h, http.MethodGet, "/api/issues", nil, http.StatusOK)
	if len(issues) != 1 || issues[0].ID != "req-1" {
		t.Fatalf("requirement list from sqlite = %+v", issues)
	}
	projects := requestJSON[[]domain.Project](t, h, http.MethodGet, "/api/projects", nil, http.StatusOK)
	if len(projects) != 1 || projects[0].ID != "atlas" {
		t.Fatalf("project list from sqlite = %+v", projects)
	}
	runners := requestJSON[[]domain.Runner](t, h, http.MethodGet, "/api/runners", nil, http.StatusOK)
	if len(runners) != 1 || runners[0].ID != "runner-1" {
		t.Fatalf("machine inventory from sqlite = %+v", runners)
	}

	// Personal settings: the credentials the user saves are stored and read back.
	saved := requestJSONAuthed[domain.User](t, h, user, http.MethodPut, "/api/users/"+strconv.FormatInt(user.ID, 10)+"/credentials", map[string]any{"codeBuddyApiKey": "ck_sqlite"}, http.StatusOK)
	if !saved.CodeBuddyAPIKeyConfigured {
		t.Fatalf("personal settings did not record the credential: %+v", saved)
	}
	stored, ok := s.User(user.ID)
	if !ok || stored.CodeBuddyAPIKey != "ck_sqlite" {
		t.Fatalf("personal settings were not stored: %+v", stored)
	}

	// The external usage report: a data source is created, then an execution
	// is ingested through the source's bearer token.
	created := requestJSONAuthed[struct {
		Source domain.InsightSource `json:"source"`
		Token  string               `json:"token"`
	}](t, h, user, http.MethodPost, "/api/insights/v1/sources", map[string]any{
		"projectId":     "atlas",
		"name":          "runner",
		"userMappings":  map[string]int64{"external-user": user.ID},
		"expiresInDays": 30,
	}, http.StatusCreated)
	if created.Source.ID == "" || created.Token == "" {
		t.Fatalf("insight source was not created: %+v", created)
	}
	now := time.Now().UTC()
	ingested := requestJSONBearer[map[string]any](t, h, http.MethodPost, "/api/insights/v1/ingest", created.Token, map[string]any{
		"id":                  "exec-1",
		"type":                "workflow_execution",
		"externalExecutionId": "ext-1",
		"workflow":            map[string]any{"id": "wf-1", "name": "用量上报"},
		"externalUserId":      "external-user",
		"status":              "succeeded",
		"startedAt":           now.Add(-time.Minute).Format(time.RFC3339Nano),
		"finishedAt":          now.Format(time.RFC3339Nano),
		"usage": []map[string]any{{
			"provider": "omp", "model": "muse-1", "inputTokens": 120, "outputTokens": 34,
			"cacheReadTokens": 0, "cacheWriteTokens": 0, "reasoningTokens": 0, "costStatus": "unknown",
		}},
	}, http.StatusCreated)
	if ingested["status"] != "accepted" {
		t.Fatalf("usage report was not accepted: %+v", ingested)
	}
	sources := requestJSONAuthed[[]domain.InsightSource](t, h, user, http.MethodGet, "/api/insights/v1/sources?projectId=atlas", nil, http.StatusOK)
	if len(sources) != 1 || sources[0].ID != created.Source.ID {
		t.Fatalf("insight sources from sqlite = %+v", sources)
	}
}

// requestJSONBearer performs a request that carries a bearer token instead of
// a session cookie.
func requestJSONBearer[T any](t *testing.T, h http.Handler, method, path, token string, body any, wantStatus int) T {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: status %d, body %s", method, path, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
