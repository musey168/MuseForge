package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentflow/internal/auth"

	"github.com/gin-gonic/gin"
)

func TestAuthTokenPureBranches(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	t.Setenv("AGENTFLOW_AUTH_REQUIRED", "not-a-bool")
	if authRequired() {
		t.Fatal("invalid auth-required value enabled auth")
	}
	for path, want := range map[string]bool{
		insightIngestPath: false, "/mcp": true, "/other": false,
		"/api/health": false, "/api/auth/photo/name": false, "/api/auth/tokens": true,
		"/api/projects": true, "/runners/heartbeat": false,
	} {
		if got := protectedAccessPath(path); got != want {
			t.Errorf("protectedAccessPath(%q)=%v want %v", path, got, want)
		}
	}
	for value, want := range map[string]bool{
		"": false, "Basic x": false, "Bearer": false, "Bearer ": false, "bearer token": true,
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", value)
		_, got := bearerToken(req)
		if got != want {
			t.Errorf("bearerToken(%q)=%v", value, got)
		}
	}
	if tokenPrefix("short") != "short" || len(tokenPrefix(strings.Repeat("x", 20))) != 12 {
		t.Fatal("token prefix")
	}

	d := deps{repo: f.s}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestUserContextKey{}, domainUserToAuth(f.user)))
	if resolved, ok := d.requestUser(req); !ok || resolved.ID != f.user.ID {
		t.Fatalf("context user: %+v %v", resolved, ok)
	}

	t.Setenv("AGENTFLOW_PREVIEW_LOCAL_LOGIN", "1")
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", "")
	if _, ok := d.previewLocalAuthUser(); ok {
		t.Fatal("empty preview login accepted")
	}
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", f.user.LoginName)
	if resolved, ok := d.previewLocalAuthUser(); !ok || resolved.ID != f.user.ID {
		t.Fatalf("preview user: %+v %v", resolved, ok)
	}

	middleware := (&tokenHandlers{deps: &d}).userAccessAuth()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	middleware(c)
}

