package agentusage

import (
	"context"
	"testing"
)

func TestPiReaderSumsMessageUsage(t *testing.T) {
	reader := NewPi("testdata/pi")
	if reader.Root() != "testdata/pi/agent/sessions" {
		t.Fatalf("root = %q", reader.Root())
	}
	sessions, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	first := sessions[0]
	if first.Key != "2026-11-21T11-46-47-204Z_01a1fdee" {
		t.Fatalf("key = %q", first.Key)
	}
	if len(first.Usage) != 1 || first.Usage[0].CostUSD != nil {
		t.Fatalf("missing cost must be unknown: %+v", first.Usage)
	}

	second := sessions[1]
	if second.Key != "2026-11-23T03-20-19-302Z_01a2066b" {
		t.Fatalf("key = %q", second.Key)
	}
	if len(second.Usage) != 1 {
		t.Fatalf("want one provider/model entry, got %d", len(second.Usage))
	}
	usage := second.Usage[0]
	if usage.Provider != "minimax-cn" || usage.Model != "MiniMax-M3.1-Flash" {
		t.Fatalf("provider/model = %q/%q", usage.Provider, usage.Model)
	}
	// (1000+100) + (9500+5000+20): pi reports non-cached input separately.
	if usage.InputTokens != 15620 || usage.OutputTokens != 100 {
		t.Fatalf("totals = %+v", usage)
	}
	if usage.CacheReadTokens != 5100 || usage.CacheWriteTokens != 20 {
		t.Fatalf("cache = %+v", usage)
	}
	if usage.CostStatus != "reported" || usage.CostUSD == nil || *usage.CostUSD < 0.029 || *usage.CostUSD > 0.031 {
		t.Fatalf("cost = %+v", usage)
	}
}
