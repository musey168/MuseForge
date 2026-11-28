package httpapi

import (
	"net/http"
	"testing"

	"agentflow/internal/auth"
	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// httpFixture is a store-backed server with one user: the shared fixture the
// httpapi tests that need a signed-in or preview-able user start from.
type httpFixture struct {
	s    *repo.Store
	user domain.User
	h    http.Handler
}

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.PutUser(domain.User{LoginName: "auth-user", ChineseName: "测试用户"}); err != nil {
		t.Fatal(err)
	}
	user, ok := s.UserByLoginName("auth-user")
	if !ok {
		t.Fatal("fixture user was not stored")
	}
	return httpFixture{s: s, user: user, h: New(s, nil, "", "")}
}

// cvBNewHTTPFixture keeps the name the case files use.
func cvBNewHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	return newHTTPFixture(t)
}

// newTestAuthServer serves the authentication endpoints with the given auth
// client, so a test can sign a session with its own secret.
func newTestAuthServer(t *testing.T, client *auth.Client) (http.Handler, error) {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		return nil, err
	}
	t.Cleanup(s.Close)
	d := &deps{repo: s, authClient: client, passwordAttempts: newPasswordLimiter()}
	api := &Server{deps: d, auth: &authHandlers{deps: d}, users: &usersHandlers{deps: d}}
	g := gin.New()
	g.GET("/api/auth/current", api.auth.authCurrent)
	return g, nil
}
