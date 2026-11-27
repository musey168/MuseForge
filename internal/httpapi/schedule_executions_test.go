package httpapi

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/engine"
	"agentflow/internal/runtime"
	"agentflow/internal/tracker"
)

// A Trace ID pasted into a Studio chat resolves through the read-only
// platform tool, scoped to the conversation's project.
// Execution history exposes prompts' outcomes and errors; it follows the
// same read access as the schedule job itself.
func TestScheduleJobExecutionsFollowJobAccess(t *testing.T) {
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "true")
	t.Setenv("AGENTFLOW_SESSION_SECRET", "schedule-exec-test-secret")
	h, _ := workbenchFixture(t)
	st := h.repo
	for _, login := range []string{"exec-owner", "exec-viewer", "exec-outsider"} {
		if err := st.PutUser(domain.User{LoginName: login, CodeBuddyAPIKey: "key-" + login}); err != nil {
			t.Fatal(err)
		}
	}
	owner, _ := st.UserByLoginName("exec-owner")
	viewer, _ := st.UserByLoginName("exec-viewer")
	outsider, _ := st.UserByLoginName("exec-outsider")
	if err := st.PutProject(domain.Project{ID: "exec-proj", Name: "Exec", OwnerID: owner.ID, }); err != nil {
		t.Fatal(err)
	}
	if err := st.PutScheduleJob(domain.ScheduleJob{ID: "exec-job", ProjectID: "exec-proj", Name: "巡检", SkipDispatch: true, AssistantPrompt: "p", IntervalSec: 3600, OwnerUserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	for _, e := range []domain.ScheduleJobExecution{
		{JobID: "exec-job", ProjectID: "exec-proj", TraceID: "t-ok", Status: "success", StartedAt: started},
		{JobID: "exec-job", ProjectID: "exec-proj", TraceID: "t-bad", Status: "failed", Error: "boom", StartedAt: started.Add(time.Second)},
	} {
		if err := st.PutScheduleJobExecution(e); err != nil {
			t.Fatal(err)
		}
	}
	server := New(st, engine.New(st, runtime.NewManager(filepath.Join(t.TempDir(), "workspaces"))), tracker.NewService(), "", "")

	all := requestJSONAuthed[[]domain.ScheduleJobExecution](t, server, viewer, http.MethodGet, "/api/schedule-job-executions?projectId=exec-proj", nil, http.StatusOK)
	if len(all) != 2 || all[0].TraceID != "t-bad" {
		t.Fatalf("viewer executions = %+v", all)
	}
	failed := requestJSONAuthed[[]domain.ScheduleJobExecution](t, server, owner, http.MethodGet, "/api/schedule-job-executions?jobId=exec-job&status=failed", nil, http.StatusOK)
	if len(failed) != 1 || failed[0].Error != "boom" {
		t.Fatalf("failed executions = %+v", failed)
	}
	if hidden := requestJSONAuthed[[]domain.ScheduleJobExecution](t, server, outsider, http.MethodGet, "/api/schedule-job-executions?traceId=t-bad", nil, http.StatusOK); len(hidden) != 0 {
		t.Fatalf("outsider read executions: %+v", hidden)
	}
	if code := scheduleAuthzStatus(t, server, outsider, http.MethodGet, "/api/schedule-job-executions?jobId=exec-job", nil); code != http.StatusForbidden {
		t.Fatalf("outsider job filter status = %d", code)
	}

	// The dashboard filters, counts and pages on the server.
	dash := requestJSONAuthed[scheduleExecutionDashboard](t, server, viewer, http.MethodGet, "/api/schedule-job-executions/dashboard?projectId=exec-proj&limit=1", nil, http.StatusOK)
	if dash.Total != 2 || len(dash.Items) != 1 || dash.Items[0].TraceID != "t-bad" || dash.Records != 2 || dash.RecentFailed != 1 {
		t.Fatalf("viewer dashboard = %+v", dash)
	}
	if c := dash.Overview.Current; c.Total != 2 || c.Success != 1 || c.Failed != 1 || dash.Overview.Since.IsZero() {
		t.Fatalf("viewer overview = %+v", dash.Overview)
	}
	next := requestJSONAuthed[scheduleExecutionDashboard](t, server, viewer, http.MethodGet, "/api/schedule-job-executions/dashboard?projectId=exec-proj&limit=1&offset=1", nil, http.StatusOK)
	if len(next.Items) != 1 || next.Items[0].TraceID != "t-ok" {
		t.Fatalf("second page = %+v", next.Items)
	}
	searched := requestJSONAuthed[scheduleExecutionDashboard](t, server, owner, http.MethodGet, "/api/schedule-job-executions/dashboard?projectId=exec-proj&jobId=exec-job&q=BOOM", nil, http.StatusOK)
	if searched.Total != 1 || searched.Items[0].TraceID != "t-bad" || searched.Records != 2 {
		t.Fatalf("searched dashboard = %+v", searched)
	}
	later := requestJSONAuthed[scheduleExecutionDashboard](t, server, owner, http.MethodGet, "/api/schedule-job-executions/dashboard?projectId=exec-proj&since="+started.Add(time.Hour).Format(time.RFC3339), nil, http.StatusOK)
	if later.Total != 0 || len(later.Items) != 0 || later.Overview.Current.Total != 2 {
		t.Fatalf("since filters the list, not the overview: %+v", later)
	}
	if hidden := requestJSONAuthed[scheduleExecutionDashboard](t, server, outsider, http.MethodGet, "/api/schedule-job-executions/dashboard?projectId=exec-proj", nil, http.StatusOK); hidden.Total != 0 || hidden.Records != 0 || hidden.Overview.Current.Total != 0 {
		t.Fatalf("outsider read the dashboard: %+v", hidden)
	}
	for path, want := range map[string]int{
		"/api/schedule-job-executions/dashboard?projectId=exec-proj&jobId=exec-job":  http.StatusForbidden,
		"/api/schedule-job-executions/dashboard":                                     http.StatusBadRequest,
		"/api/schedule-job-executions/dashboard?projectId=exec-proj&since=yesterday": http.StatusBadRequest,
	} {
		if code := scheduleAuthzStatus(t, server, outsider, http.MethodGet, path, nil); code != want {
			t.Errorf("%s = %d, want %d", path, code, want)
		}
	}
}

// Executions outlive the conversations a job keeps; the list marks an
// execution whose conversation is gone so the page shows no dead link.