func TestRunnerBinaryHelperBranches(t *testing.T) {
	root := t.TempDir()
	if got := sanitizePlatformParam(" Linux "); got != "linux" {
		t.Fatalf("sanitized = %q", got)
	}
	for _, value := range []string{"bad-value", strings.Repeat("a", 17)} {
		if sanitizePlatformParam(value) != "" {
			t.Errorf("unsafe platform accepted: %q", value)
		}
	}
	t.Setenv("AGENTFLOW_SOURCE_ROOT", root)
	if runnerSourceRoot() != "" {
		t.Fatal("invalid source root accepted")
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "agentflow-runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runnerSourceRoot() != root {
		t.Fatal("valid source root not selected")
	}
	t.Setenv("AGENTFLOW_SOURCE_ROOT", "")
	if !dirExists(filepath.Join(root, "cmd")) || dirExists(filepath.Join(root, "missing")) {
		t.Fatal("directory helper")
	}

	digestRoot := t.TempDir()
	if _, err := runnerSourceDigest(digestRoot); err == nil {
		t.Fatal("incomplete source digest succeeded")
	}
	for _, dir := range []string{"cmd/agentflow-runner", "cmd/agentflow-gate-mcp", "internal/pkg/testdata"} {
		if err := os.MkdirAll(filepath.Join(digestRoot, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"go.mod": "module test\n", "go.sum": "", "cmd/agentflow-runner/main.go": "package main\n",
		"cmd/agentflow-runner/main_test.go": "ignored", "internal/pkg/x.go": "package pkg\n",
		"internal/pkg/testdata/ignored.go": "ignored",
	} {
		if err := os.WriteFile(filepath.Join(digestRoot, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if digest, err := runnerSourceDigest(digestRoot); err != nil || len(digest) != 64 {
		t.Fatalf("digest = %q %v", digest, err)
	}
	if got := clipBuildOutput([]byte(strings.Repeat("x", 500))); len(got) != 400 {
		t.Fatalf("clipped length = %d", len(got))
	}
}



func TestGongfengOAuthAuthenticatedStart(t *testing.T) {
	t.Setenv("AGENTFLOW_GONGFENG_HOST", "git.review.test")
	f := cvBNewHTTPFixture(t)
	t.Setenv("AGENTFLOW_GONGFENG_OAUTH_CLIENT_ID", "client")
	t.Setenv("AGENTFLOW_GONGFENG_OAUTH_CLIENT_SECRET", "secret")
	t.Setenv("AGENTFLOW_GONGFENG_OAUTH_REDIRECT_URI", "https://example.test/callback")
	call := func(client *auth.Client, want int) {
		h := &authHandlers{deps: &deps{repo: f.s, authClient: client}}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		req := httptest.NewRequest(http.MethodGet, "/api/auth/gongfeng/start?return=quick-access", nil)
		c.Request = req.WithContext(context.WithValue(req.Context(), requestUserContextKey{}, domainUserToAuth(f.user)))
		h.gongfengOAuthStart(c)
		if rec.Code != want {
			t.Fatalf("OAuth start = %d want %d: %s", rec.Code, want, rec.Body.String())
		}
	}
	call(auth.NewClient(auth.Config{SessionSecret: "oauth-secret"}), http.StatusFound)
}

func TestAuthCurrentGatewayAndPreviewBranches(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	stored := f.user
	stored.ChineseName = "Gateway User"
	stored.DeptID = 42
	stored.DeptName = "Engineering"
	stored.StaffID = 99
	if err := f.s.PutUser(stored); err != nil {
		t.Fatal(err)
	}
	h := &authHandlers{deps: &deps{repo: f.s, authClient: auth.NewClient(auth.Config{SessionSecret: "secret"})}}
	t.Setenv("AGENTFLOW_TRUST_IDENTITY_HEADERS", "1")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Staffname", stored.LoginName)
	c.Request = req
	h.authCurrent(c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Engineering") {
		t.Fatalf("gateway current = %d: %s", rec.Code, rec.Body.String())
	}

	t.Setenv("AGENTFLOW_PREVIEW_LOCAL_LOGIN", "1")
	t.Setenv("AGENTFLOW_LOCAL_LOGIN_USER", stored.LoginName)
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	h.authCurrent(c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), stored.LoginName) {
		t.Fatalf("preview current = %d: %s", rec.Code, rec.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/auth?return_url=https://example.com/path", nil)
	if got := h.getReturnURL(request); got != "https://example.com/path" {
		t.Fatalf("return URL = %q", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/auth", nil)
	request.Header.Set("Referer", "https://example.com/ref")
	if got := h.getReturnURL(request); got != "https://example.com/ref" {
		t.Fatalf("referer URL = %q", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/auth?return_url=https://return.test/path", nil)
	request.Header.Set("Referer", "https://referer.test/")
	if got := h.getReturnURL(request); got != "http://example.com/" {
		t.Fatalf("foreign return URL must fall back to site root, got %q", got)
	}
}

func TestUsersNilAndMalformedBranches(t *testing.T) {
	h := &usersHandlers{deps: &deps{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/users", nil)
	h.users(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil users = %d", rec.Code)
	}

	f := cvBNewHTTPFixture(t)
	uh := &usersHandlers{deps: &deps{repo: f.s}}
	rec = cvBDirectHandlerRequest(t, f, http.MethodPut, "/users/x", nil, uh.user,
		map[string]string{"id": "invalid"}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid user id = %d", rec.Code)
	}
}

func TestRequestUserRejectsInvalidSignedCookie(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	h := &tokenHandlers{deps: &deps{repo: f.s}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "invalid"})
	if _, ok := h.requestUser(req); ok {
		t.Fatal("invalid signed cookie authenticated")
	}
}
