package repo

import (
	"testing"
	"time"

	"agentflow/internal/domain"
)

// The analytics projections read payload JSON through sqlPayloadJSONText; every
// projected column must round-trip from the stored issue payload.
func TestIssueAnalyticsViewsProjectsPayloadJSONColumns(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	started := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	issue := domain.Issue{
		ID: "iss-projection", ProjectID: "alpha", Title: "projection", Source: "tapd", Kind: "requirement",
		TapdID: "1001", DeliveryState: "merged", DeliveryMRURL: "https://git.example.com/mr/7",
		DeliveryMRState: "opened", DeliveryMRs: []domain.DeliveryMR{{URL: "https://github.com/example/frontend/pull/7", State: "merged"}, {URL: "https://github.com/example/backend/pull/8", State: "opened"}},
		ExternalRef:   &domain.ExternalRef{ExternalID: "2002", ExternalURL: "https://tapd.example.com/story/2002"},
		Clarification: domain.ClarificationAssessment{StartedAt: started},
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	views := s.IssueAnalyticsViews()
	if len(views) != 1 {
		t.Fatalf("views = %+v", views)
	}
	got := views[0]
	if got.ExternalRef == nil || got.ExternalRef.ExternalURL != "https://tapd.example.com/story/2002" || got.ExternalRef.ExternalID != "2002" {
		t.Fatalf("external ref = %+v", got.ExternalRef)
	}
	if got.TapdID != "1001" || got.DeliveryState != "merged" || got.DeliveryMRURL != "https://git.example.com/mr/7" {
		t.Fatalf("payload columns = %+v", got)
	}
	if !got.Clarification.StartedAt.Equal(started) {
		t.Fatalf("clarification startedAt = %v, want %v", got.Clarification.StartedAt, started)
	}
	if got.DeliveryMRState != "opened" || len(got.DeliveryMRs) != 2 || got.DeliveryMRs[1].State != "opened" {
		t.Fatalf("necessary MR projection lost pending delivery: %+v", got)
	}
}

// The lifecycle evidence of an issue lives in its stored payload. The
// analytics projection reads the clarification timestamp out of that payload
// through sqlPayloadJSONText, and the full issue read returns the remaining
// lifecycle columns.
func TestIssueLifecyclePayloadColumnsRoundTrip(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	started := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	issue := domain.Issue{
		ID: "iss-lifecycle", ProjectID: "alpha", Title: "lifecycle", Status: "open", Kind: "requirement", Source: "platform",
		Clarification: domain.ClarificationAssessment{StartedAt: started},
		Operations:    []domain.IssueOperation{{At: started, Action: "create"}, {At: started.Add(time.Hour), Action: "close"}},
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	// CompletedAt is stamped by PutIssue on the transition to done.
	issue.Status = "done"
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	stored, ok := s.Issue(issue.ID)
	if !ok || stored.CompletedAt.IsZero() {
		t.Fatalf("stored completion = %+v (found=%v)", stored.CompletedAt, ok)
	}
	if !stored.Clarification.StartedAt.Equal(started) {
		t.Fatalf("stored clarification startedAt = %v, want %v", stored.Clarification.StartedAt, started)
	}
	if len(stored.Operations) < 2 || stored.Operations[0].Action != "create" || stored.Operations[len(stored.Operations)-1].Action != "close" {
		t.Fatalf("stored operations = %+v", stored.Operations)
	}
	view := domain.Issue{}
	for _, candidate := range s.IssueAnalyticsViews() {
		if candidate.ID == issue.ID {
			view = candidate
		}
	}
	if view.ID == "" {
		t.Fatalf("issue %s missing from the analytics projection", issue.ID)
	}
	if !view.Clarification.StartedAt.Equal(started) {
		t.Fatalf("projected clarification startedAt = %v, want %v", view.Clarification.StartedAt, started)
	}
}
