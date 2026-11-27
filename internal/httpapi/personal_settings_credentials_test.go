package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/engine"
	"agentflow/internal/repo"
	"agentflow/internal/runtime"
	"agentflow/internal/tracker"
)

// Personal settings still hold the single account's own credentials: the model
// keys, the GitHub authorization and the Git credentials. A configured flag
// travels to the browser; the secret never does.
func TestPersonalSettingsKeepTheAccountsCredentials(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "workspaces"))), tracker.NewService(), "", "")

	user, ok := s.User(1)
	if !ok {
		t.Fatal("the test store has no account")
	}

	const (
		codeBuddyKey = "ck-personal-secret"
		codexKey     = "ckx-personal-secret"
		gitToken     = "git-personal-secret"
	)
	saved := requestJSONAuthed[domain.User](t, h, user, http.MethodPut,
		"/api/users/"+strconv.FormatInt(user.ID, 10)+"/credentials",
		map[string]any{
			"codeBuddyApiKey": codeBuddyKey,
			"codexApiKey":     codexKey,
		}, http.StatusOK)
	if !saved.CodeBuddyAPIKeyConfigured || !saved.CodexAPIKeyConfigured {
		t.Fatalf("the model keys were not recorded: %+v", saved)
	}
	body := mustJSON(t, saved)
	for _, secret := range []string{codeBuddyKey, codexKey} {
		if strings.Contains(body, secret) {
			t.Fatalf("the personal settings response leaks the secret %q", secret)
		}
	}

	// The GitHub authorization arrives through the OAuth callback; personal
	// settings then show that it is configured and never the token itself.
	if _, err := s.UsersRepo.SaveGongfengOAuth(user.ID, gitToken, "refresh-personal-secret", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	afterOAuth := requestJSON[struct {
		Users []domain.User `json:"users"`
	}](t, h, http.MethodGet, "/api/bootstrap", nil, http.StatusOK)
	if len(afterOAuth.Users) == 0 || !afterOAuth.Users[0].GitTokenOAuth {
		t.Fatalf("the GitHub authorization is not shown as configured: %+v", afterOAuth)
	}
	for _, secret := range []string{gitToken, "refresh-personal-secret"} {
		if strings.Contains(mustJSON(t, afterOAuth), secret) {
			t.Fatalf("the personal settings response leaks the secret %q", secret)
		}
	}

	// The account's own record keeps the secrets; the API never hands them out.
	stored, ok := s.User(user.ID)
	if !ok || stored.CodeBuddyAPIKey != codeBuddyKey || stored.CodexAPIKey != codexKey {
		t.Fatalf("the credentials were not stored: %+v", stored)
	}
	shell := requestJSON[struct {
		Users []domain.User `json:"users"`
	}](t, h, http.MethodGet, "/api/bootstrap", nil, http.StatusOK)
	for _, listed := range shell.Users {
		if strings.Contains(mustJSON(t, listed), gitToken) {
			t.Fatalf("the account list leaks a secret: %+v", listed)
		}
	}

	// The Git credentials are the account's own: a saved host comes back
	// without its token.
	credentials := requestJSONAuthed[[]domain.GitHTTPCredential](t, h, user, http.MethodPut,
		"/api/users/"+strconv.FormatInt(user.ID, 10)+"/git-http-credentials",
		map[string]any{
			"host":     "github.com",
			"name":     "Personal",
			"username": "muse",
			"token":    "http-personal-secret",
		}, http.StatusOK)
	if len(credentials) != 1 || credentials[0].Host != "github.com" {
		t.Fatalf("the Git credential was not saved: %+v", credentials)
	}
	if strings.Contains(mustJSON(t, credentials), "http-personal-secret") {
		t.Fatalf("the Git credential response leaks the token: %+v", credentials)
	}
}

// mustJSON renders a value as JSON so a test can look for a secret in it.
func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
