package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentflow/internal/auth"
)

// 回归用例：浏览器会话的 AuthTime 超过 authSessionTTL 后，/api/auth/current 必须
// 回落到访客身份并清掉过期 Cookie，让前端把用户重新送到 SSO 登录页。
// 修复前这里会用原始 AuthTime 重新签发 Cookie，形成"Cookie 一直续期、AuthTime
// 早已过期"的半失效状态：/api/auth/current 报告已登录，受保护接口却一律 401，
// 用户永久卡在「启动失败：authentication required」页面且无法自愈。
func TestAuthCurrent_ExpiredSessionFallsBackToGuest(t *testing.T) {
	authClient := auth.NewClient(auth.Config{
		AppDomain:     "loopforge.example.com",
		SessionSecret: "secret-session-expiry",
	})
	handler, _ := newTestAuthServer(t, authClient)

	expired := &auth.User{
		StaffID:   1001,
		LoginName: "expired_user",
		IsGuest:   false,
		AuthTime:  time.Now().UTC().Add(-authSessionTTL - time.Hour),
	}
	token, err := authClient.SignUser(expired)
	if err != nil {
		t.Fatalf("sign user failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		User     auth.User `json:"user"`
		LoginURL string    `json:"loginUrl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if !body.User.IsGuest {
		t.Fatalf("expected expired session to be reported as guest, got %+v", body.User)
	}
	if body.LoginURL == "" {
		t.Errorf("expected loginUrl so the Studio can bounce the user back to the SSO login page")
	}

	// 必须清掉过期 Cookie，否则浏览器会一直带着它重试，永远走不到访客分支。
	cleared := false
	for _, raw := range rec.Header().Values("Set-Cookie") {
		if !strings.Contains(raw, authCookieName+"=") {
			continue
		}
		if strings.Contains(raw, "Max-Age=0") || strings.Contains(raw, "MaxAge=0") {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("expected expired cookie to be cleared, got Set-Cookie: %v", rec.Header().Values("Set-Cookie"))
	}
}

// 有效会话必须照旧保持登录，并续签 Cookie（防止修复把正常会话也踢成访客）。