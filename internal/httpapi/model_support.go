package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"agentflow/internal/domain"
	"agentflow/internal/managedcli"
	"agentflow/internal/repo"
	"agentflow/internal/runtime"

	"github.com/gin-gonic/gin"

	"agentflow/internal/cronexpr"
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
	if path, ok := runtime.OnDemandCLIPath("codebuddy"); ok {
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

func assistantSessionRoot(userID int64) string {
	return filepath.Join(assistantSessionBase(), strconv.FormatInt(userID, 10))
}

func newAssistantSessionID() string {
	return "af" + strconv.FormatInt(time.Now().UnixNano(), 36)
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

func scheduleLoginRunner(r *repo.Store, userID int64, projectID, engine string) domain.Runner {
	if r == nil || userID <= 0 || !assistantSupportsRunnerLogin(engine) {
		return domain.Runner{}
	}
	id := r.DefaultRunner(userID)
	if id == "" {
		return domain.Runner{}
	}
	machine, ok := r.Runner(id)
	if !ok || machine.OwnerUserID != userID || !machine.SupportsCLISettings() ||
		!machine.SupportsProvider(engine) || !runnerUsesLocalLogin(machine, engine) ||
		!r.CanUseRunner(machine, projectID, userID) {
		return domain.Runner{}
	}
	return machine
}

func scheduledAssistantDefault(r *repo.Store, user domain.User, projectID, model string) string {
	model = strings.TrimSpace(model)
	if !assistantModelUnspecified(model) {
		return model
	}
	if preferred := personalDefaultModel(user); preferred != "" {
		return preferred
	}
	for _, option := range assistantEngineOptions(user) {
		if !scheduleHasPersonalCredential(r, user, projectID, option.ID) {
			continue
		}
		if option.ID == assistantEngineCodeBuddy {
			// Resolve the legacy default with this account's CodeBuddy catalog.
			return model
		}
		return featureDefaultModel(user, option.ID)
	}
	return model
}

func scheduleCredentialUser(r *repo.Store, user domain.User, projectID, engine string) (domain.User, bool) {
	return user, scheduleHasPersonalCredential(r, user, projectID, engine)
}

func sameScheduleReferenceIDs(a, b []domain.AssetReference) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

func parseAssistantCron(expr string) (cronexpr.Schedule, error) {
	return cronexpr.Parse(expr)
}

func nextAssistantCron(expr string, after time.Time) (time.Time, error) {
	return cronexpr.Next(expr, after)
}

func repositoryRunContext(r domain.Repository) domain.RepositoryContext {
	return domain.RepositoryContext{RepositoryID: r.ID, Provider: r.Provider, Name: r.Name, GongfengProjectID: r.GongfengProjectID, WebURL: r.WebURL, CloneURL: r.CloneURL, BaseBranch: r.DefaultBranch, DefaultBranch: r.DefaultBranch, LocalPath: r.LocalPath, PipelineProjectID: r.PipelineProjectID, PipelineDefID: r.PipelineDefID, PipelineURLPattern: r.PipelineURLPattern}
}

func applyNativeToolSafety(argv []string, mode AgentToolMode) []string {
	_ = mode
	// --tools is a built-in allowlist. CodeBuddy also applies it to MCP tools,
	// so listing Read/Grep/Glob or mcp__* globs hides submit_* and TAPD/工蜂.
	argv = dropNativeArg(argv, "--tools")
	if !slices.Contains(argv, "--disallowedTools") {
		argv = appendNativeArgs(argv, "--disallowedTools", nativeDisallowedTools)
	}
	if !slices.Contains(argv, "--strict-mcp-config") {
		argv = appendNativeArgs(argv, "--strict-mcp-config")
	}
	return argv
}

func appendNativeArgs(argv []string, args ...string) []string {
	for i, arg := range argv {
		if arg == "--" {
			out := append([]string(nil), argv[:i]...)
			out = append(out, args...)
			return append(out, argv[i:]...)
		}
	}
	return append(argv, args...)
}

func replaceArgValue(argv []string, flag, value string) []string {
	out := make([]string, 0, len(argv)+2)
	replaced := false
	skip := false
	for i := 0; i < len(argv); i++ {
		if skip {
			skip = false
			continue
		}
		if argv[i] == flag {
			out = append(out, flag, value)
			skip = true
			replaced = true
			continue
		}
		out = append(out, argv[i])
	}
	if !replaced {
		out = appendNativeArgs(out, flag, value)
	}
	return out
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

func botChannelName(bot domain.NotificationBot) string {
	switch bot.ChannelKind() {
	case domain.BotChannelFeishu:
		return "飞书"
	case domain.BotChannelDingTalk:
		return "钉钉"
	default:
		return "企业微信"
	}
}

func assistantStreamImageInput(prompt string, imagePaths []string) (*bytes.Buffer, error) {
	parts := []map[string]any{{"type": "text", "text": prompt}}
	for _, path := range imagePaths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("无法打开本轮图片附件")
		}
		data, mime, err := readAssistantImage(file)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{"type": "image", "source": map[string]any{
			"type": "base64", "media_type": mime, "data": data,
		}})
	}
	var input bytes.Buffer
	err := json.NewEncoder(&input).Encode(map[string]any{
		"type": "user", "message": map[string]any{"role": "user", "content": parts},
	})
	return &input, err
}

