package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentflow/internal/auth"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuthCookieSecureBehindHTTPSProxy(t *testing.T) {
	s := &Server{deps: &deps{authClient: auth.NewClient(auth.Config{})}}
	s.auth = &authHandlers{deps: s.deps}
	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/auth/current", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	s.auth.setAuthCookie(rec, req, "signed")
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("HTTPS proxy response must set a Secure cookie: %+v", cookies)
	}
}
