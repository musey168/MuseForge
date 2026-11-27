package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/runtime"
)

// The CLI connectivity test asks a bound machine to run its installed CLI
// once. The machine starts the process over its control connection and the
// platform polls it until the process finishes. Nothing here belongs to a
// conversation: there is no session, no workspace and no tool gateway.

// runnerCLIExecutor runs one command on a bound runner.
type runnerCLIExecutor struct {
	controls    *runnerControlHub
	runnerID    string
	runnerLogin bool
	sessionKey  string
	root        string
	user        domain.User
}

// conversationProcessStopTimeout bounds how long a finished or cancelled
// process may take to confirm its exit.
var conversationProcessStopTimeout = 30 * time.Second

// conversationProcessBusyMaxDelay is the longest backoff step while a runner
// reports that it is still busy with another request.
var conversationProcessBusyMaxDelay = 3200 * time.Millisecond

// execute runs the command on the runner and returns its output.
func (e *runnerCLIExecutor) execute(ctx context.Context, command runtime.CommandSpec, credentials runtime.RunCredentials, sink runtime.EventSink) (string, error) {
	call := func(callCtx context.Context, req runtime.ConversationProcessRequest) (runtime.ConversationProcessResult, error) {
		req.SessionKey = e.sessionKey
		req.Transient = true
		req.SourceRoot = e.root
		raw, err := json.Marshal(req)
		if err != nil {
			return runtime.ConversationProcessResult{}, err
		}
		envelope := domain.RunnerWorkspaceRequest{
			Operation:   domain.RunnerWorkspaceOperationConversationCLI,
			Conversation: raw,
		}
		var response domain.RunnerWorkspaceResponse
		for delay := 50 * time.Millisecond; ; delay *= 2 {
			response, err = e.controls.requestWorkspace(callCtx, e.runnerID, envelope)
			// The runner returned 429 before handling the request. Retry with
			// backoff instead of failing the test.
			if err != nil || response.Status != http.StatusTooManyRequests || delay > conversationProcessBusyMaxDelay {
				break
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-callCtx.Done():
				timer.Stop()
				return runtime.ConversationProcessResult{}, callCtx.Err()
			}
			timer.Stop()
		}
		if err != nil {
			return runtime.ConversationProcessResult{}, fmt.Errorf("%w: %w", errAssistantExecutionUnavailable, err)
		}
		if response.Error != "" {
			return runtime.ConversationProcessResult{}, errors.New(response.Error)
		}
		var result runtime.ConversationProcessResult
		err = json.Unmarshal(response.Payload, &result)
		return result, err
	}
	if _, err := call(ctx, runtime.ConversationProcessRequest{Operation: "prepare"}); err != nil {
		return "", err
	}
	wire := runtime.ConversationCommandFor(command, credentials)
	wire.RunnerLogin = e.runnerLogin
	wire.Argv[0] = assistantEngineBinary(command.Provider)
	id, err := runtime.NewConversationProcessID()
	if err != nil {
		return "", err
	}
	var stopOnce sync.Once
	stopProcess := func() {
		stopOnce.Do(func() {
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), conversationProcessStopTimeout)
			defer cancel()
			_ = waitConversationProcessStop(stopCtx, id, call, nil)
		})
	}
	defer stopProcess()
	started, err := call(ctx, runtime.ConversationProcessRequest{Operation: "start", ID: id, Command: wire})
	if err != nil {
		stopOnce.Do(func() {}) // confirmed failure before the process started
		return "", err
	}
	after := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		result, pollErr := call(ctx, runtime.ConversationProcessRequest{Operation: "poll", ID: started.ID, After: after})
		if pollErr != nil {
			return "", pollErr
		}
		if sink != nil {
			for _, event := range result.Events {
				sink(event)
			}
		}
		after = result.Sequence
		if result.Done {
			if result.Error != "" {
				return "", errors.New(result.Error)
			}
			return "", nil
		}
	}
}

// waitConversationProcessStop confirms that a started process has exited.
func waitConversationProcessStop(ctx context.Context, id string, call func(context.Context, runtime.ConversationProcessRequest) (runtime.ConversationProcessResult, error), notify func(bool)) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := call(ctx, runtime.ConversationProcessRequest{Operation: "poll", ID: id})
		if err != nil {
			return err
		}
		if result.Done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// testRunnerCLI asks a bound runner to run its CLI once and answer.
func testRunnerCLI(ctx context.Context, controls *runnerControlHub, runner domain.Runner, provider string, runnerLogin bool, user domain.User) error {
	root, err := os.MkdirTemp("", "loopforge-cli-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	model := provider + ":default"
	if provider == assistantEngineCodeBuddy {
		model = loadAssistantModels(user.CodeBuddyInternetEnvironment, user.CodeBuddyAPIKey).Default
		if user.CodeBuddyBaseURL != "" {
			model = personalCodeBuddyCatalog(ctx, user).Default
		}
	} else if !runnerLogin {
		models, _ := assistantDiscoveredModelsFor(ctx, user, provider)
		if len(models) == 0 {
			models = assistantEngineBuiltinModels(provider)
		}
		if len(models) > 0 {
			model = provider + ":" + models[0].ID
		}
	}
	bin := assistantEngineBinary(provider)
	command, err := prepareAssistantCommand(assistantCommandOpts{
		Bin: bin, Config: filepath.Join(root, "mcp.json"), Prompt: "Respond with OK.",
		SystemPrompt: "You are a connectivity check. Reply in plain text. Do not use tools.",
		Model: model, ReviewOnly: true,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "mcp.json"), []byte(`{"mcpServers":{}}`), 0600); err != nil {
		return err
	}
	executor := &runnerCLIExecutor{
		controls: controls, runnerID: runner.ID, runnerLogin: runnerLogin,
		sessionKey: newAssistantSessionID(), root: root, user: user,
	}
	_, err = executor.execute(ctx, command, runtime.RunCredentials{CodeBuddy: runtime.CodeBuddyUserCredential(user)}, nil)
	return err
}

// runnerCLITestFailure maps a CLI test error onto an HTTP status.
func runnerCLITestFailure(ctx context.Context, err error) int {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

// runnerCLITestMessage explains a CLI test failure in the operator's words.
func runnerCLITestMessage(ctx context.Context, err error) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
		return "CLI 测试超时，请检查执行机器和模型服务的连接"
	case errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled):
		return "CLI 测试已取消"
	case strings.Contains(err.Error(), "执行机器未安装 CLI"):
		return "执行机器未安装该 CLI"
	default:
		return "CLI 测试失败，请检查执行机器上的 CLI、认证和模型服务配置"
	}
}