func trimAssistantReplyText(text string) string {
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	for {
		line, rest, ok := strings.Cut(text, "\n")
		if !ok || strings.TrimSpace(line) != "" {
			return text
		}
		text = rest
	}
}

var errAssistantMissingResult = errors.New("CLI did not return a non-empty answer in type=result")

func assistantCLIMaxTurns(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(text, "max turns (") && strings.HasSuffix(text, ") exceeded")
}

// assistantSystemPromptText is the default system prompt of a tool-free
// model call: it only asks for a direct answer.
func assistantSystemPromptText() string {
	return "你是 MuseForge 的分析助手。只输出结论，不要解释你的推理过程。"
}

func decodeAssistantCLIResult(final string, resultError bool, processErr error, stderr []string) (string, error) {
	if assistantCLIMaxTurns(final) {
		return "", runtime.ErrMaxTurns
	}
	if processErr != nil {
		if len(stderr) > 0 {
			return "", fmt.Errorf("%w; stderr=%s", processErr, strings.Join(stderr, " | "))
		}
		return "", processErr
	}
	final = trimAssistantReplyText(final)
	if final == "" {
		msg := ""
		if len(stderr) > 0 {
			msg = "; stderr=" + strings.Join(stderr, " | ")
		}
		return "", fmt.Errorf("%w%s", errAssistantMissingResult, msg)
	}
	if resultError {
		log.Printf("model call marked is_error=true with %d-byte structured result; using body", len(final))
	}
	return final, nil
}

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

// errAssistantCredentialRejected means CodeBuddy answered the turn with an
// authorization failure instead of model output; retrying cannot help.
var errAssistantCredentialRejected = errors.New("CodeBuddy 拒绝了你的 API Key（401/403）：请在「个人配置 → CodeBuddy API Key」确认网络环境与创建 Key 的站点一致，且填的是 CodeBuddy 的 Key，而不是 DeepSeek、OpenAI 等第三方模型的 Key")

// dropNativeArg removes a flag and its value from an argument list.
func dropNativeArg(argv []string, flag string) []string {
	out := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		if argv[i] == flag {
			i++
			continue
		}
		if strings.HasPrefix(argv[i], flag+"=") {
			continue
		}
		out = append(out, argv[i])
	}
	return out
}

