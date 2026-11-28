package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// trackerImportResponse is the answer of the tracker import endpoints.
type trackerImportResponse struct {
	Run *domain.TrackerImportRun `json:"run"`
}

// The import can be triggered on demand, its outcome is visible, and a
// repository whose credential is broken fails with a reason while the other
// repository still imports.
func TestTrackerImportRunsOnDemandAndReportsTheLastRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/example/broken/issues" {
			// GitHub hides a repository the credential cannot read.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":10,"number":2,"title":"First issue","body":"First brief","state":"open","html_url":"https://github.com/example/good/issues/2"},
			{"id":11,"number":3,"title":"A pull request","body":"Not a requirement","state":"open","html_url":"https://github.com/example/good/pull/3","pull_request":{}}
		]`))
	}))
	defer srv.Close()
	t.Setenv("AGENTFLOW_GITHUB_API_BASE", srv.URL)

	root := t.TempDir()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{LoginName: "tracker-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := s.UserByLoginName("tracker-owner")
	if err := s.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitHub", Host: "github.com", Platform: domain.GitPlatformGitHub}, Token: "owner-token"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRepository(domain.Repository{ID: "repo-good", ProjectID: "project-1", Provider: "github", Name: "good", GongfengProjectID: "github:example/good", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRepository(domain.Repository{ID: "repo-broken", ProjectID: "project-1", Provider: "github", Name: "broken", GongfengProjectID: "github:example/broken", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	h := New(s, tracker.NewService(), "", "")

	// Before the first run there is nothing to show.
	empty := requestJSON[trackerImportResponse](t, h, http.MethodGet, "/api/tracker/import?provider=github", nil, http.StatusOK)
	if empty.Run != nil {
		t.Fatalf("there must be no run before the first import: %+v", empty.Run)
	}

	answer := requestJSON[trackerImportResponse](t, h, http.MethodPost, "/api/tracker/import", map[string]any{"provider": "github"}, http.StatusOK)
	run := answer.Run
	if run == nil {
		t.Fatal("the trigger must answer with the run")
	}
	if run.Status != domain.TrackerImportPartial {
		t.Fatalf("run status %q, want partial (reason %q)", run.Status, run.Reason)
	}
	if run.Imported != 1 || run.Failed != 1 {
		t.Fatalf("run counts %+v", run)
	}
	if len(run.Repositories) != 2 {
		t.Fatalf("the run must name every repository: %+v", run.Repositories)
	}
	var broken domain.TrackerImportRepository
	for _, outcome := range run.Repositories {
		if outcome.RepositoryID == "repo-broken" {
			broken = outcome
		}
	}
	if broken.Status != domain.TrackerImportFailed || broken.Reason == "" {
		t.Fatalf("the broken repository must fail with a reason: %+v", broken)
	}

	// The run is persisted, so the last outcome survives a restart.
	stored := requestJSON[trackerImportResponse](t, h, http.MethodGet, "/api/tracker/import?provider=github", nil, http.StatusOK)
	if stored.Run == nil {
		t.Fatal("the last run must be readable")
	}
	if stored.Run.Status != run.Status || stored.Run.Imported != run.Imported || stored.Run.Failed != run.Failed {
		t.Fatalf("the stored run differs from the answered one: %+v vs %+v", stored.Run, run)
	}
	if stored.Run.FinishedAt.IsZero() {
		t.Fatalf("the stored run must record when it finished: %+v", stored.Run)
	}

	// The imported requirement carries the GitHub fields, and the pull
	// request did not become one.
	issue, ok := s.Issue("gh-repo-good-2")
	if !ok {
		t.Fatal("the issue did not become a requirement")
	}
	if issue.Number != 2 || issue.Title != "First issue" || issue.Description != "First brief" || issue.Status != "todo" {
		t.Fatalf("imported requirement lost a field: %+v", issue)
	}
	if issue.ExternalRef == nil || issue.ExternalRef.ExternalURL != "https://github.com/example/good/issues/2" {
		t.Fatalf("the requirement must link back to GitHub: %+v", issue.ExternalRef)
	}
	if _, ok := s.Issue("gh-repo-good-3"); ok {
		t.Fatal("a pull request must not become a requirement")
	}
}

// A GitLab project imports through the same trigger and last-run endpoints as
// GitHub: one requirement per issue, with GitLab's state and type mapped and a
// link back to the issue.
func TestGitLabTrackerImportRunsOnDemandAndReportsTheLastRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/projects/example%2Frepo/issues" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "gitlab-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":90,"iid":1,"title":"GitLab bug","description":"Bug brief","state":"opened","issue_type":"bug","web_url":"https://gitlab.com/example/repo/-/issues/1"},
			{"id":91,"iid":2,"title":"GitLab incident","description":"Incident brief","state":"closed","issue_type":"incident","web_url":"https://gitlab.com/example/repo/-/issues/2"}
		]`))
	}))
	defer srv.Close()
	t.Setenv("AGENTFLOW_GITLAB_API_BASE", srv.URL)

	root := t.TempDir()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{LoginName: "gitlab-owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := s.UserByLoginName("gitlab-owner")
	if err := s.SaveGitHTTPCredential(owner.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitLab", Host: "gitlab.com", Platform: domain.GitPlatformGitLab}, Token: "gitlab-token"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProject(domain.Project{ID: "project-1", Name: "Project", OwnerID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRepository(domain.Repository{ID: "repo-gl", ProjectID: "project-1", Provider: "gitlab", Name: "repo", GongfengProjectID: "gitlab:gitlab.com/example/repo", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	h := New(s, tracker.NewService(), "", "")

	// Before the first run there is nothing to show for GitLab.
	empty := requestJSON[trackerImportResponse](t, h, http.MethodGet, "/api/tracker/import?provider=gitlab", nil, http.StatusOK)
	if empty.Run != nil {
		t.Fatalf("there must be no GitLab run before the first import: %+v", empty.Run)
	}

	answer := requestJSON[trackerImportResponse](t, h, http.MethodPost, "/api/tracker/import", map[string]any{"provider": "gitlab"}, http.StatusOK)
	run := answer.Run
	if run == nil || run.Provider != "gitlab" {
		t.Fatalf("the trigger must answer with the GitLab run: %+v", run)
	}
	if run.Status != domain.TrackerImportSuccess || run.Imported != 2 {
		t.Fatalf("run %+v", run)
	}
	if len(run.Repositories) != 1 || run.Repositories[0].RepositoryID != "repo-gl" || run.Repositories[0].Imported != 2 {
		t.Fatalf("the run must name the GitLab repository: %+v", run.Repositories)
	}

	// The run is persisted, so the last outcome survives a restart.
	stored := requestJSON[trackerImportResponse](t, h, http.MethodGet, "/api/tracker/import?provider=gitlab", nil, http.StatusOK)
	if stored.Run == nil || stored.Run.Status != run.Status || stored.Run.Imported != run.Imported {
		t.Fatalf("the stored GitLab run differs from the answered one: %+v vs %+v", stored.Run, run)
	}
	if stored.Run.FinishedAt.IsZero() {
		t.Fatalf("the stored run must record when it finished: %+v", stored.Run)
	}

	bug, ok := s.Issue("gl-repo-gl-1")
	if !ok {
		t.Fatal("the GitLab issue did not become a requirement")
	}
	if bug.Number != 1 || bug.Title != "GitLab bug" || bug.Description != "Bug brief" || bug.Status != "todo" || bug.Kind != "bug" {
		t.Fatalf("imported GitLab requirement lost a field: %+v", bug)
	}
	if bug.ExternalRef == nil || bug.ExternalRef.ExternalURL != "https://gitlab.com/example/repo/-/issues/1" {
		t.Fatalf("the requirement must link back to GitLab: %+v", bug.ExternalRef)
	}
	incident, ok := s.Issue("gl-repo-gl-2")
	if !ok {
		t.Fatal("the closed GitLab issue did not become a requirement")
	}
	if incident.Status != "closed" || incident.Kind != "issue" {
		t.Fatalf("closed incident mapped to status %q kind %q, want closed/issue", incident.Status, incident.Kind)
	}
}

// An unknown provider is refused instead of silently importing nothing.
func TestTrackerImportRefusesAnUnknownProvider(t *testing.T) {
	root := t.TempDir()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPost, "/api/tracker/import", map[string]any{"provider": "bitbucket"}, http.StatusBadRequest)
}
