package httpapi

import (
	"agentflow/internal/repo"
	"testing"
)

func TestLocalSignupOptionsForExternalIdentity(t *testing.T) {
	for _, mode := range []struct {
		name string
		env  map[string]string
	}{
		{name: "gateway", env: map[string]string{"AGENTFLOW_TRUST_IDENTITY_HEADERS": "true"}},
		{name: "SSO", env: map[string]string{"AGENTFLOW_SSO_APP_TOKEN": "app-token", "AGENTFLOW_SSO_LOGIN_URL": "https://sso.example.test/login"}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			t.Setenv("AGENTFLOW_TRUST_IDENTITY_HEADERS", "false")
			t.Setenv("AGENTFLOW_SSO_APP_TOKEN", "")
			t.Setenv("AGENTFLOW_SSO_LOGIN_URL", "")
			for key, value := range mode.env {
				t.Setenv(key, value)
			}
			t.Setenv("AGENTFLOW_ALLOW_SIGNUP", "true")
			h := &authHandlers{deps: &deps{}}
			needsSetup, signup, err := h.localSignupOptions()
			if err != nil || needsSetup || signup {
				t.Fatalf("setup=%v signup=%v err=%v", needsSetup, signup, err)
			}
		})
	}
}

// Only the app token and the login URL together switch the login page to
// SSO. Either one alone still serves password sign-in, so a fresh install
// must keep its browser setup step.
func TestLocalSignupOptionsPartialSSOKeepsPasswordSetup(t *testing.T) {
	for _, key := range []string{"AGENTFLOW_SSO_APP_TOKEN", "AGENTFLOW_SSO_LOGIN_URL"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("AGENTFLOW_TRUST_IDENTITY_HEADERS", "false")
			t.Setenv("AGENTFLOW_SSO_APP_TOKEN", "")
			t.Setenv("AGENTFLOW_SSO_LOGIN_URL", "")
			t.Setenv("AGENTFLOW_PLATFORM_ADMINS", "")
			t.Setenv("AGENTFLOW_ALLOW_SIGNUP", "false")
			t.Setenv(key, "configured")
			s, err := repo.OpenTest()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			h := &authHandlers{deps: &deps{repo: s}}
			needsSetup, signup, err := h.localSignupOptions()
			if err != nil || !needsSetup || !signup {
				t.Fatalf("setup=%v signup=%v err=%v", needsSetup, signup, err)
			}
		})
	}
}