func scheduleHasPersonalCredential(r *repo.Store, user domain.User, projectID, engine string) bool {
	key, _ := assistantEngineCredential(user, engine)
	return key != "" || scheduleLoginRunner(r, user.ID, projectID, engine).ID != ""
}
func assistantSessionBase() string {
	if override := strings.TrimSpace(os.Getenv("AGENTFLOW_ASSISTANT_SESSION_DIR")); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".agentflow", "assistant")
}
func assistantImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}
const workbenchAttachmentLimit = 10 << 20
func readAssistantImage(file io.Reader) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(file, workbenchAttachmentLimit+1))
	if err != nil || len(data) > workbenchAttachmentLimit {
		return nil, "", fmt.Errorf("图片附件读取失败或超过 10 MB")
	}
	mime := http.DetectContentType(data)
	if !assistantImageMIME(mime) {
		return nil, "", fmt.Errorf("图片附件格式不受支持，请使用 PNG、JPEG、WebP 或 GIF")
	}
	return data, mime, nil
}

// botExecutionUser resolves the platform account behind an IM sender.
func botExecutionUser(store *repo.Store, bot domain.NotificationBot, senderID string) (domain.User, error) {
	current, err := store.NotificationBot(bot.ID)
	if err != nil || !current.Enabled || !current.InteractionEnabled || current.BotID != bot.BotID || current.ChannelKind() != bot.ChannelKind() {
		return domain.User{}, errors.New("Bot 对话已停用")
	}
	binding, err := store.BotUserBinding(current.IdentityScope(), senderID)
	if err != nil {
		return domain.User{}, errors.New("请先绑定平台账号")
	}
	if current.ChannelKind() != domain.BotChannelWeCom && binding.ConfigID != current.ID {
		return domain.User{}, errors.New("请先绑定平台账号")
	}
	user, ok := store.User(binding.UserID)
	if !ok {
		return user, errors.New("绑定的平台账号不可用")
	}
	project, ok := store.Project(current.ProjectID)
	if !ok {
		return user, errors.New("Bot 项目不存在")
	}
	if project.AccountOwns(user.ID) {
		return user, nil
	}
	return user, errors.New("你的平台账号没有该项目的执行权限")
}
func assistantEngineBinary(kind string) string {
	switch kind {
	case assistantEngineCodeBuddy:
		return "codebuddy"
	case assistantEngineCodex:
		return "codex"
	case assistantEngineClaude:
		return "claude"
	default:
		return runtime.AgentCLIBinary(kind)
	}
}

// workbenchProjectAllowed reports whether the caller may work in a project:
// its owner.
func workbenchProjectAllowed(d *deps, c *gin.Context, projectID string, userID int64) bool {
	if projectID == "" {
		return true
	}
	p, ok := d.repo.Project(projectID)
	if !ok {
		return false
	}
	return workbenchProjectAllowsUser(p, userID)
}

// errAssistantExecutionUnavailable means the machine that owns the work is
// not reachable right now.
var errAssistantExecutionUnavailable = errors.New("conversation execution machine unavailable")

func validateUserKnowledgeRefsWithDeps(d *deps, uid int64, refs []domain.KnowledgeRef) error {
	if len(refs) == 0 {
		return nil
	}
	if err := domain.ValidateKnowledgeRefs(refs); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	svc := d.knowledgeService()
	if svc == nil {
		return nil
	}
	for _, ref := range refs {
		conn, ok := svc.GetConnector(uid, ref.ConnectorID)
		if !ok || !conn.Enabled {
			return fmt.Errorf("所选知识库连接器 %s 不存在、已停用或不属于当前用户", ref.ConnectorID)
		}
	}
	return nil
}
func repositoryBaseBranchName(rc domain.RepositoryContext) string {
	if branch := strings.TrimSpace(rc.BaseBranch); branch != "" {
		return branch
	}
	return strings.TrimSpace(rc.DefaultBranch)
}
func workbenchProjectAllowsUser(p domain.Project, userID int64) bool {
	return p.AccountOwns(userID)
}
