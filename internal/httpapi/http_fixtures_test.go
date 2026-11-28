package httpapi

import (
	"net/http"
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
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
