package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"agentflow/internal/repo"
	"agentflow/internal/tracker"
)

// The personal settings store the single account's model keys and read them
// back configured — never in clear text on the way out.
func TestPersonalCredentialsRoundTripWithoutLeakingTheSecret(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")

	account := requestJSON[map[string]any](t, h, http.MethodPost, "/api/account", map[string]any{}, http.StatusOK)
	id, _ := account["id"].(float64)
	if id == 0 {
		t.Fatalf("the account endpoint must return the account: %+v", account)
	}
	path := "/api/users/" + strconvID(id) + "/credentials"

	saved := requestJSON[map[string]any](t, h, http.MethodPut, path, map[string]any{"codeBuddyApiKey": "ck_test_secret"}, http.StatusOK)
	if saved["codeBuddyApiKeyConfigured"] != true {
		t.Fatalf("the saved key must read back as configured: %+v", saved)
	}
	raw := requestJSON[map[string]any](t, h, http.MethodPut, path, map[string]any{"codeBuddyApiKey": "ck_test_secret"}, http.StatusOK)
	for key, value := range raw {
		if text, ok := value.(string); ok && strings.Contains(text, "ck_test_secret") {
			t.Fatalf("the response leaks the secret in %q", key)
		}
	}

	// The catalogue is what the console renders, and it reports the CLI as
	// configured without naming the key.
	catalog := requestJSON[map[string]any](t, h, http.MethodGet, "/api/model-catalog", nil, http.StatusOK)
	engines, _ := catalog["engines"].([]any)
	found := false
	for _, entry := range engines {
		engine, _ := entry.(map[string]any)
		if engine["id"] == "codebuddy" {
			found = true
			if engine["configured"] != true {
				t.Fatalf("the catalogue must report CodeBuddy configured: %+v", engine)
			}
			if !engine["personal"].(bool) {
				t.Fatalf("a key the account saved is personal, not shared: %+v", engine)
			}
		}
	}
	if !found {
		t.Fatalf("the catalogue has no CodeBuddy entry: %+v", catalog)
	}
}

// Clearing a key removes it: the catalogue reports the CLI unconfigured again.
func TestPersonalCredentialCanBeCleared(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	account := requestJSON[map[string]any](t, h, http.MethodPost, "/api/account", map[string]any{}, http.StatusOK)
	id, _ := account["id"].(float64)
	path := "/api/users/" + strconvID(id) + "/credentials"
	requestJSON[map[string]any](t, h, http.MethodPut, path, map[string]any{"codeBuddyApiKey": "ck_test_secret"}, http.StatusOK)
	cleared := requestJSON[map[string]any](t, h, http.MethodPut, path, map[string]any{"codeBuddyApiKeyClear": true}, http.StatusOK)
	if cleared["codeBuddyApiKeyConfigured"] == true {
		t.Fatalf("a cleared key must read back unconfigured: %+v", cleared)
	}
}

// The account endpoint is idempotent: the platform has exactly one account, and
// asking again returns it rather than creating a second.
func TestAccountEndpointIsIdempotent(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, tracker.NewService(), "", "")
	first := requestJSON[map[string]any](t, h, http.MethodPost, "/api/account", map[string]any{}, http.StatusOK)
	second := requestJSON[map[string]any](t, h, http.MethodPost, "/api/account", map[string]any{"loginName": "someone-else"}, http.StatusOK)
	if first["id"] != second["id"] {
		t.Fatalf("the platform must keep one account: %+v then %+v", first, second)
	}
	if second["loginName"] != first["loginName"] {
		t.Fatalf("the existing account must be returned unchanged: %+v", second)
	}
}

// strconvID renders an id for a URL path.
func strconvID(v float64) string { return strconv.FormatInt(int64(v), 10) }
