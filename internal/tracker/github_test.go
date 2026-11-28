package tracker

import (
	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubImportUsesTheImportingProjectOwnersToken(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	owner := func(login, token string) int64 {
		if err := st.PutUser(domain.User{LoginName: login}); err != nil {
			t.Fatal(err)
		}
		u, _ := st.UserByLoginName(login)
		if token != "" {
			if err := st.SaveGitHTTPCredential(u.ID, "", domain.GitHTTPCredentialSecret{GitHTTPCredential: domain.GitHTTPCredential{Name: "GitHub", Host: "github.com", Platform: domain.GitPlatformGitHub}, Token: token}); err != nil {
				t.Fatal(err)
			}
		}
		return u.ID
	}
	// "a-*" sorts before "z-*", so the old code used the a-project Owner's
	// token for every project that binds the repository.
	projects := []struct{ id, login, token string }{
		{"a-no-token", "owner-no-token", ""},
		{"m-member", "owner-with-access", "access-token"},
		{"z-other", "owner-other", "other-token"},
	}
	repos := map[string]domain.Repository{}
	for _, p := range projects {
		if err := st.PutProject(domain.Project{ID: p.id, Name: p.id, OwnerID: owner(p.login, p.token)}); err != nil {
			t.Fatal(err)
		}
		r := domain.Repository{ID: "repo-" + p.id, ProjectID: p.id, Provider: "github", Name: "private", GongfengProjectID: "github:example/private", DefaultBranch: "main", WebURL: "https://github.com/example/private"}
		if err := st.PutRepository(r); err != nil {
			t.Fatal(err)
		}
		repos[p.id] = r
	}
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		if auth != "Bearer access-token" && auth != "Bearer other-token" {
			w.WriteHeader(http.StatusNotFound) // GitHub hides private repositories
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":10,"number":2,"title":"Private issue","body":"secret","state":"open"}]`))
	}))
	defer srv.Close()
	s := NewService()
	s.ConfigureWithStore(st)
	s.githubAPIBase = srv.URL

	for _, c := range []struct {
		project, auth string
		imported      bool
	}{
		{"m-member", "Bearer access-token", true},
		{"z-other", "Bearer other-token", true},
		{"a-no-token", "", false},
	} {
		seen = nil
		n, err := s.importProviderRepository(st, &githubProvider{service: s}, repos[c.project])
		if len(seen) != 1 || seen[0] != c.auth {
			t.Fatalf("%s: GitHub saw Authorization %q, want %q", c.project, seen, c.auth)
		}
		_, stored := st.Issue("gh-repo-" + c.project + "-2")
		if c.imported != (err == nil && n == 1 && stored) {
			t.Fatalf("%s: imported=%d stored=%v err=%v, want imported=%v", c.project, n, stored, err, c.imported)
		}
	}
}
