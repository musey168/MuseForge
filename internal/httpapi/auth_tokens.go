package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/auth"
	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

type requestUserContextKey struct{}

func authRequired() bool {
	value := strings.TrimSpace(os.Getenv("AGENTFLOW_AUTH_REQUIRED"))
	if value == "" {
		return false
	}
	required, err := strconv.ParseBool(value)
	return err == nil && required
}

type tokenHandlers struct {
	*deps
}

// userAccessAuth protects public API and MCP calls when explicitly enabled.
// Studio assets and the login exchange remain public so a guest can reach the
// sign-in flow. Token-management handlers always enforce interactive login.
func (h *tokenHandlers) userAccessAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		// Always attach the signed-in actor when present. LocalInvoker and
		// background generation hops reuse this request context; without the
		// key, login-gated tools (runs_cancel, etc.) see an anonymous
		// call and return 请先登录 even though the outer /mcp or generation
		// request was authenticated via cookie.
		user, ok := h.requestUser(c.Request)
		if authRequired() && protectedAccessPath(c.Request.URL.Path) && !ok {
			c.Header("WWW-Authenticate", `Bearer realm="AgentFlow"`)
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			c.Abort()
			return
		}
		if ok {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestUserContextKey{}, user))
		}
		c.Next()
	}
}

func isKnowledgeOAuthCallbackPath(path string) bool {
	if path == "/api/knowledge/oauth/callback" {
		return true
	}
	rest, ok := strings.CutPrefix(path, "/api/knowledge/oauth/")
	if !ok {
		return false
	}
	typ, ok := strings.CutSuffix(rest, "/callback")
	if !ok || typ == "" {
		return false
	}
	return !strings.Contains(typ, "/")
}

func protectedAccessPath(path string) bool {
	// Ingestion always checks its own project-scoped credential.
	if path == insightIngestPath {
		return false
	}
	// OAuth callbacks arrive in a fresh tab without a platform session; the
	// signed state parameter is the credential.
	if isKnowledgeOAuthCallbackPath(path) {
		return false
	}
	if path == "/mcp" {
		return true
	}
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	// Runner-agent endpoints carry their own bootstrap/session credentials.
	if isRunnerAgentPath(path) {
		return false
	}
	switch path {
	// gongfeng/start checks the session itself so a signed-out browser
	// gets a page that says so, not a bare JSON 401.
	case "/api/health", "/api/auth/password", "/api/auth/setup", "/api/auth/register", "/api/auth/current", "/api/auth/exchange", "/api/auth/logout", "/api/auth/local", "/api/auth/photo", "/api/auth/gongfeng/start", "/api/auth/gongfeng/callback":
		return false
	default:
		if strings.HasPrefix(path, "/api/auth/photo/") {
			return false
		}
		return true
	}
}

// requestUser 挂 deps（身份解析与多个领域共用）。It resolves a signed
// browser session. Gateway identity headers are resolved by authCurrent, which
// exchanges them for the signed HttpOnly browser cookie.
func (d deps) requestUser(r *http.Request) (*auth.User, bool) {
	if current, ok := r.Context().Value(requestUserContextKey{}).(*auth.User); ok && current != nil && !current.IsGuest {
		return current, true
	}
	cookie, err := r.Cookie(authCookieName)
	if err != nil || cookie.Value == "" {
		return d.previewLocalAuthUser()
	}
	user, err := d.getAuthClient().ParseSignedUser(cookie.Value)
	if err != nil || user == nil || user.IsGuest || strings.TrimSpace(user.LoginName) == "" {
		return d.previewLocalAuthUser()
	}
	if authSessionExpired(user) {
		return d.previewLocalAuthUser()
	}
	if stored, exists := d.repo.UserByLoginName(user.LoginName); exists {
		user.ID = stored.ID
		user.StaffID = stored.StaffID
		user.ChineseName = stored.ChineseName
		user.DeptID = stored.DeptID
		user.DeptName = stored.DeptName
	}
	return user, true
}

func bearerToken(r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	scheme, raw, ok := strings.Cut(value, " ")
	raw = strings.TrimSpace(raw)
	return raw, ok && strings.EqualFold(scheme, "Bearer") && raw != ""
}

// previewLocalAuthUser signs the configured local acceptance user into this
// process only. It stays off unless the preview start script sets
// AGENTFLOW_PREVIEW_LOCAL_LOGIN=1, authentication is not required, and
// AGENTFLOW_LOCAL_LOGIN_USER names an existing user.
func (d deps) previewLocalAuthUser() (*auth.User, bool) {
	if strings.TrimSpace(os.Getenv("AGENTFLOW_PREVIEW_LOCAL_LOGIN")) != "1" || authRequired() || d.repo == nil {
		return nil, false
	}
	login := strings.TrimSpace(os.Getenv("AGENTFLOW_LOCAL_LOGIN_USER"))
	if login == "" {
		return nil, false
	}
	stored, ok := d.repo.UserByLoginName(login)
	if !ok {
		return nil, false
	}
	user := domainUserToAuth(stored)
	user.AvatarURL = auth.AvatarURL(stored.LoginName, 100)
	return user, true
}

func domainUserToAuth(user domain.User) *auth.User {
	return &auth.User{
		ID:          user.ID,
		StaffID:     user.StaffID,
		LoginName:   user.LoginName,
		ChineseName: user.ChineseName,
		DeptID:      user.DeptID,
		DeptName:    user.DeptName,
		IsGuest:     false,
		AuthTime:    time.Now().UTC(),
	}
}

// isAccount reports whether the request acts as the platform's single account.
// The personal build stores one account, so with authentication off every
// request is that account; with it on, the resolved identity is.
func (d deps) isAccount(r *http.Request) bool {
	if !authRequired() {
		return true
	}
	_, ok := d.requestUser(r)
	return ok
}

func randomID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
