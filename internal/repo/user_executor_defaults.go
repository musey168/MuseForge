package repo

import (
	"errors"

	"agentflow/internal/domain"
)

// ErrAssistantDefaultExecutorUnavailable means the chosen default executor
// has no credential behind it.
var ErrAssistantDefaultExecutorUnavailable = errors.New("请选择已配置的执行器")

// firstExecutorWithCredential is the CLI an account can run with when no
// default was chosen.
func firstExecutorWithCredential(user domain.User) string {
	for _, executor := range append([]string{"codebuddy", "codex", "claude-code"}, domain.AgentCLIExecutorKinds...) {
		if user.HasExecutorCredential(executor) {
			return executor
		}
	}
	return ""
}
