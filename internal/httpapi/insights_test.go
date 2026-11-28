package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// insightSourceResponse is the answer of the source endpoint: the source and,
// on creation only, the token that is shown once.
type insightSourceResponse struct {
	Source struct {
		ID        string `json:"id"`
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
	} `json:"source"`
	Token string `json:"token"`
}

type usageResponse struct {
	Summary struct {
		Executions     int     `json:"executions"`
		InputTokens    int64   `json:"inputTokens"`
		OutputTokens   int64   `json:"outputTokens"`
		TotalTokens    int64   `json:"totalTokens"`
		TotalCostUsd   float64 `json:"totalCostUsd"`
		CacheHitRate   float64 `json:"cacheHitRate"`
		MalformedCount int     `json:"malformedReports"`
	} `json:"summary"`
	Models []struct {
		Key          string `json:"key"`
		Executions   int    `json:"executions"`
		Tokens       int64  `json:"tokens"`
		InputTokens  int64  `json:"inputTokens"`
		OutputTokens int64  `json:"outputTokens"`
	} `json:"models"`
	Empty bool `json:"empty"`
}

func newUsageServer(t *testing.T) http.Handler {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPut, "/api/projects", map[string]any{"id": "demo", "key": "DM", "name": "演示", "ownerId": 1}, http.StatusOK)
	return h
}

// The reporting credential is created once, its token is returned exactly once,
// and a report it authenticates reaches the usage console.
func TestUsageReportReachesTheConsole(t *testing.T) {
	h := newUsageServer(t)

	// The console is empty before anything is reported.
	empty := requestJSON[usageResponse](t, h, http.MethodGet, "/api/usage", nil, http.StatusOK)
	if !empty.Empty || empty.Summary.Executions != 0 {
		t.Fatalf("a fresh console must be empty: %+v", empty.Summary)
	}

	created := requestJSON[insightSourceResponse](t, h, http.MethodPost, "/api/insights/v1/sources", map[string]any{"projectId": "demo", "name": "omp", "expiresInDays": 30}, http.StatusCreated)
	if created.Token == "" || created.Source.ID == "" {
		t.Fatalf("the source endpoint must return the token once: %+v", created)
	}

	report := map[string]any{
		"id":                  "exec-1",
		"type":                "workflow_execution",
		"externalExecutionId": "exec-1",
		"workflow":            map[string]any{"id": "omp", "name": "omp"},
		"status":              "succeeded",
		"startedAt":           "2026-11-28T05:00:00Z",
		"finishedAt":          "2026-11-28T05:10:00Z",
		"usage": []map[string]any{{
			"provider": "anthropic", "model": "claude-opus-4",
			"inputTokens": 1200, "outputTokens": 500, "cacheReadTokens": 200,
			"costUsd": 0.25, "costStatus": "reported",
		}},
	}
	if code, body := postWithBearer(t, h, "/api/insights/v1/ingest", created.Token, report); code != http.StatusCreated {
		t.Fatalf("ingest = %d %s", code, body)
	}

	console := requestJSON[usageResponse](t, h, http.MethodGet, "/api/usage", nil, http.StatusOK)
	if console.Summary.Executions != 1 || console.Summary.TotalTokens != 1700 {
		t.Fatalf("console summary = %+v", console.Summary)
	}
	if len(console.Models) != 1 || console.Models[0].InputTokens != 1200 || console.Models[0].OutputTokens != 500 {
		t.Fatalf("the model breakdown must split input from output: %+v", console.Models)
	}
	if console.Summary.TotalCostUsd <= 0 {
		t.Fatalf("a reported cost must reach the console: %+v", console.Summary)
	}

	// The same report again is a duplicate, not a second execution.
	if code, body := postWithBearer(t, h, "/api/insights/v1/ingest", created.Token, report); code != http.StatusOK {
		t.Fatalf("a duplicate report = %d %s, want 200", code, body)
	}
	after := requestJSON[usageResponse](t, h, http.MethodGet, "/api/usage", nil, http.StatusOK)
	if after.Summary.Executions != 1 {
		t.Fatalf("a duplicate must not count twice: %+v", after.Summary)
	}
}

// A malformed report is counted and skipped: it never breaks the console.
func TestMalformedUsageReportIsCountedAndSkipped(t *testing.T) {
	h := newUsageServer(t)
	created := requestJSON[insightSourceResponse](t, h, http.MethodPost, "/api/insights/v1/sources", map[string]any{"projectId": "demo", "name": "omp", "expiresInDays": 30}, http.StatusCreated)

	if code, _ := postWithBearer(t, h, "/api/insights/v1/ingest", created.Token, map[string]any{"type": "workflow_execution"}); code < 400 || code >= 500 {
		t.Fatalf("a malformed report = %d, want a 4xx", code)
	}
	console := requestJSON[usageResponse](t, h, http.MethodGet, "/api/usage", nil, http.StatusOK)
	if console.Summary.MalformedCount != 1 || console.Summary.Executions != 0 {
		t.Fatalf("a malformed report must be counted, not stored: %+v", console.Summary)
	}
}

// A report without a valid token is refused: the console read is unauthenticated,
// the report is not.
func TestUsageIngestNeedsTheSourceToken(t *testing.T) {
	h := newUsageServer(t)
	if code, _ := postWithBearer(t, h, "/api/insights/v1/ingest", "afi_not-a-token", map[string]any{"type": "workflow_execution"}); code != http.StatusUnauthorized {
		t.Fatalf("an unknown token = %d, want 401", code)
	}
}

// postWithBearer sends one JSON body with a Bearer token and returns the status
// and the body, so a test can assert on either.
func postWithBearer(t *testing.T, h http.Handler, path, token string, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}
