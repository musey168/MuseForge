package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// The platform has no administrator side: no member-account console, no
// administrator role, no shared key, no platform-level host trust, no public
// address and no access keys. Every one of those routes must not answer.
func TestPlatformAdministratorRoutesAreGone(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")

	for _, tc := range []struct {
		method string
		path   string
	}{
		// The access-key page: the external caller's access key.
		{http.MethodGet, "/api/auth/tokens"},
		{http.MethodPost, "/api/auth/tokens"},
		{http.MethodDelete, "/api/auth/tokens/token-1"},
		// The member-account console.
		{http.MethodGet, "/api/platform/accounts"},
		{http.MethodPost, "/api/platform/accounts"},
		{http.MethodPut, "/api/platform/accounts/1/password"},
		// The platform-administrator role and its grant/revoke flow.
		{http.MethodGet, "/api/platform/admins"},
		{http.MethodPut, "/api/platform/admins"},
		// The shared CodeBuddy key.
		{http.MethodGet, "/api/platform/codebuddy-key"},
		{http.MethodPut, "/api/platform/codebuddy-key"},
		{http.MethodDelete, "/api/platform/codebuddy-key"},
		// The platform-level Git host trust.
		{http.MethodGet, "/api/platform/git-ssh-hosts"},
		{http.MethodPost, "/api/platform/git-ssh-hosts"},
		{http.MethodDelete, "/api/platform/git-ssh-hosts"},
		{http.MethodPost, "/api/platform/git-ssh-hosts/preview"},
		{http.MethodPut, "/api/platform/git-ssh-hosts/github.com"},
		{http.MethodDelete, "/api/platform/git-ssh-hosts/github.com"},
		{http.MethodGet, "/api/platform/git-https-hosts"},
		{http.MethodPost, "/api/platform/git-https-hosts"},
		{http.MethodDelete, "/api/platform/git-https-hosts"},
		{http.MethodPost, "/api/platform/git-https-hosts/preview"},
		{http.MethodPut, "/api/platform/git-https-hosts/github.com"},
		{http.MethodDelete, "/api/platform/git-https-hosts/github.com"},
		{http.MethodGet, "/api/git-https-hosts"},
		{http.MethodGet, "/api/platform/git-http-hosts"},
		{http.MethodPost, "/api/platform/git-http-hosts"},
		{http.MethodDelete, "/api/platform/git-http-hosts"},
		{http.MethodDelete, "/api/platform/git-http-hosts/github.com"},
		{http.MethodGet, "/api/git-http-hosts"},
		// The public-address setting.
		{http.MethodGet, "/api/platform/public-url"},
		{http.MethodPut, "/api/platform/public-url"},
		{http.MethodDelete, "/api/platform/public-url"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, "http://agentflow.example"+tc.path, nil))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s still answers with %d", tc.method, tc.path, rec.Code)
		}
	}
}
