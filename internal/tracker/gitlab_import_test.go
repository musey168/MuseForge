package tracker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
)

// gitlabImportFixture is a store with one owner, one project and one GitLab
// repository, plus the service that imports it.
func gitlabImportFixture(t *testing.T, handler http.Handler) (*repo.Store, *Service, domain.Repository) {
	t.Helper()
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.PutUser(domain.User{LoginName: "gitlab-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName("gitlab-owner")
	if err := st.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitLab", Host: "gitlab.com", Platform: domain.GitPlatformGitLab}, Token: "owner-token"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	repository := domain.Repository{ID: "repo-gl", ProjectID: "project-1", Provider: "gitlab", Name: "gl", GongfengProjectID: "gitlab:gitlab.com/example/repo", DefaultBranch: "main"}
	if err := st.PutRepository(repository); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	s := NewService()
	s.ConfigureWithStore(domain.SyncConfig{}, st)
	s.gitlabAPIBase = srv.URL
	return st, s, repository
}

// A GitLab issue becomes one requirement with its number, title, body, state,
// kind and a link back to the GitLab issue. GitLab merge requests are not
// issues, so the listing never yields one.
func TestGitLabImportCreatesARequirementPerIssue(t *testing.T) {
	st, s, _ := gitlabImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/projects/example%2Frepo/issues" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "owner-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":50,"iid":1,"title":"Opened bug","description":"Bug brief","state":"opened","issue_type":"bug","web_url":"https://gitlab.com/example/repo/-/issues/1","created_at":"2026-11-01T08:00:00Z"},
			{"id":51,"iid":2,"title":"Closed task","description":"Task brief","state":"closed","issue_type":"task","web_url":"https://gitlab.com/example/repo/-/issues/2","created_at":"2026-11-02T08:00:00Z"},
			{"id":52,"iid":3,"title":"An incident","description":"Incident brief","state":"opened","issue_type":"incident","web_url":"https://gitlab.com/example/repo/-/issues/3","created_at":"2026-11-03T08:00:00Z"},
			{"id":53,"iid":4,"title":"A test case","description":"Test brief","state":"opened","issue_type":"test_case","web_url":"https://gitlab.com/example/repo/-/issues/4","created_at":"2026-11-04T08:00:00Z"}
		]`))
	}))
	run := s.ImportTracker(st, "gitlab", "")
	if run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	if run.Imported != 4 {
		t.Fatalf("imported %d requirements, want 4", run.Imported)
	}
	bug, ok := st.Issue("gl-repo-gl-1")
	if !ok {
		t.Fatal("the opened issue did not become a requirement")
	}
	if bug.Number != 1 || bug.Title != "Opened bug" || bug.Description != "Bug brief" {
		t.Fatalf("imported requirement lost a field: %+v", bug)
	}
	if bug.Status != "todo" || bug.Kind != "bug" {
		t.Fatalf("opened bug mapped to status %q kind %q, want todo/bug", bug.Status, bug.Kind)
	}
	if bug.ExternalRef == nil || bug.ExternalRef.Source != "gitlab" || bug.ExternalRef.ExternalURL != "https://gitlab.com/example/repo/-/issues/1" || bug.ExternalRef.ExternalState != "opened" {
		t.Fatalf("the requirement must link back to GitLab: %+v", bug.ExternalRef)
	}
	closed, ok := st.Issue("gl-repo-gl-2")
	if !ok {
		t.Fatal("the closed issue did not become a requirement")
	}
	if closed.Status != "closed" || closed.CloseSource != "external" || closed.Kind != "task" {
		t.Fatalf("closed task mapped to status %q close %q kind %q, want closed/external/task", closed.Status, closed.CloseSource, closed.Kind)
	}
	for id, want := range map[string]string{"gl-repo-gl-3": "issue", "gl-repo-gl-4": "issue"} {
		issue, ok := st.Issue(id)
		if !ok {
			t.Fatalf("%s did not become a requirement", id)
		}
		if issue.Kind != want {
			t.Fatalf("%s kind %q, want %q", id, issue.Kind, want)
		}
	}
}

// A re-import refreshes the tracker fields but leaves the local work alone:
// a description the user edited stays, and a status the user moved stays.
func TestGitLabImportKeepsLocalDescriptionAndUserStatus(t *testing.T) {
	state := "opened"
	title := "First issue"
	st, s, _ := gitlabImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":50,"iid":2,"title":"` + title + `","description":"Original brief","state":"` + state + `","issue_type":"issue","web_url":"https://gitlab.com/example/repo/-/issues/2"}]`))
	}))
	if run := s.ImportTracker(st, "gitlab", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	issue, _ := st.Issue("gl-repo-gl-2")
	issue.Description = "Locally clarified requirement"
	issue.Status = "done"
	if err := st.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	// The tracker closes and renames the issue while the requirement is done.
	state = "closed"
	title = "Renamed issue"
	if run := s.ImportTracker(st, "gitlab", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	got, _ := st.Issue("gl-repo-gl-2")
	if got.Title != "Renamed issue" {
		t.Fatalf("the tracker title must be refreshed: %q", got.Title)
	}
	if got.ExternalRef == nil || got.ExternalRef.ExternalState != "closed" {
		t.Fatalf("the tracker state must be refreshed: %+v", got.ExternalRef)
	}
	if got.Description != "Locally clarified requirement" {
		t.Fatalf("the edited description must survive a re-import: %q", got.Description)
	}
	if got.Status != "done" {
		t.Fatalf("a status the user set must survive a re-import: %q", got.Status)
	}
}

// A GitLab project whose credential is missing or invalid fails with a
// reason while the other projects still import.
func TestGitLabImportRecordsEveryProjectOutcome(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.PutUser(domain.User{LoginName: "gitlab-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName("gitlab-owner")
	if err := st.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitLab", Host: "gitlab.com", Platform: domain.GitPlatformGitLab}, Token: "owner-token"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutUser(domain.User{LoginName: "no-gitlab-owner"}); err != nil {
		t.Fatal(err)
	}
	bare, _ := st.UserByLoginName("no-gitlab-owner")
	if err := st.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutProject(domain.Project{ID: "project-2", Name: "Bare Project", OwnerID: bare.ID}); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []domain.Repository{
		{ID: "repo-good", ProjectID: "project-1", Provider: "gitlab", Name: "good", GongfengProjectID: "gitlab:gitlab.com/example/good", DefaultBranch: "main"},
		{ID: "repo-bad", ProjectID: "project-1", Provider: "gitlab", Name: "bad", GongfengProjectID: "gitlab:gitlab.com/example/bad", DefaultBranch: "main"},
		{ID: "repo-missing", ProjectID: "project-2", Provider: "gitlab", Name: "missing", GongfengProjectID: "gitlab:gitlab.com/example/missing", DefaultBranch: "main"},
	} {
		if err := st.PutRepository(repository); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/projects/example%2Fbad/issues":
			// An invalid token is the only credential GitLab rejects.
			w.WriteHeader(http.StatusUnauthorized)
		case "/projects/example%2Fgood/issues":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":70,"iid":7,"title":"Healthy issue","description":"Brief","state":"opened","issue_type":"issue","web_url":"https://gitlab.com/example/good/-/issues/7"}]`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	s := NewService()
	s.ConfigureWithStore(domain.SyncConfig{}, st)
	s.gitlabAPIBase = srv.URL

	run := s.ImportTracker(st, "gitlab", "")
	if run.Status != domain.TrackerImportPartial {
		t.Fatalf("run status %q, want partial (reason %q)", run.Status, run.Reason)
	}
	if run.Imported != 1 || run.Failed != 2 {
		t.Fatalf("run counts %+v", run)
	}
	if len(run.Repositories) != 3 {
		t.Fatalf("the run must name every repository: %+v", run.Repositories)
	}
	outcomes := map[string]domain.TrackerImportRepository{}
	for _, outcome := range run.Repositories {
		outcomes[outcome.RepositoryID] = outcome
	}
	if good := outcomes["repo-good"]; good.Status != domain.TrackerImportSuccess || good.Imported != 1 {
		t.Fatalf("the healthy project must import: %+v", good)
	}
	if bad := outcomes["repo-bad"]; bad.Status != domain.TrackerImportFailed || !strings.Contains(bad.Reason, "401") {
		t.Fatalf("an invalid credential must fail with its HTTP status: %+v", bad)
	}
	if missing := outcomes["repo-missing"]; missing.Status != domain.TrackerImportFailed || !strings.Contains(missing.Reason, "凭据缺失") {
		t.Fatalf("a missing credential must fail with a reason: %+v", missing)
	}
	if _, ok := st.Issue("gl-repo-good-7"); !ok {
		t.Fatal("the healthy project must still import")
	}
}
