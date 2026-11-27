package engine

import (
	"agentflow/internal/repo"
	"agentflow/internal/runtime"
)

const (
	codeBuddyAPIKeyEnv   = "CODEBUDDY_API_KEY"
	codeBuddyInternetEnv = "CODEBUDDY_INTERNET_ENVIRONMENT"
)

// resolveCodeBuddyCredential returns the responsible user's own key.
func resolveCodeBuddyCredential(s *repo.Store, userID int64) runtime.CodeBuddyCredential {
	if s == nil || userID <= 0 {
		return runtime.CodeBuddyCredential{}
	}
	user, ok := s.User(userID)
	if !ok {
		return runtime.CodeBuddyCredential{}
	}
	return runtime.CodeBuddyUserCredential(user)
}
