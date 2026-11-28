package agentusage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"agentflow/internal/domain"
)

// fakeIngest speaks the platform's ingest contract: it accepts a valid
// execution once and answers duplicate for the identical payload.
type fakeIngest struct {
	mu        sync.Mutex
	seen      map[string]bool
	validated int
}

func (f *fakeIngest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/api/insights/v1/ingest" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"需要数据源 Bearer Token"}`))
		return
	}
	body, _ := io.ReadAll(r.Body)
	var execution domain.InsightExecution
	if err := json.Unmarshal(body, &execution); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid json"}`))
		return
	}
	if reason := checkExecution(execution, time.Now().UTC()); reason != "" {
		f.mu.Lock()
		f.validated++
		f.mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": reason})
		return
	}
	sum := sha256.Sum256(body)
	key := hex.EncodeToString(sum[:])
	f.mu.Lock()
	duplicate := f.seen[key]
	f.seen[key] = true
	f.mu.Unlock()
	if duplicate {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"duplicate"}`))
		return
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// checkExecution mirrors validateInsightExecution for the fields the
// connectors produce.
func checkExecution(v domain.InsightExecution, now time.Time) string {
	if v.Type != "workflow_execution" {
		return "type 必须为 workflow_execution"
	}
	for _, value := range []string{v.ID, v.ExternalExecutionID, v.Workflow.ID, v.Workflow.Name} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return "record id must be non-empty and <= 256 bytes"
		}
	}
	if v.Status != "succeeded" && v.Status != "failed" && v.Status != "cancelled" {
		return "only terminal executions are accepted"
	}
	if v.StartedAt.Year() < 2000 || v.FinishedAt.Before(v.StartedAt) || v.FinishedAt.After(now.Add(5*time.Minute)) {
		return "invalid timestamps"
	}
	if len(v.Usage) == 0 || len(v.Usage) > 64 {
		return "usage must have 1..64 entries"
	}
	seen := map[string]bool{}
	for _, u := range v.Usage {
		if strings.TrimSpace(u.Provider) == "" || strings.TrimSpace(u.Model) == "" || len(u.Provider) > 128 || len(u.Model) > 128 {
			return "usage entry needs provider and model"
		}
		key := u.Provider + "\x00" + u.Model
		if seen[key] {
			return "duplicate provider/model"
		}
		seen[key] = true
		for _, n := range []int64{u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.ReasoningTokens} {
			if n < 0 || n > 1e12 {
				return "token counts out of range"
			}
		}
		if u.CacheReadTokens+u.CacheWriteTokens > u.InputTokens {
			return "cache tokens must be included in input"
		}
		if u.ReasoningTokens > u.OutputTokens {
			return "reasoning tokens must be included in output"
		}
		switch u.CostStatus {
		case "unknown":
			if u.CostUSD != nil {
				return "unknown cost must be null"
			}
		case "reported", "estimated":
			if u.CostUSD == nil || *u.CostUSD < 0 {
				return "known cost must be a non-negative number"
			}
		default:
			return "invalid costStatus"
		}
	}
	return ""
}

// fixtureExecutions reads every adapter fixture through the real readers.
func fixtureExecutions(t *testing.T) []domain.InsightExecution {
	t.Helper()
	var executions []domain.InsightExecution
	for _, reader := range []Reader{
		NewOMP(newOMPDatabase(t)),
		NewCodex("testdata/codex"),
		NewPi("testdata/pi"),
		NewClaude("testdata/claude"),
	} {
		sessions, err := reader.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		executions = append(executions, Executions(reader.Agent(), sessions)...)
	}
	if len(executions) != 9 {
		t.Fatalf("want 9 executions from the fixtures, got %d", len(executions))
	}
	return executions
}

func TestClientPostsValidExecutions(t *testing.T) {
	fake := &fakeIngest{seen: map[string]bool{}}
	server := httptest.NewServer(fake)
	defer server.Close()
	client := NewClient(server.URL, "test-token")

	executions := fixtureExecutions(t)
	for _, execution := range executions {
		result, err := client.Ingest(context.Background(), execution)
		if err != nil {
			t.Fatalf("%s: %v", execution.ExternalExecutionID, err)
		}
		if result != ResultAccepted {
			t.Fatalf("%s: first post = %q", execution.ExternalExecutionID, result)
		}
	}
	if fake.validated != 0 {
		t.Fatalf("the fake server refused %d executions", fake.validated)
	}
	for _, execution := range executions {
		result, err := client.Ingest(context.Background(), execution)
		if err != nil {
			t.Fatalf("%s: %v", execution.ExternalExecutionID, err)
		}
		if result != ResultDuplicate {
			t.Fatalf("%s: second post = %q", execution.ExternalExecutionID, result)
		}
	}
}

func TestClientSurfacesServerError(t *testing.T) {
	fake := &fakeIngest{seen: map[string]bool{}}
	server := httptest.NewServer(fake)
	defer server.Close()
	client := NewClient(server.URL, "test-token")

	broken := domain.InsightExecution{
		ID:                  "omp:broken",
		Type:                "workflow_execution",
		ExternalExecutionID: "omp:broken",
		Workflow:            domain.InsightWorkflow{ID: "omp", Name: "omp"},
		StartedAt:           time.Now().Add(-time.Hour),
		FinishedAt:          time.Now().Add(-time.Minute),
		Status:              "succeeded",
		Usage: []domain.InsightUsage{{
			Provider: "p", Model: "m",
			InputTokens: 1, CacheReadTokens: 5,
			CostStatus: "unknown",
		}},
	}
	_, err := client.Ingest(context.Background(), broken)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || !apiErr.Fatal() {
		t.Fatalf("status = %d, fatal = %v", apiErr.StatusCode, apiErr.Fatal())
	}
	if !strings.Contains(apiErr.Message, "cache") {
		t.Fatalf("server message lost: %q", apiErr.Message)
	}
}

func TestClientClassifiesAuthAndConflict(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusConflict} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"拒绝"}`))
		}))
		client := NewClient(server.URL, "t")
		execution := fixtureExecutions(t)[0]
		_, err := client.Ingest(context.Background(), execution)
		server.Close()
		apiErr, ok := err.(*APIError)
		if !ok {
			t.Fatalf("%d: want *APIError, got %v", status, err)
		}
		if apiErr.Conflict() != (status == http.StatusConflict) {
			t.Fatalf("%d: conflict = %v", status, apiErr.Conflict())
		}
		if apiErr.Fatal() != (status == http.StatusUnauthorized) {
			t.Fatalf("%d: fatal = %v", status, apiErr.Fatal())
		}
	}
}
