package agentusage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newOMPDatabase replays the trimmed stats.sql fixture into a temp database.
func newOMPDatabase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	script, err := os.ReadFile("testdata/omp/stats.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(script), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("apply fixture statement: %v", err)
		}
	}
	return dir
}

func TestOMPReaderAggregatesSessions(t *testing.T) {
	sessions, err := NewOMP(newOMPDatabase(t)).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(sessions))
	}
	first := sessions[0]
	if first.Key != "2026-10-01T00-00-00-000Z_aaaa" {
		t.Fatalf("session key must be the file basename: %q", first.Key)
	}
	if first.Status != "" && first.Status != "succeeded" {
		t.Fatalf("status = %q", first.Status)
	}
	if first.ExecutionDurationMs == nil || *first.ExecutionDurationMs != 3000 {
		t.Fatalf("duration = %v, want 3000", first.ExecutionDurationMs)
	}
	if len(first.Usage) != 2 {
		t.Fatalf("want two provider/model entries, got %d", len(first.Usage))
	}
	type entry struct {
		input, output, cacheRead, cacheWrite int64
		cost                                 float64
		status                               string
	}
	byModel := map[string]entry{}
	for _, item := range first.Usage {
		cost := 0.0
		if item.CostUSD != nil {
			cost = *item.CostUSD
		}
		byModel[item.Model] = entry{item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens, cost, item.CostStatus}
	}
	minimax := byModel["MiniMax-M3.1-Flash"]
	if minimax.input != 400 || minimax.output != 30 || minimax.cacheRead != 100 {
		t.Fatalf("minimax must include cache in input: %+v", minimax)
	}
	if minimax.status != "reported" || minimax.cost < 0.029 || minimax.cost > 0.031 {
		t.Fatalf("minimax cost = %+v", minimax)
	}
	deepseek := byModel["deepseek-flash"]
	if deepseek.input != 55 || deepseek.cacheWrite != 5 {
		t.Fatalf("cache write must be included in input: %+v", deepseek)
	}

	cancelled := sessions[1]
	if cancelled.Status != "cancelled" {
		t.Fatalf("aborted session status = %q", cancelled.Status)
	}
	if len(cancelled.Usage) != 1 || cancelled.Usage[0].CostUSD != nil || cancelled.Usage[0].CostStatus != "unknown" {
		t.Fatalf("unpriced zero cost must be unknown: %+v", cancelled.Usage)
	}
	if sessions[2].Status != "failed" {
		t.Fatalf("errored session status = %q", sessions[2].Status)
	}

	executions := Executions(AgentOMP, sessions)
	for _, execution := range executions {
		if !strings.HasPrefix(execution.ID, "omp:") || execution.Workflow.ID != "omp" || execution.Workflow.Name != "omp" {
			t.Fatalf("unexpected omp identity: %+v", execution)
		}
	}
}

func TestOMPReaderWithoutDatabase(t *testing.T) {
	sessions, err := NewOMP(t.TempDir()).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("want no sessions, got %d", len(sessions))
	}
}
