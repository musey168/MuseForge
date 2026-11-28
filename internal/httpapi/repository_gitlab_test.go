package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// repositoryResponse is the answer of the repository endpoints.
type repositoryResponse struct {
	ID                string `json:"id"`
	Provider          string `json:"provider"`
	Name              string `json:"name"`
	GongfengProjectID string `json:"gongfengProjectId"`
	WebURL            string `json:"webUrl"`
	CloneURL          string `json:"cloneUrl"`
}

// A GitLab project key names the host and the group path. Binding one records
// a GitLab repository with the addresses its project lives at, so the GitLab
// import has a source to read.
func TestGitLabRepositoryBindsWithItsProjectKey(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPut, "/api/projects", map[string]any{"id": "gl", "key": "GL", "name": "GitLab 项目", "ownerId": 1}, http.StatusOK)

	bound := requestJSON[repositoryResponse](t, h, http.MethodPost, "/api/repositories", map[string]any{
		"projectId":         "gl",
		"gongfengProjectId": "gitlab:gitlab.com/my-group/my-repo",
	}, http.StatusOK)
	if bound.Provider != "gitlab" {
		t.Fatalf("provider = %q, want gitlab", bound.Provider)
	}
	if bound.GongfengProjectID != "gitlab:gitlab.com/my-group/my-repo" {
		t.Fatalf("project key = %q", bound.GongfengProjectID)
	}
	if bound.WebURL != "https://gitlab.com/my-group/my-repo" {
		t.Fatalf("web url = %q", bound.WebURL)
	}
	if bound.CloneURL != "https://gitlab.com/my-group/my-repo.git" {
		t.Fatalf("clone url = %q", bound.CloneURL)
	}
	if bound.Name != "my-repo" {
		t.Fatalf("name = %q, want my-repo", bound.Name)
	}
}

// The import finds the GitLab repository and reports the missing credential
// with a reason, instead of importing nothing and saying so silently.
func TestGitLabImportReportsTheMissingCredential(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	requestJSON[map[string]any](t, h, http.MethodPut, "/api/projects", map[string]any{"id": "gl", "key": "GL", "name": "GitLab 项目", "ownerId": 1}, http.StatusOK)
	requestJSON[repositoryResponse](t, h, http.MethodPost, "/api/repositories", map[string]any{
		"projectId":         "gl",
		"gongfengProjectId": "gitlab:gitlab.com/my-group/my-repo",
	}, http.StatusOK)

	answer := requestJSON[trackerImportResponse](t, h, http.MethodPost, "/api/tracker/import", map[string]any{"provider": "gitlab", "projectId": "gl"}, http.StatusOK)
	run := answer.Run
	if run == nil {
		t.Fatal("the import returned no run")
	}
	if run.Status != domain.TrackerImportFailed {
		t.Fatalf("status = %q, want failed", run.Status)
	}
	if len(run.Repositories) != 1 {
		t.Fatalf("repositories = %d, want the one bound GitLab repository", len(run.Repositories))
	}
	if run.Repositories[0].Status != domain.TrackerImportFailed {
		t.Fatalf("repository status = %q, want failed", run.Repositories[0].Status)
	}
	if !strings.Contains(run.Reason, "GitLab") || !strings.Contains(run.Reason, "凭据") {
		t.Fatalf("reason = %q, want it to name the missing GitLab credential", run.Reason)
	}
}
