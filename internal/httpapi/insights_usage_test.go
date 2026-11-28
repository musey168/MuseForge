package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// The console shows the tokens, the cost, the cache hits and the executions of
// the work reported from outside the platform.
func TestUsageConsoleReadsReportedExecutions(t *testing.T) {
	t.Setenv("AGENTFLOW_SCHEDULER_ENABLED", "false")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.PutUser(domain.User{LoginName: "console-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := s.UserByLoginName("console-owner")
	if err := s.PutProject(domain.Project{ID: "console-project", Name: "Console", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	h := New(s, tracker.NewService(), "", "")
	created := requestJSONAuthed[map[string]any](t, h, owner, "POST", "/api/insights/v1/sources", map[string]any{"projectId": "console-project", "name": "CI", "expiresInDays": 30}, 201)
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("credential: %+v", created)
	}
	post := func(token string, body any, want int) string {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, insightIngestPath, bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("ingest = %d want %d: %s", rec.Code, want, rec.Body.String())
		}
		return rec.Body.String()
	}
	now := time.Now().UTC().Add(-time.Minute)
	reported := 0.5
	first := domain.InsightExecution{ID: "e1", Type: "workflow_execution", ExternalExecutionID: "x1", Workflow: domain.InsightWorkflow{ID: "flow", Name: "CI flow"}, Status: "succeeded", StartedAt: now.Add(-time.Minute), FinishedAt: now,
		Usage: []domain.InsightUsage{{Provider: "p", Model: "glm-5.3", InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, CostStatus: "reported", CostUSD: &reported}}}
	post(token, first, 201)
	second := first
	second.ID = "e2"
	second.ExternalExecutionID = "x2"
	second.FinishedAt = now.Add(time.Minute)
	second.StartedAt = now
	second.Usage = []domain.InsightUsage{{Provider: "q", Model: "glm-5.3", InputTokens: 400, OutputTokens: 100, CostStatus: "unknown"}}
	post(token, second, 201)
	// A malformed report is refused and counted; the console keeps working.
	post(token, map[string]any{"id": "broken"}, 400)

	snapshot := requestJSONAuthed[domain.UsageConsoleSnapshot](t, h, owner, "GET", "/api/usage?projectId=console-project&grain=day&tz=UTC", nil, 200)
	if snapshot.Empty || snapshot.Summary.Executions != 2 {
		t.Fatalf("console = %+v", snapshot.Summary)
	}
	if snapshot.Summary.TotalTokens != 2000 || snapshot.Summary.InputTokens != 1400 || snapshot.Summary.OutputTokens != 600 || snapshot.Summary.CacheReadTokens != 200 {
		t.Fatalf("tokens = %+v", snapshot.Summary)
	}
	// The reported input includes the cache tokens: cache-read over the input.
	if snapshot.Summary.CacheHitRate != 200.0/1400.0*100 {
		t.Fatalf("cache hit rate = %v", snapshot.Summary.CacheHitRate)
	}
	if snapshot.Summary.ReportedExecutions != 1 || snapshot.Summary.EstimatedExecutions != 1 || snapshot.Summary.UnknownCostExecutions != 0 {
		t.Fatalf("cost status = %+v", snapshot.Summary)
	}
	if snapshot.Summary.CostReportedUSD != 0.5 || snapshot.Summary.TotalCostCNY != snapshot.Summary.TotalCostUSD*domain.AnalyticsUSDToCNY {
		t.Fatalf("cost = %+v", snapshot.Summary)
	}
	if snapshot.Summary.MalformedReports != 1 {
		t.Fatalf("malformed reports = %v", snapshot.Summary.MalformedReports)
	}
	if len(snapshot.Models) != 1 || snapshot.Models[0].Key != "glm-5.3" || len(snapshot.Providers) != 2 {
		t.Fatalf("breakdowns = %+v %+v", snapshot.Models, snapshot.Providers)
	}
	if len(snapshot.Trend) != 1 || snapshot.Trend[0].Executions != 2 {
		t.Fatalf("trend = %+v", snapshot.Trend)
	}
	if len(snapshot.Executions) != 2 || snapshot.Executions[0].ID != "e2" || snapshot.Executions[1].ID != "e1" {
		t.Fatalf("executions are not newest first: %+v", snapshot.Executions)
	}
	if len(snapshot.Executions[0].Usage) != 1 || snapshot.Executions[0].Usage[0].Model != "glm-5.3" {
		t.Fatalf("execution detail = %+v", snapshot.Executions[0])
	}
	// A scope without reports is empty, not a page of zeros.
	empty := requestJSONAuthed[domain.UsageConsoleSnapshot](t, h, owner, "GET", "/api/usage?projectId=quiet-project", nil, 200)
	if !empty.Empty || empty.Summary.Executions != 0 {
		t.Fatalf("empty scope = %+v", empty.Summary)
	}
	// A revoked credential is refused.
	requestJSONAuthed[map[string]any](t, h, owner, "DELETE", "/api/insights/v1/sources/"+created["source"].(map[string]any)["id"].(string), nil, 200)
	post(token, first, 401)
	// An expired credential is refused.
	expired := domain.InsightSource{ID: "expired-console", ProjectID: "console-project", ExpiresAt: time.Now().UTC().Add(-time.Hour)}
	if err := s.CreateInsightSource(expired, insightTokenHash("expired-console-token")); err != nil {
		t.Fatal(err)
	}
	post("expired-console-token", first, 401)
}
