package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/engine"
	"agentflow/internal/repo"
)

func TestRunnerProjectModesAndLoadPayload(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	project := domain.Project{ID: "runner-modes", Name: "Runner modes", OwnerID: f.user.ID}
	if err := f.s.PutProject(project); err != nil {
		t.Fatal(err)
	}
	machine := domain.Runner{ID: "my-machine", Name: "My machine", OwnerUserID: f.user.ID, Status: domain.RunnerStatusOnline,
		RunnerVersion: domain.RunnerVersionConversationCLI}
	if err := f.s.PutRunner(machine); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetRunnerSharedProjects(machine.ID, f.user.ID, []string{project.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetDefaultRunner(f.user.ID, machine.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.e.ConfigureCloudRunnerPool(engine.CloudRunnerPool{Label: "platform-pool", Projects: []string{project.ID}, Required: true})
	for _, run := range []domain.Run{
		{ID: "platform-work", Status: "running", ExecutionContext: domain.TelemetryContext{ProjectID: "other"}},
		{ID: "personal-work", Status: "running", ExecutionContext: domain.TelemetryContext{ProjectID: project.ID, ExecutionRunnerID: machine.ID}},
	} {
		if err := f.s.PutRun(run); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/runners/default?projectId=" + project.ID
	for _, tc := range []struct {
		body map[string]string
		path string
	}{
		{map[string]string{"mode": "auto"}, "/api/runners/default"},
		{map[string]string{"mode": "other"}, path},
		{map[string]string{"mode": "platform", "runnerId": machine.ID}, path},
	} {
		if rec := cvBRequest(t, f, http.MethodPut, tc.path, tc.body, true); rec.Code != 400 {
			t.Fatalf("invalid mode %v: got %d %s", tc.body, rec.Code, rec.Body)
		}
	}
	for _, tc := range []struct {
		mode, selected, effective, projectDefault string
	}{
		{"auto", "", "", ""},
		{"platform", "", "", ""},
		{"", machine.ID, machine.ID, machine.ID},
		{"", "", machine.ID, ""},
	} {
		rec := cvBRequest(t, f, http.MethodPut, path, map[string]string{"mode": tc.mode, "runnerId": tc.selected}, true)
		var updated map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil || rec.Code != 200 ||
			updated["projectRunnerMode"] != tc.mode || updated["runnerId"] != tc.effective || updated["message"] == "" {
			t.Fatalf("updating mode %q: %d %s", tc.mode, rec.Code, rec.Body)
		}
		for _, url := range []string{path, path + "&conversation=1"} {
			rec := cvBRequest(t, f, http.MethodGet, url, nil, true)
			var got struct {
				RunnerID, PersonalRunnerID, ProjectRunnerID, ProjectRunnerMode, MinRunnerVersion string
				Runner                                                                           domain.Runner
				Online                                                                           bool
				CloudPool                                                                        struct {
					Configured bool
					Online     int
				}
				PlatformRunner domain.PlatformRunnerStatus
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 ||
				got.RunnerID != tc.effective || got.Runner.ID != tc.effective || got.Online != (tc.effective != "") ||
				got.PersonalRunnerID != machine.ID || got.ProjectRunnerID != tc.projectDefault || got.ProjectRunnerMode != tc.mode ||
				got.MinRunnerVersion != domain.RunnerVersionDefaultSelection || !got.CloudPool.Configured || got.CloudPool.Online != 0 ||
				got.PlatformRunner.RunLimit != 10 || got.PlatformRunner.Name != domain.PlatformRunnerName || got.PlatformRunner.CanEditLimit ||
				got.PlatformRunner.AllowPrivilegedSandbox ||
				got.PlatformRunner.Status != domain.RunnerStatusOffline || got.PlatformRunner.Load == nil || got.PlatformRunner.Load.RunningRuns != 1 {
				t.Fatalf("%s after %q: %d %s", url, tc.mode, rec.Code, rec.Body)
			}
		}
	}
	for _, path := range []string{"/api/runners?mine=1", "/api/runners?projectId=" + project.ID} {
		rec := cvBRequest(t, f, http.MethodGet, path, nil, true)
		var got []repo.RunnerProjectView
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 ||
			len(got) != 1 || got[0].ID != machine.ID || got[0].Load == nil ||
			*got[0].Load != (domain.RunnerLoad{RunningRuns: 1, OccupiedRuns: 1}) {
			t.Fatalf("runner load response: %d %s", rec.Code, rec.Body)
		}
	}
}

func TestPlatformRunnerRunLimitWriteGuardAndPromotion(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	member := domain.User{ID: 95, LoginName: "limit-member"}
	if err := f.s.PutUser(member); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.GrantPlatformAdmin(f.user.LoginName); err != nil {
		t.Fatal(err)
	}
	const path = "/api/runners/platform/run-limit"
	put := func(user domain.User, origin, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "http://localhost"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = authedRequest(t, req, user)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("write from %q with %s: got %d, want %d, %s", origin, body, rec.Code, want, rec.Body)
		}
		return rec
	}
	const origin = "http://localhost"
	put(member, origin, `{"runLimit":5}`, 403)
	put(f.user, "https://other.example", `{"runLimit":5}`, 403)
	put(f.user, "", `{"runLimit":5}`, 403)
	for _, body := range []string{`{}`, `{"runLimit":null}`, `{"runLimit":-1}`, `{"runLimit":1.5}`} {
		put(f.user, origin, body, 400)
	}
	if got := f.s.PlatformRunnerRunLimit(); got != 10 {
		t.Fatalf("invalid writes changed the limit to %d", got)
	}
	put(f.user, origin, `{"runLimit":1}`, 200)
	run := domain.Run{ID: "claim-loop-marker", Status: "running", WorkflowID: "approval", CreatedAt: time.Now().UTC(),
		WorkflowSnapshot: domain.WorkflowDefinition{ID: "approval", SandboxProfileID: repo.TestNativeSandboxID,
			Nodes: []domain.WorkflowNode{{ID: "gate", Type: "approval", Name: "Gate"}}},
		Nodes: map[string]*domain.NodeRun{"gate": {NodeID: "gate", Status: "pending"}}}
	if err := f.s.PutRun(run); err != nil {
		t.Fatal(err)
	}
	run.ID = "queued-platform"
	if err := f.s.PutRun(run); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = f.e.Drain(context.Background())
	})
	f.e.StartClaimLoop(ctx)
	waitFor := func(timeout time.Duration, condition func() bool, message string) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal(message)
	}
	waitFor(time.Second, func() bool {
		marker, _ := f.s.RunState("claim-loop-marker")
		return marker.Status == "waiting"
	}, "initial claim pass did not run")
	if queued, _ := f.s.RunState(run.ID); queued.Status != "queued" {
		t.Fatalf("queue exceeded limit 1: %s", queued.Status)
	}
	rec := put(f.user, origin, `{"runLimit":0}`, 200)
	var got domain.PlatformRunnerStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.RunLimit != 0 || !got.CanEditLimit ||
		got.Load == nil || got.Status != domain.RunnerStatusOnline || !strings.Contains(got.Name, "平台执行机器") {
		t.Fatalf("updated platform status: %s", rec.Body)
	}
	waitFor(1500*time.Millisecond, func() bool {
		promoted, _ := f.s.RunState(run.ID)
		return promoted.AdmissionState == "held" && promoted.Status != "queued"
	}, "clearing the limit did not wake the claim loop before its normal tick")
}

func TestConversationRunnerDefaultSkipsMachinesThatOnlySupportSOPs(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	project := "runner-compatibility"
	if err := f.s.PutProject(domain.Project{ID: project, Name: "Runner compatibility", OwnerID: f.user.ID}); err != nil {
		t.Fatal(err)
	}
	for _, runner := range []domain.Runner{
		{ID: "sop-only", OwnerUserID: f.user.ID, Status: domain.RunnerStatusOnline, RunnerVersion: "0.9.2"},
		{ID: "chat-ready", OwnerUserID: f.user.ID, Status: domain.RunnerStatusOnline, RunnerVersion: domain.RunnerVersionConversationCLI},
	} {
		if err := f.s.PutRunner(runner); err != nil {
			t.Fatal(err)
		}
		if err := f.s.SetRunnerSharedProjects(runner.ID, f.user.ID, []string{project}); err != nil {
			t.Fatal(err)
		}
	}
	putDefault := func(path, id string) {
		t.Helper()
		rec := cvBRequest(t, f, http.MethodPut, path, map[string]string{"runnerId": id}, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("set %s: %d %s", id, rec.Code, rec.Body.String())
		}
	}
	path := "/api/runners/default?projectId=" + project
	check := func(path, expected string) {
		t.Helper()
		var row struct{ RunnerID string }
		rec := cvBRequest(t, f, http.MethodGet, path, nil, true)
		if err := json.Unmarshal(rec.Body.Bytes(), &row); err != nil || rec.Code != http.StatusOK || row.RunnerID != expected {
			t.Errorf("%s: got %d %s, want runner %q", path, rec.Code, rec.Body.String(), expected)
		}
	}
	putDefault(path, "sop-only")
	putDefault("/api/runners/default", "chat-ready")
	check(path, "sop-only")
	check(path+"&conversation=1", "chat-ready")
	rec := cvBRequest(t, f, http.MethodGet, "/api/runners?projectId="+project, nil, true)
	var rows []struct {
		ID                   string
		SupportsConversation *bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || rec.Code != http.StatusOK || len(rows) != 2 {
		t.Fatalf("runner list: %d %s", rec.Code, rec.Body.String())
	}
	for _, row := range rows {
		if row.SupportsConversation == nil || *row.SupportsConversation != (row.ID == "chat-ready") {
			t.Errorf("wrong chat capability in runner list: %s", rec.Body.String())
		}
	}
	if err := f.s.SetRunnerSharedProjects("chat-ready", f.user.ID, nil); err != nil {
		t.Fatal(err)
	}
	check(path, "sop-only")
	check(path+"&conversation=1", "chat-ready")
	putDefault("/api/runners/default", "sop-only")
	check("/api/runners/default?conversation=1", "")
}
