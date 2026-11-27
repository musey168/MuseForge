package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentflow/internal/domain"
	"agentflow/internal/runtime"

	"github.com/gin-gonic/gin"
)

// A model call is one tool-free turn against a locally installed agent CLI.
// It backs the surfaces that need a model's judgement rather than a
// conversation: the run evaluation, the SOP detection and the evolution
// proposals. It never keeps a session, never offers tools and never records
// a conversation.

type modelCallRequest struct {
	Prompt, Model, SystemPrompt string
	UserID                       int64
}

// runModelCall runs one tool-free turn and returns the model's reply text.
func runModelCall(ctx context.Context, req modelCallRequest, user domain.User) (string, error) {
	model := assistantRequestModel("", req.Model)
	engine, engineModel := assistantModelEngine(model)
	if engineModel == "default" && (engine == assistantEngineCodex || engine == assistantEngineClaude) {
		engineModel = ""
	}
	bin, err := resolveAssistantEngineBin(engine)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "agentflow-model-call-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	for _, dir := range []string{filepath.Join(root, "home", "tmp"), filepath.Join(root, "ws")} {
		if err = os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
	}
	config := filepath.Join(root, "mcp.json")
	if err = os.WriteFile(config, []byte(`{"mcpServers":{}}`), 0600); err != nil {
		return "", err
	}
	sessionID := newAssistantSessionID()
	engineOpts := assistantEngineInvocationOpts{
		Bin: bin, Root: root, Config: config, Prompt: req.Prompt, Model: engineModel,
		SessionID: sessionID, SystemPrompt: req.SystemPrompt, ReviewOnly: true,
	}
	var inv assistantCLIInvocation
	switch engine {
	case assistantEngineClaude:
		engineOpts.User = runtime.ProviderCredential{APIKey: user.ClaudeCodeAPIKey, BaseURL: user.ClaudeCodeBaseURL}
		inv, err = prepareAssistantClaudeInvocation(engineOpts)
	case assistantEngineCodex:
		engineOpts.User = runtime.ProviderCredential{APIKey: user.CodexAPIKey, BaseURL: user.CodexBaseURL}
		inv, err = prepareAssistantCodexInvocation(engineOpts)
	default:
		if runtime.IsAgentCLI(engine) {
			engineOpts.User = runtime.AgentCLIUserCredentials(user)[engine]
			inv, err = prepareAssistantAgentCLIInvocation(engine, engineOpts)
			break
		}
		inv.command, err = prepareAssistantCommand(assistantCommandOpts{
			Bin: bin, Config: config, Prompt: req.Prompt, Model: engineModel,
			SessionID: sessionID, SystemPrompt: req.SystemPrompt, ReviewOnly: true,
		})
		inv.credentials = runtime.RunCredentials{CodeBuddy: runtime.CodeBuddyUserCredential(user)}
		inv.sessionRef = sessionID
	}
	if err != nil {
		return "", err
	}
	inv.credentials.Extra = runtime.ExtraRunCredentials(user.ID)
	command := inv.command.WithPrivateHomeDir(filepath.Join(root, "home"))
	workspace := filepath.Join(root, "ws")
	var final string
	var resultError bool
	var stderrTail []string
	var claudeResultText, claudeMessageID string
	var claudeTextBlocks []string
	captureClaudeText := engine == assistantEngineClaude
	sink := func(event domain.TelemetryEvent) {
		if event.Name != "process.stdout" {
			if event.Name == "process.stderr" {
				if line := strings.TrimSpace(event.Message); line != "" {
					stderrTail = append(stderrTail, line)
					if len(stderrTail) > 20 {
						stderrTail = stderrTail[len(stderrTail)-20:]
					}
				}
			}
			return
		}
		line := strings.TrimSpace(event.Message)
		if inv.translate != nil {
			for _, translated := range inv.translate(event.Message) {
				line = strings.TrimSpace(translated)
				parseModelCallLine(line, &final, &resultError, &claudeResultText, &claudeMessageID, &claudeTextBlocks, captureClaudeText)
			}
			return
		}
		parseModelCallLine(line, &final, &resultError, &claudeResultText, &claudeMessageID, &claudeTextBlocks, captureClaudeText)
	}
	if _, err = runtime.ExecuteLocalCommand(ctx, workspace, command, inv.credentials, sink); err != nil {
		if len(stderrTail) > 0 {
			return "", fmt.Errorf("%w; stderr=%s", err, strings.Join(stderrTail, " | "))
		}
		return "", err
	}
	if captureClaudeText && claudeResultText != "" {
		final = claudeResultText
	}
	return decodeAssistantCLIResult(final, resultError, nil, stderrTail)
}

