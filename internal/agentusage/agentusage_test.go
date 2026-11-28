package agentusage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"
)

func TestExecutionsBuildsPlatformIdentity(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 123456789, time.UTC)
	sessions := []Session{{
		Key:                 "2026-10-01T00-00-00-000Z_aaaa",
		StartedAt:           start,
		FinishedAt:          start.Add(90 * time.Minute),
		Status:              "failed",
		ExecutionDurationMs: new(int64(3000)),
		Usage: []domain.InsightUsage{
			{Provider: "minimax-code", Model: "MiniMax-M3", InputTokens: 100, OutputTokens: 10, CacheReadTokens: 40, CostUSD: new(0.5), CostStatus: "reported"},
			{Provider: "minimax-code", Model: "MiniMax-M3", InputTokens: 200, OutputTokens: 20, CacheReadTokens: 60, CostUSD: new(0.25), CostStatus: "reported"},
		},
	}}
	executions := Executions(AgentOMP, sessions)
	if len(executions) != 1 {
		t.Fatalf("want 1 execution, got %d", len(executions))
	}
	got := executions[0]
	if got.ID != "omp:2026-10-01T00-00-00-000Z_aaaa" || got.ExternalExecutionID != got.ID {
		t.Fatalf("identity is not deterministic: id=%q external=%q", got.ID, got.ExternalExecutionID)
	}
	if got.Type != "workflow_execution" || got.Status != "failed" {
		t.Fatalf("type/status = %q/%q", got.Type, got.Status)
	}
	if got.Workflow.ID != "omp" || got.Workflow.Name != "omp" {
		t.Fatalf("workflow = %+v", got.Workflow)
	}
	if got.ExecutionDurationMs == nil || *got.ExecutionDurationMs != 3000 {
		t.Fatalf("duration not carried: %v", got.ExecutionDurationMs)
	}
	if !got.StartedAt.Equal(start.Truncate(time.Millisecond)) || !got.FinishedAt.Equal(start.Add(90*time.Minute).Truncate(time.Millisecond)) {
		t.Fatalf("timestamps not normalised: %s .. %s", got.StartedAt, got.FinishedAt)
	}
	if len(got.Usage) != 1 {
		t.Fatalf("duplicate provider/model entries must merge, got %d", len(got.Usage))
	}
	usage := got.Usage[0]
	if usage.InputTokens != 300 || usage.OutputTokens != 30 || usage.CacheReadTokens != 100 {
		t.Fatalf("merged usage = %+v", usage)
	}
	if usage.CostStatus != "reported" || usage.CostUSD == nil || *usage.CostUSD != 0.75 {
		t.Fatalf("merged cost = %+v", usage)
	}
}

func TestExecutionsAreReproducible(t *testing.T) {
	sessions := []Session{
		{Key: "b", StartedAt: time.UnixMilli(1790942400000), FinishedAt: time.UnixMilli(1790942700000), Usage: []domain.InsightUsage{{Provider: "p", Model: "m", InputTokens: 5, OutputTokens: 1}}},
		{Key: "a", StartedAt: time.UnixMilli(1790812800000), FinishedAt: time.UnixMilli(1790814600000), Usage: []domain.InsightUsage{{Provider: "p", Model: "m", InputTokens: 5, OutputTokens: 1}}},
	}
	first, err := json.Marshal(Executions(AgentCodex, sessions))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(Executions(AgentCodex, sessions))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("payload is not reproducible:\n%s\n%s", first, second)
	}
	executions := Executions(AgentCodex, sessions)
	if executions[0].ID != "codex:a" || executions[1].ID != "codex:b" {
		t.Fatalf("sessions are not sorted: %q %q", executions[0].ID, executions[1].ID)
	}
}

func TestExecutionsBoundLongSessions(t *testing.T) {
	key := strings.Repeat("x", 300)
	session := Session{
		Key:        key,
		StartedAt:  time.UnixMilli(1790812800000),
		FinishedAt: time.UnixMilli(1790812900000),
		Usage:      []domain.InsightUsage{{Provider: "p", Model: "m", InputTokens: 1, OutputTokens: 1}},
	}
	first := Executions(AgentClaude, []Session{session})[0]
	second := Executions(AgentClaude, []Session{session})[0]
	if first.ID != second.ID || len(first.ID) > 256 {
		t.Fatalf("long key is not bounded deterministically: %q (%d bytes)", first.ID, len(first.ID))
	}
	if !strings.HasPrefix(first.ID, "claude:sha256:") {
		t.Fatalf("unexpected long-key form: %q", first.ID)
	}
}

func TestUsageKeepsUnknownCostNullable(t *testing.T) {
	session := Session{
		Key:        "s",
		StartedAt:  time.UnixMilli(1790812800000),
		FinishedAt: time.UnixMilli(1790812900000),
		Usage:      []domain.InsightUsage{{Provider: "p", Model: "m", InputTokens: 10, OutputTokens: 2, CacheReadTokens: 4, CostStatus: "unknown"}},
	}
	usage := Executions(AgentPi, []Session{session})[0].Usage[0]
	if usage.CostUSD != nil || usage.CostStatus != "unknown" {
		t.Fatalf("unknown cost must stay null: %+v", usage)
	}
}

func TestInclusiveInputAddsCache(t *testing.T) {
	if got := inclusiveInput(100, 40, 5); got != 145 {
		t.Fatalf("inclusiveInput = %d, want 145", got)
	}
}
