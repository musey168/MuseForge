package agentusage

import (
	"context"
	"strings"
	"testing"
)

func TestClaudeReaderToleratesTranscript(t *testing.T) {
	reader := NewClaude("testdata/claude")
	if reader.Root() != "testdata/claude/projects" {
		t.Fatalf("root = %q", reader.Root())
	}
	sessions, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	byKey := map[string]Session{}
	for _, session := range sessions {
		byKey[session.Key] = session
	}
	main, ok := sessionByKeyPrefix(sessions, "01a20700-0000-7000-8000-000000000001")
	if !ok {
		t.Fatalf("transcript with usage missing: %v", byKey)
	}
	usage := main.Usage[0]
	if usage.Provider != "anthropic" || usage.Model != "claude-sonnet-4-5" {
		t.Fatalf("provider/model = %q/%q", usage.Provider, usage.Model)
	}
	// (12+300+400) + (6+0+100): cache tokens are subsets of input.
	if usage.InputTokens != 818 || usage.OutputTokens != 33 {
		t.Fatalf("totals = %+v", usage)
	}
	if usage.CacheReadTokens != 500 || usage.CacheWriteTokens != 300 {
		t.Fatalf("cache = %+v", usage)
	}
	if usage.CostUSD != nil || usage.CostStatus != "unknown" {
		t.Fatalf("claude reports no cost: %+v", usage)
	}

	nameless, ok := sessionByKeyPrefix(sessions, "01a20700-0000-7000-8000-000000000002")
	if !ok {
		t.Fatalf("second transcript missing: %v", byKey)
	}
	if nameless.Usage[0].Model != "unknown" || nameless.Usage[0].InputTokens != 5 {
		t.Fatalf("missing model must fall back to unknown: %+v", nameless.Usage[0])
	}
}

// sessionByKeyPrefix finds a session by its name; the key carries a hash of
// the full path after it.
func sessionByKeyPrefix(sessions []Session, prefix string) (Session, bool) {
	for _, session := range sessions {
		if strings.HasPrefix(session.Key, prefix+"-") {
			return session, true
		}
	}
	return Session{}, false
}

func TestClaudeReaderWithoutData(t *testing.T) {
	sessions, err := NewClaude(t.TempDir()).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("want no sessions, got %d", len(sessions))
	}
}