// parseModelCallLine reads one stream-json line of a CLI turn. CodeBuddy
// reports the answer in a "result" line; Claude Code streams text blocks and
// may close successfully with an empty result.
func parseModelCallLine(line string, final *string, resultError *bool, claudeResultText, claudeMessageID *string, claudeTextBlocks *[]string, captureClaudeText bool) {
	if line == "" {
		return
	}
	var result struct {
		Type            string `json:"type"`
		Subtype         string `json:"subtype"`
		Result          string `json:"result"`
		IsError         bool   `json:"is_error"`
		StopReason      string `json:"stop_reason"`
		ParentToolUseID string `json:"parent_tool_use_id"`
		Message         struct {
			ID      string `json:"id"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &result) != nil {
		return
	}
	switch result.Type {
	case "result":
		*final, *resultError = result.Result, result.IsError
		if captureClaudeText && result.ParentToolUseID == "" {
			if result.Subtype == "success" && !result.IsError && strings.TrimSpace(*final) == "" &&
				(result.StopReason == "" || result.StopReason == "end_turn" || result.StopReason == "stop_sequence") {
				*claudeResultText = joinClaudeTextBlocks(*claudeTextBlocks)
			}
			*claudeMessageID, *claudeTextBlocks = "", nil
		}
	case "assistant":
		if !captureClaudeText {
			return
		}
		resetClaudeText := result.Message.ID == "" || result.Message.ID != *claudeMessageID
		for _, block := range result.Message.Content {
			if result.ParentToolUseID != "" || block.Type != "text" {
				continue
			}
			text := strings.TrimSpace(block.Text)
			if text == "" || assistantCLINoticeText(text) {
				continue
			}
			if resetClaudeText {
				*claudeMessageID, *claudeTextBlocks = result.Message.ID, nil
				resetClaudeText = false
			}
			*claudeTextBlocks = append(*claudeTextBlocks, block.Text)
		}
	}
}

// assistantCLINoticeText reports whether a CLI line is a notice instead of
// the assistant's answer.
func assistantCLINoticeText(text string) bool {
	return strings.HasPrefix(text, "[")
}

// modelCallUser resolves the account a model call runs as.
func modelCallUser(c *gin.Context) (domain.User, bool) {
	user, ok := c.Get("user")
	if !ok {
		return domain.User{}, false
	}
	stored, ok := user.(domain.User)
	return stored, ok
}
func joinClaudeTextBlocks(blocks []string) string {
	var reply strings.Builder
	for i, block := range blocks {
		if i > 0 {
			left := strings.TrimRight(blocks[i-1], " \t")
			right := strings.TrimLeft(block, " \t")
			if !strings.HasSuffix(left, "\n") && !strings.HasSuffix(left, "\r") &&
				!strings.HasPrefix(right, "\n") && !strings.HasPrefix(right, "\r") {
				reply.WriteString("\n\n")
			}
		}
		reply.WriteString(block)
	}
	return reply.String()
}

func isCredentialRejection(text string) bool {
	text = strings.TrimSpace(text)
	return len(text) < 120 && (strings.HasPrefix(text, "401 ") || strings.HasPrefix(text, "403 ") || text == "401" || text == "403")
}

type assistantCommandOpts struct {
	Bin, Config, Prompt, Model, Effort, SessionID, SystemPrompt, Settings string
	ImagePaths                                                            []string
	Resume, Native, ReportOnly, ReviewOnly, Workspace                     bool
}

func prepareAssistantCommand(opts assistantCommandOpts) (runtime.CommandSpec, error) {
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = assistantFixedCatalog().Default
	}
	sys := strings.TrimSpace(opts.SystemPrompt)
	if sys == "" {
		sys = assistantSystemPromptText()
	}
	workspace := opts.Workspace && !opts.ReviewOnly && !opts.ReportOnly
	if workspace {
		sys += "\n\n" + runtime.CodeBuddyBackgroundAgentInstructions
	}
	// Use the CLI's unlimited value; omitting the flag retains a finite default.
	// A turn has no deadline; explicit cancellation still stops execution.
	args := []string{"-p", "--max-turns", nativeAssistantUnlimitedTurns, "--system-prompt", sys, "--output-format", "stream-json", "--include-partial-messages", "--mcp-config", opts.Config}
	if strings.TrimSpace(opts.Settings) != "" {
		args = append(args, "--settings", opts.Settings)
	}
	inv := runtime.InvocationOptions{}
	if opts.Resume && strings.TrimSpace(opts.SessionID) != "" {
		inv.ResumeSessionID = opts.SessionID
	} else if strings.TrimSpace(opts.SessionID) != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	ex := domain.ExecutorDefinition{Kind: "codebuddy", Binary: opts.Bin, Model: model, Effort: opts.Effort, Args: args}
	command, err := runtime.PrepareProviderCommand(ex, opts.Prompt, "local", inv)
	if err != nil {
		return runtime.CommandSpec{}, err
	}
	if opts.ReviewOnly {
		// Reviewers have no tools, MCP gateway, project settings or persistent history.
		command.Argv = replaceArgValue(command.Argv, "--tools", "")
		command.Argv = replaceArgValue(command.Argv, "--max-turns", "1")
		command.Argv = replaceArgValue(command.Argv, "--setting-sources", "")
		command.Argv = appendNativeArgs(command.Argv, "--no-session-persistence")
	} else if opts.ReportOnly {
		command.Argv = applyNativeToolSafety(command.Argv, AgentToolReport)
		command.Argv = replaceArgValue(command.Argv, "--max-turns", "6")
		command.Argv = replaceArgValue(command.Argv, "--disallowedTools", nativeDisallowedTools+",Read,Grep,Glob,Skill,TodoWrite,AskUserQuestion,EnterPlanMode,ExitPlanMode")
		command.Argv = replaceArgValue(command.Argv, "--setting-sources", "")
		command.Argv = appendNativeArgs(command.Argv, "--no-session-persistence")
	} else if !workspace {
		command.Argv = applyNativeToolSafety(command.Argv, AgentToolAssistant)
	} else {
		command.Argv = replaceArgValue(command.Argv, "--setting-sources", "user")
		command.Argv = appendNativeArgs(command.Argv, "--strict-mcp-config")
	}
	command.CredentialBindings = []runtime.CredentialBinding{runtime.BindCodeBuddy}
	command.Telemetry = nil
	if workspace || len(opts.ImagePaths) > 0 {
		command.Argv = appendNativeArgs(command.Argv, "--verbose")
		command, err = withAssistantStreamInput(command, opts.Prompt, opts.ImagePaths, 0)
		if err != nil {
			return runtime.CommandSpec{}, err
		}
	}
	// A fresh session embeds the whole conversation in the prompt; past
	// MAX_ARG_STRLEN it would fail with E2BIG before the CLI starts.
	command = runtime.PromptOnStdin(command, opts.Prompt)
	return command.WithMCPStartupWait(agentToolSessionMCPStartupWait), nil
}

// assistantCodexGuard denies every native Codex tool: a model call offers no
// tools at all, and the platform's own MCP gateway stays the only way in.
const assistantCodexGuard = `import json,sys
event=json.load(sys.stdin)
name=event.get("tool_name", "")
if not name.startswith("mcp__agentflow_session__"):
    print(json.dumps({"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Use the LoopForge session MCP tools; native tools are disabled in Main Bot."}}))
    sys.exit(2)
`
