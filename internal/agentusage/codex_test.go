package agentusage

import (
	"context"
	"strings"
	"testing"
)

func TestCodexReaderReadsRollouts(t *testing.T) {
	reader := NewCodex("testdata/codex")
	if reader.Root() != "testdata/codex/sessions" {
		t.Fatalf("root = %q", reader.Root())
	}
	sessions, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 rollouts, got %d", len(sessions))
	}
	first := sessions[0]
	if !strings.HasPrefix(first.Key, "rollout-2026-11-19T12-18-26-01a1f206-") || len(first.Key) != len("rollout-2026-11-19T12-18-26-01a1f206")+9 {
		t.Fatalf("key = %q: the name plus an 8-hex path hash", first.Key)
	}
	if len(first.Usage) != 1 {
		t.Fatalf("want one usage entry, got %d", len(first.Usage))
	}
	usage := first.Usage[0]
	// The damaged token_count line must not hide the last complete record.
	if usage.Provider != "minimax-cn" || usage.Model != "MiniMax-M2.5" {
		t.Fatalf("provider/model = %q/%q", usage.Provider, usage.Model)
	}
	if usage.InputTokens != 20544 || usage.OutputTokens != 54 {
		t.Fatalf("totals = %+v", usage)
	}
	if usage.CacheReadTokens != 10414 || usage.CacheWriteTokens != 0 {
		t.Fatalf("cache = %+v", usage)
	}
	if usage.CostUSD != nil || usage.CostStatus != "unknown" {
		t.Fatalf("codex has no cost: %+v", usage)
	}
	if first.StartedAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-11-19T04:18:26Z" {
		t.Fatalf("startedAt = %s", first.StartedAt)
	}
	if first.FinishedAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-11-19T04:18:31Z" {
		t.Fatalf("finishedAt = %s", first.FinishedAt)
	}

	second := sessions[1]
	usage = second.Usage[0]
	if usage.Provider != "openai" || usage.Model != "unknown" {
		t.Fatalf("missing model must fall back to unknown: %q/%q", usage.Provider, usage.Model)
	}
	if usage.InputTokens != 1000 || usage.CacheWriteTokens != 10 || usage.ReasoningTokens != 5 {
		t.Fatalf("second rollout totals = %+v", usage)
	}
}
