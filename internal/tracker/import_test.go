package tracker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
)

// trackerImportFixture is a store with one owner, one project and one GitHub
// repository, plus the service that imports it.
func trackerImportFixture(t *testing.T, handler http.Handler) (*repo.Store, *Service, domain.Repository) {
	t.Helper()
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.PutUser(domain.User{LoginName: "tracker-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName("tracker-owner")
	if err := st.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitHub", Host: "github.com", Platform: domain.GitPlatformGitHub}, Token: "owner-token"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	repository := domain.Repository{ID: "repo-1", ProjectID: "project-1", Provider: "github", Name: "repo", GongfengProjectID: "github:example/repo", DefaultBranch: "main"}
	if err := st.PutRepository(repository); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	s := NewService()
	s.ConfigureWithStore(domain.SyncConfig{}, st)
	s.githubAPIBase = srv.URL
	return st, s, repository
}

// A GitHub issue becomes one requirement with its number, title, body, state
// and a link back to GitHub. A pull request is not a requirement.
func TestTrackerImportCreatesOneRequirementPerIssueAndSkipsPullRequests(t *testing.T) {
	st, s, _ := trackerImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/repos/example/repo/issues" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[
			{"id":10,"number":2,"title":"First issue","body":"First brief","state":"open","html_url":"https://github.com/example/repo/issues/2"},
			{"id":11,"number":3,"title":"A pull request","body":"Not a requirement","state":"open","html_url":"https://github.com/example/repo/pull/3","pull_request":{}}
		]`))
	}))
	run := s.ImportTracker(st, "github", "")
	if run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	if run.Imported != 1 {
		t.Fatalf("imported %d requirements, want 1", run.Imported)
	}
	issue, ok := st.Issue("gh-repo-1-2")
	if !ok {
		t.Fatal("the issue did not become a requirement")
	}
	if issue.Number != 2 || issue.Title != "First issue" || issue.Description != "First brief" {
		t.Fatalf("imported requirement lost a field: %+v", issue)
	}
	if issue.Status != "todo" {
		t.Fatalf("status %q, want todo", issue.Status)
	}
	if issue.ExternalRef == nil || issue.ExternalRef.ExternalURL != "https://github.com/example/repo/issues/2" || issue.ExternalRef.ExternalState != "open" {
		t.Fatalf("the requirement must link back to GitHub: %+v", issue.ExternalRef)
	}
	if _, ok := st.Issue("gh-repo-1-3"); ok {
		t.Fatal("a pull request must not become a requirement")
	}
}

// A re-import refreshes the tracker fields but leaves the local work alone:
// a description the user edited stays, and a status the user moved stays.
func TestTrackerImportKeepsLocalDescriptionAndUserStatus(t *testing.T) {
	state := "open"
	st, s, _ := trackerImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":10,"number":2,"title":"First issue","body":"Original brief","state":"` + state + `","html_url":"https://github.com/example/repo/issues/2"}]`))
	}))
	if run := s.ImportTracker(st, "github", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	issue, _ := st.Issue("gh-repo-1-2")
	issue.Description = "Locally clarified requirement"
	issue.Status = "done"
	if err := st.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	// The tracker closes the issue while the requirement is done.
	state = "closed"
	if run := s.ImportTracker(st, "github", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	got, _ := st.Issue("gh-repo-1-2")
	if got.Description != "Locally clarified requirement" {
		t.Fatalf("the edited description must survive a re-import: %q", got.Description)
	}
	if got.Status != "done" {
		t.Fatalf("a status the user set must survive a re-import: %q", got.Status)
	}
}

// A requirement nobody touched follows the tracker, so an issue closed on
// GitHub is closed here too.
func TestTrackerImportFollowsTheTrackerForUntouchedRequirements(t *testing.T) {
	state := "open"
	st, s, _ := trackerImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":10,"number":2,"title":"First issue","body":"Original brief","state":"` + state + `","html_url":"https://github.com/example/repo/issues/2"}]`))
	}))
	if run := s.ImportTracker(st, "github", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	state = "closed"
	if run := s.ImportTracker(st, "github", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	got, _ := st.Issue("gh-repo-1-2")
	if got.Status != "closed" {
		t.Fatalf("an issue closed on GitHub must be closed here: %q", got.Status)
	}
	if got.CloseSource != "external" {
		t.Fatalf("close source %q, want external", got.CloseSource)
	}
}

// One broken repository must not stop the others: the run records the failure
// with its reason next to the repository that imported fine.
func TestTrackerImportRecordsEveryRepositoryOutcome(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.PutUser(domain.User{LoginName: "tracker-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName("tracker-owner")
	if err := st.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitHub", Host: "github.com", Platform: domain.GitPlatformGitHub}, Token: "owner-token"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRepository(domain.Repository{ID: "repo-good", ProjectID: "project-1", Provider: "github", Name: "good", GongfengProjectID: "github:example/good", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRepository(domain.Repository{ID: "repo-broken", ProjectID: "project-1", Provider: "github", Name: "broken", GongfengProjectID: "github:example/broken", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/example/broken/issues" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":10,"number":2,"title":"First issue","body":"Brief","state":"open"}]`))
	}))
	t.Cleanup(srv.Close)
	s := NewService()
	s.ConfigureWithStore(domain.SyncConfig{}, st)
	s.githubAPIBase = srv.URL

	run := s.ImportTracker(st, "github", "")
	if run.Status != domain.TrackerImportPartial {
		t.Fatalf("run status %q, want partial (reason %q)", run.Status, run.Reason)
	}
	if run.Imported != 1 || run.Failed != 1 {
		t.Fatalf("run counts %+v", run)
	}
	if len(run.Repositories) != 2 {
		t.Fatalf("the run must name every repository: %+v", run.Repositories)
	}
	var good, broken domain.TrackerImportRepository
	for _, outcome := range run.Repositories {
		if outcome.RepositoryID == "repo-good" {
			good = outcome
		}
		if outcome.RepositoryID == "repo-broken" {
			broken = outcome
		}
	}
	if good.Status != domain.TrackerImportSuccess || good.Imported != 1 {
		t.Fatalf("the healthy repository must import: %+v", good)
	}
	if broken.Status != domain.TrackerImportFailed || broken.Reason == "" {
		t.Fatalf("the broken repository must fail with a reason: %+v", broken)
	}
	if _, ok := st.Issue("gh-repo-good-2"); !ok {
		t.Fatal("the healthy repository must still import")
	}
}

// The run is persisted: the outcome of the last run survives a restart.
func TestTrackerImportRunIsPersisted(t *testing.T) {
	st, s, _ := trackerImportFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":10,"number":2,"title":"First issue","body":"Brief","state":"open"}]`))
	}))
	if run := s.ImportTracker(st, "github", ""); run.Status != domain.TrackerImportSuccess {
		t.Fatalf("run status %q reason %q", run.Status, run.Reason)
	}
	stored, ok := st.LatestTrackerImportRun("github")
	if !ok {
		t.Fatal("the run must be persisted")
	}
	if stored.Imported != 1 || stored.Status != domain.TrackerImportSuccess {
		t.Fatalf("stored run %+v", stored)
	}
	if stored.StartedAt.IsZero() || stored.FinishedAt.IsZero() {
		t.Fatalf("the run must record when it started and finished: %+v", stored)
	}
}
