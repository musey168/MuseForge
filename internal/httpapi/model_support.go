package httpapi

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"agentflow/internal/agentcli"
	"agentflow/internal/domain"
	"agentflow/internal/managedcli"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(text string, max int) string {
	return limitRunes(strings.TrimSpace(text), max)
}

func limitRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

func resolveCodeBuddyBin() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENTFLOW_CODEBUDDY_BIN")); override != "" {
		if fileExecutable(override) {
			return override, nil
		}
	}
	if root := strings.TrimSpace(os.Getenv("AGENTFLOW_MANAGED_CLI_ROOT")); root != "" {
		inventory, err := managedcli.ReadInventory(root)
		if err != nil {
			return "", fmt.Errorf("读取托管 CodeBuddy 清单失败: %w", err)
		}
		if binary, ok := inventory.BinaryFor("codebuddy"); ok {
			return binary, nil
		}
		providers, err := managedcli.EnabledProviders()
		if err != nil {
			return "", fmt.Errorf("读取托管 CLI 配置失败: %w", err)
		}
		if slices.Contains(providers, "codebuddy") {
			return "", fmt.Errorf("托管 CodeBuddy 尚未安装，请检查 worker 的 CLI 安装状态")
		}
	}
	if p, err := exec.LookPath("codebuddy"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	if strings.TrimSpace(home) == "" {
		home = "/root"
	}
	candidates := []string{filepath.Join(home, ".local", "bin", "codebuddy")}
	for _, dir := range assistantCLIGlobalDirs {
		candidates = append(candidates, filepath.Join(dir, "codebuddy"))
	}
	for _, c := range candidates {
		if fileExecutable(c) {
			return c, nil
		}
	}
	if path, ok := agentcli.OnDemandCLIPath("codebuddy"); ok {
		return path, nil
	}
	return "", assistantCLIMissingError{engine: assistantEngineCodeBuddy, binary: "codebuddy"}
}

func fileExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func assistantCatalogLists(catalog assistantModelCatalog, id string) bool {
	for _, model := range catalog.Models {
		if model.ID == id {
			return true
		}
	}
	return false
}

func runnerUsesLocalLogin(runner domain.Runner, engine string) bool {
	return assistantSupportsRunnerLogin(engine) &&
		runner.RuntimeAuthMode(engine) == domain.RuntimeAuthLocal
}

func assistantSupportsRunnerLogin(engine string) bool {
	return domain.SupportsRunnerLogin(engine)
}

func redactAgentToolPath(path string) string {
	const prefix = "/internal/agent-tools/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := path[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return prefix + "[redacted]" + rest[i:]
	}
	return prefix + "[redacted]"
}

var errAssistantMissingResult = errors.New("CLI did not return a non-empty answer in type=result")

// The agent CLIs the platform can drive. A model call never offers tools, so
// only the two modes that shape the command line survive.
type AgentToolMode string

const (
	AgentToolAssistant AgentToolMode = "assistant"
	AgentToolReport    AgentToolMode = "report"
)

const (
	// Monitor and PowerShell run shell commands on the control plane like Bash.
	nativeDisallowedTools = "Bash,PowerShell,Monitor,Write,Edit,MultiEdit,NotebookEdit,WebFetch,WebSearch,Task,KillShell,CronCreate,CronList,CronDelete"
	// Match CodeBuddy's interactive unlimited value (Number.MAX_SAFE_INTEGER).
	nativeAssistantUnlimitedTurns = "9007199254740991"
	agentToolSessionMCPServerID   = "agentflow_session"
	// The session gateway lists upstream TAPD/工蜂 tools before it answers
	// tools/list, which can exceed CodeBuddy's ~2s headless MCP wait.
	agentToolSessionMCPStartupWait = 30 * time.Second
)

// assistantExecutor maps a legacy executor field onto a CLI kind.
func assistantExecutor(id string) string {
	switch strings.TrimSpace(id) {
	case "", assistantEngineCodeBuddy:
		return assistantEngineCodeBuddy
	case assistantEngineCodex:
		return assistantEngineCodex
	case assistantEngineClaude, "claude":
		return assistantEngineClaude
	default:
		return strings.TrimSpace(id)
	}
}

const workbenchAttachmentLimit = 10 << 20

func assistantEngineBinary(kind string) string {
	switch kind {
	case assistantEngineCodeBuddy:
		return "codebuddy"
	case assistantEngineCodex:
		return "codex"
	case assistantEngineClaude:
		return "claude"
	default:
		return agentcli.AgentCLIBinary(kind)
	}
}
