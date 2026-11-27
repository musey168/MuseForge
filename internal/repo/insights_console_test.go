package repo

import (
	"math"
	"testing"
	"time"

	"agentflow/internal/domain"
)

// The console keeps a reported cost as it is, prices a report without one from
// the published table, and reads the cache-hit rate from the reported input.
func TestUsageConsoleCostPrecedenceAndCacheRate(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	source := domain.InsightSource{ID: "console-source", ProjectID: "alpha", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateInsightSource(source, "console-hash"); err != nil {
		t.Fatal(err)
	}
	reported := 0.42
	executions := []domain.InsightExecution{
		{ID: "e1", Type: "workflow_execution", ExternalExecutionID: "x1", Workflow: domain.InsightWorkflow{ID: "flow", Name: "External flow"}, Status: "succeeded", StartedAt: now.Add(-time.Hour), FinishedAt: now,
			Usage: []domain.InsightUsage{{Provider: "p", Model: "glm-5.3", InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, CacheWriteTokens: 100, CostStatus: "reported", CostUSD: &reported}}},
		{ID: "e2", Type: "workflow_execution", ExternalExecutionID: "x2", Workflow: domain.InsightWorkflow{ID: "flow", Name: "External flow"}, Status: "succeeded", StartedAt: now.Add(-time.Hour), FinishedAt: now,
			Usage: []domain.InsightUsage{{Provider: "q", Model: "glm-5.3", InputTokens: 2000, OutputTokens: 1000, CacheReadTokens: 400, CostStatus: "unknown"}}},
		{ID: "e3", Type: "workflow_execution", ExternalExecutionID: "x3", Workflow: domain.InsightWorkflow{ID: "flow", Name: "External flow"}, Status: "failed", StartedAt: now.Add(-time.Hour), FinishedAt: now,
			Usage: []domain.InsightUsage{{Provider: "q", Model: "no-tariff-model", InputTokens: 10, OutputTokens: 5, CostStatus: "unknown"}}},
	}
	for _, execution := range executions {
		if _, err := s.IngestInsightExecution(source, execution); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "alpha", Grain: "day", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	summary := snapshot.Summary
	if summary.Executions != 3 {
		t.Fatalf("executions = %d", summary.Executions)
	}
	if summary.TotalTokens != 4515 || summary.InputTokens != 3010 || summary.OutputTokens != 1505 || summary.CacheReadTokens != 600 || summary.CacheWriteTokens != 100 {
		t.Fatalf("tokens = %+v", summary)
	}
	// The reported input includes the cache tokens: cache-read over the input.
	if math.Abs(summary.CacheHitRate-600.0/3010.0*100) > 1e-9 {
		t.Fatalf("cache hit rate = %v", summary.CacheHitRate)
	}
	if summary.ReportedExecutions != 1 || summary.EstimatedExecutions != 1 || summary.UnknownCostExecutions != 1 {
		t.Fatalf("cost status counts = %+v", summary)
	}
	if math.Abs(summary.CostReportedUSD-0.42) > 1e-9 {
		t.Fatalf("reported cost = %v", summary.CostReportedUSD)
	}
	// glm-5.3 is published in CNY: (2000-400) x 8 + 1000 x 28 + 400 x 2 per million,
	// converted into the USD equivalent the platform keeps.
	estimated := (1600.0*8 + 1000.0*28 + 400.0*2) / 1e6 / domain.AnalyticsUSDToCNY
	if math.Abs(summary.CostEstimatedUSD-estimated) > 1e-9 {
		t.Fatalf("estimated cost = %v want %v", summary.CostEstimatedUSD, estimated)
	}
	if math.Abs(summary.TotalCostUSD-(0.42+estimated)) > 1e-9 {
		t.Fatalf("total cost = %v", summary.TotalCostUSD)
	}
	if math.Abs(summary.TotalCostCNY-summary.TotalCostUSD*domain.AnalyticsUSDToCNY) > 1e-9 || summary.CostCurrency != "CNY" {
		t.Fatalf("reader currency = %+v", summary)
	}
	if math.Abs(summary.CostCoveragePct-200.0/3.0) > 1e-9 {
		t.Fatalf("coverage = %v", summary.CostCoveragePct)
	}
	if len(snapshot.Models) != 2 || snapshot.Models[0].Key != "glm-5.3" || snapshot.Models[0].Tokens != 4500 {
		t.Fatalf("models = %+v", snapshot.Models)
	}
	if math.Abs(snapshot.Models[0].TokenSharePct-4500.0/4515.0*100) > 1e-9 {
		t.Fatalf("model token share = %v", snapshot.Models[0].TokenSharePct)
	}
	if len(snapshot.Providers) != 2 {
		t.Fatalf("providers = %+v", snapshot.Providers)
	}
	if len(snapshot.Executions) != 3 {
		t.Fatalf("executions = %+v", snapshot.Executions)
	}
	seen := map[string]bool{}
	for _, execution := range snapshot.Executions {
		seen[execution.ID] = true
	}
	if !seen["e1"] || !seen["e2"] || !seen["e3"] {
		t.Fatalf("executions = %+v", snapshot.Executions)
	}
}

// The trend buckets the reported executions in the period's own timezone, and
// one bucket narrows the period.
func TestUsageConsoleTrendBucketsInThePeriodTimezone(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	source := domain.InsightSource{ID: "trend-source", ProjectID: "beta", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateInsightSource(source, "trend-hash"); err != nil {
		t.Fatal(err)
	}
	late := time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC)
	early := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	for _, execution := range []domain.InsightExecution{
		{ID: "late", Type: "workflow_execution", ExternalExecutionID: "late", Workflow: domain.InsightWorkflow{ID: "flow", Name: "Flow"}, Status: "succeeded", StartedAt: late.Add(-time.Minute), FinishedAt: late,
			Usage: []domain.InsightUsage{{Provider: "p", Model: "glm-5.3", InputTokens: 10, OutputTokens: 5, CostStatus: "unknown"}}},
		{ID: "early", Type: "workflow_execution", ExternalExecutionID: "early", Workflow: domain.InsightWorkflow{ID: "flow", Name: "Flow"}, Status: "succeeded", StartedAt: early.Add(-time.Minute), FinishedAt: early,
			Usage: []domain.InsightUsage{{Provider: "p", Model: "glm-5.3", InputTokens: 10, OutputTokens: 5, CostStatus: "unknown"}}},
	} {
		if _, err := s.IngestInsightExecution(source, execution); err != nil {
			t.Fatal(err)
		}
	}
	utc, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "day", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if len(utc.Trend) != 1 || utc.Trend[0].Date != "2026-10-08" || utc.Trend[0].Executions != 2 {
		t.Fatalf("utc trend = %+v", utc.Trend)
	}
	shanghai, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "day", TimeZone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if len(shanghai.Trend) != 2 || shanghai.Trend[0].Date != "2026-10-08" || shanghai.Trend[1].Date != "2026-10-09" {
		t.Fatalf("shanghai trend = %+v", shanghai.Trend)
	}
	week, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "week", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if len(week.Trend) != 1 || week.Trend[0].Date != "2026-W41" {
		t.Fatalf("week trend = %+v", week.Trend)
	}
	month, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "month", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if len(month.Trend) != 1 || month.Trend[0].Date != "2026-10" {
		t.Fatalf("month trend = %+v", month.Trend)
	}
	bucket, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "day", TimeZone: "UTC", Bucket: "2026-10-08"})
	if err != nil {
		t.Fatal(err)
	}
	if bucket.Summary.Executions != 2 || bucket.Executions[0].ID != "late" {
		t.Fatalf("bucket scope = %+v", bucket.Summary)
	}
	if _, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "beta", Grain: "day", Bucket: "not-a-bucket"}); err == nil {
		t.Fatal("an unreadable bucket was accepted")
	}
}

// A scope without reports is empty, not a page of zeros.
func TestUsageConsoleEmptyScope(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	snapshot, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Empty || snapshot.Summary.Executions != 0 || snapshot.Summary.TotalTokens != 0 {
		t.Fatalf("empty scope = %+v", snapshot.Summary)
	}
	if len(snapshot.Trend) != 0 || len(snapshot.Executions) != 0 {
		t.Fatalf("empty scope lists = %+v", snapshot)
	}
}

// The refused reports of a credential are counted, so the console can show
// that reports arrived but were not usable.
func TestUsageConsoleCountsRefusedReports(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	source := domain.InsightSource{ID: "rejected-source", ProjectID: "gamma", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateInsightSource(source, "rejected-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.CountRejectedInsightReport(source.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CountRejectedInsightReport(source.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.UsageConsole(domain.AnalyticsFilter{ProjectID: "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.MalformedReports != 2 {
		t.Fatalf("malformed reports = %v", snapshot.Summary.MalformedReports)
	}
}
