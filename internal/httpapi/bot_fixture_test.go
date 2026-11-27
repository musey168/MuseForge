package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentflow/internal/auth"

	"agentflow/internal/domain"
	"agentflow/internal/engine"
	"agentflow/internal/notification"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// botFixture builds a store with one IM bot and the two accounts bound to it.
func botFixture(t *testing.T) (*repo.Store, *deps, domain.NotificationBot) {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.PutUser(domain.User{ID: 42, LoginName: "bot-owner", CodeBuddyAPIKey: "owner-key", CodeBuddyInternetEnvironment: "ioa"})
	s.PutUser(domain.User{ID: 43, LoginName: "other-user", CodeBuddyAPIKey: "other-key", CodeBuddyInternetEnvironment: "ioa"})
	s.PutProject(domain.Project{ID: "p", OwnerID: 42, Members: []domain.ProjectMember{{UserID: 43, Role: "member"}}})
	bot := domain.NotificationBot{ID: "b", BotID: "external", ProjectID: "p", OwnerID: 42, Enabled: true, InteractionEnabled: true, Model: domain.DefaultNotificationReportModel}
	if err = s.SaveNotificationBot(bot, "secret"); err != nil {
		t.Fatal(err)
	}
	e := engine.New(s, nil)
	e.ConfigureExecution("api", "test", time.Minute, false)
	d := &deps{repo: s, engine: e}
	notifications := &notificationHandlers{deps: d, service: notification.New(s)}
	notifications.service.PublicURL = "http://platform.example"
	bindBotTestUser(t, s, bot, "new-wecom-user", 43)
	bindBotTestUser(t, s, bot, "bound-member", 42)
	return s, d, bot
}

// bindBotTestUser binds a sender to a platform account.
func bindBotTestUser(t *testing.T, s *repo.Store, bot domain.NotificationBot, sender string, userID int64) {
	t.Helper()
	now := time.Now().UTC()
	token, err := s.NewBotBindingChallenge(bot, sender, now, true)
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.ClaimBotBindingChallenge(token, userID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmBotUserBinding(bot, sender, code, now); err != nil {
		t.Fatal(err)
	}
}

// authedRequest signs a request as the given user and marks it same-origin.
func authedRequest(t *testing.T, req *http.Request, user domain.User) *http.Request {
	t.Helper()
	token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: user.ID, LoginName: user.LoginName, AuthTime: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	// Browsers send Origin on writes; in-process forwarding must keep it same-origin.
	req.Header.Set("Origin", "http://"+req.Host)
	return req
}

// botNotificationService returns the notification service of a test store.
func botNotificationService(d *deps) *notification.Service {
	return notification.New(d.repo)
}

// workbenchFixture builds a store with one owner account and one project.
func workbenchFixture(t *testing.T) (*deps, domain.User) {
	t.Helper()
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err = st.PutUser(domain.User{LoginName: "workbench-owner", CodeBuddyAPIKey: "test-only"}); err != nil {
		t.Fatal(err)
	}
	u, _ := st.UserByLoginName("workbench-owner")
	if err = st.PutProject(domain.Project{ID: "workbench", Name: "Workbench", OwnerID: u.ID}); err != nil {
		t.Fatal(err)
	}
	return &deps{repo: st}, u
}

// workbenchContext builds a gin context that acts as the given user.
func workbenchContext(u domain.User, method, path string, body *bytes.Buffer) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, path, body)
	c.Request = req.WithContext(context.WithValue(req.Context(), requestUserContextKey{}, domainUserToAuth(u)))
	return c, rec
}
