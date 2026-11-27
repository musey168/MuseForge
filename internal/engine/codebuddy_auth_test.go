package engine

import (
	"testing"

	"agentflow/internal/domain"
	"agentflow/internal/repo"
)

func TestResolveCodeBuddyAPIKeyFromUserOnly(t *testing.T) {
	t.Setenv(codeBuddyAPIKeyEnv, "ck_host")
	t.Setenv(codeBuddyInternetEnv, "ioa")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{ID: 21, LoginName: "owner", CodeBuddyAPIKey: "ck_user", CodeBuddyInternetEnvironment: "private-net"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutUser(domain.User{ID: 22, LoginName: "newcomer"}); err != nil {
		t.Fatal(err)
	}
	if got := resolveCodeBuddyCredential(s, 21); got.APIKey != "ck_user" || got.InternetEnvironment != "private-net" {
		t.Fatalf("typed user credential = %+v", got)
	}
	if got := resolveCodeBuddyCredential(s, 22); got.APIKey != "" {
		t.Fatalf("user without a key must not borrow one, got %+v", got)
	}
	if got := resolveCodeBuddyCredential(s, 999); got.APIKey != "" || got.InternetEnvironment != "" {
		t.Fatalf("missing user must not use host env, got %+v", got)
	}
	if got := resolveCodeBuddyCredential(nil, 21); got.APIKey != "" {
		t.Fatalf("nil store = %+v", got)
	}
}
