// Package agentcli holds the vocabulary and model catalogs of the agent CLIs
// the machine inventory detects. It knows nothing about running an agent: the
// platform no longer executes anything, it only reports what a machine has and
// which models its CLIs can reach.
package agentcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"agentflow/internal/domain"
)

// Agent CLIs beyond CodeBuddy / Codex / Claude Code.
const (
	AgentCLIOpenCode    = "opencode"
	AgentCLICursor      = "cursor"
	AgentCLIAntigravity = "agy"
	AgentCLIKimi        = "kimi"
	AgentCLIPi          = "pi"
	AgentCLIDeepSeek    = "dsh"
	AgentCLIZCode       = "zcode"
)

// AgentCLIKinds is the display order of the agent CLIs.
var AgentCLIKinds = domain.AgentCLIExecutorKinds

func IsAgentCLI(kind string) bool { return domain.IsAgentCLIKind(kind) }

const (
	AgentCLIProtocolChat      = domain.AgentCLIProtocolChat
	AgentCLIProtocolResponses = domain.AgentCLIProtocolResponses
	AgentCLIProtocolMessages  = domain.AgentCLIProtocolMessages
	AgentCLIProtocolGemini    = domain.AgentCLIProtocolGemini
)

func AgentCLIProtocols(kind string) []string { return domain.AgentCLIProtocolOptions[kind] }

// AgentCLIProtocol normalizes a stored protocol choice; unknown values fall
// back to the CLI's default.
func AgentCLIProtocol(kind, protocol string) string {
	return domain.NormalizeAgentCLIProtocol(kind, protocol)
}

// AgentCLILabel is the product name shown in errors and settings.
func AgentCLILabel(kind string) string {
	switch kind {
	case AgentCLIOpenCode:
		return "OpenCode"
	case AgentCLICursor:
		return "Cursor CLI"
	case AgentCLIAntigravity:
		return "Antigravity"
	case AgentCLIKimi:
		return "Kimi Code"
	case AgentCLIPi:
		return "Pi"
	case AgentCLIDeepSeek:
		return "DeepSeek Harness"
	case AgentCLIZCode:
		return "ZCode"
	}
	return kind
}

// AgentCLIBinary is the executable each agent CLI installs.
func AgentCLIBinary(kind string) string {
	switch kind {
	case AgentCLICursor:
		return "cursor-agent"
	}
	return kind
}

// ErrMaxTurns marks a CLI that gave up after its turn budget.
var ErrMaxTurns = errors.New("model call exceeded its turn budget")

// OnDemandCLIPath returns an already-installed executable for a CLI, without
// downloading anything.
func OnDemandCLIPath(kind string) (string, bool) {
	name := AgentCLIBinary(kind)
	candidates := []string{}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", name))
	}
	if root := strings.TrimSpace(os.Getenv("AGENTFLOW_MANAGED_CLI_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, kind, "current", "node_modules", ".bin", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

// OnDemandInstallable reports whether a CLI is one the platform knows how to
// install. The platform does not install CLIs any more; the answer only tells
// the inventory whether a missing binary is one it could describe.
func OnDemandInstallable(kind string) bool {
	switch kind {
	case "codex", "codebuddy", "claude-code", AgentCLIOpenCode, AgentCLICursor,
		AgentCLIAntigravity, AgentCLIKimi, AgentCLIPi, AgentCLIDeepSeek:
		return true
	}
	return false
}
