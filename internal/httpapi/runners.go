package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	stdsync "sync"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// CLI Runner connection API (cli-connect.md).
//
// Runner-agent endpoints authenticate with their own credentials and are
// therefore exempt from user access auth: register uses the shared bootstrap
// token, everything else uses the runner's session token. Management
// endpoints stay behind user auth.
//
// 路由（gin 注册，供 server.go 参考）：
//
// 管理面（userAccessAuth 保护）:
//
//	g.GET("/api/runners", h.runners)
//	g.POST("/api/runners/register", h.runnerRegister)
//	g.POST("/api/runners/deregister", h.runnerDeregister)
//	g.POST("/api/runners/disconnect", h.runnerDisconnect)
//	g.POST("/api/runners/bind-codes", h.issueRunnerBindCode)
//	g.GET("/api/runners/default", h.runnerDefault)
//	g.PUT("/api/runners/default", h.runnerDefaultSet)
//	g.DELETE("/api/runners/:id", h.runnerRemove)
//	g.PUT("/api/runners/runtimes", h.runnerRuntimeStatus)
//	g.POST("/api/runners/runtimes", h.runnerRuntimeStatus)
//
// 机器面 /runners/*（豁免 userAccessAuth，runner 使用自有凭证）:
//
//	g.POST("/runners/register", h.runnerRegister)
//	g.POST("/runners/deregister", h.runnerDeregister)
//	g.POST("/runners/disconnect", h.runnerDisconnect)
//	g.POST("/runners/heartbeat", h.runnerHeartbeat)

const (
	runnerOfflineThreshold   = 90 * time.Second
	runnerSweepInterval      = 15 * time.Second
	runnerSessionTokenPrefix = "afr_"
	runnerSessionTTL         = 30 * 24 * time.Hour
)

type runnersHandlers struct {
	*deps
	runnerControls  *runnerControlHub
	runnerBindMu    stdsync.Mutex
	runnerBindCodes map[string]runnerBindCode
}

// ---------- background sweeper ----------

// startRunnerSweeper marks silent runners offline on the presence threshold
// recommended in cli-connect.md.
func (h *runnersHandlers) startRunnerSweeper() {
	go func() {
		ticker := time.NewTicker(runnerSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			h.repo.MarkOfflineRunners(runnerOfflineThreshold, time.Now().UTC())
		}
	}()
}

// ---------- authentication ----------

func runnerBootstrapToken() string {
	return strings.TrimSpace(os.Getenv("AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN"))
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// runnerFromSession resolves the Bearer session token issued at registration.
func (h *runnersHandlers) runnerFromSession(r *http.Request) (domain.Runner, bool) {
	raw, ok := bearerToken(r)
	if !ok || !strings.HasPrefix(raw, runnerSessionTokenPrefix) {
		return domain.Runner{}, false
	}
	sum := sha256.Sum256([]byte(raw))
	return h.repo.RunnerSessionByHash(hex.EncodeToString(sum[:]))
}

// runnerSessionAuth resolves the runner session for task-plane handlers and
// writes the 401 response when the credential is missing or invalid.
func (h *runnersHandlers) runnerSessionAuth(c *gin.Context) (domain.Runner, bool) {
	runner, ok := h.runnerFromSession(c.Request)
	if !ok {
		c.Header("WWW-Authenticate", `Bearer realm="AgentFlow Runner"`)
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid runner session"})
	}
	return runner, ok
}

func newRunnerSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plain := runnerSessionTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}

// ---------- management endpoints ----------

// 路由: g.GET("/api/runners", h.runners)
func (h *runnersHandlers) runners(c *gin.Context) {
	projectID := strings.TrimSpace(c.Query("projectId"))
	mine := strings.TrimSpace(c.Query("mine")) == "1"
	user, signedIn := h.requestUser(c.Request)
	if !mine && projectID == "" && (!signedIn || h.isAccount(c.Request)) {
		list := domain.PublicRunners(h.repo.Runners())
		if !signedIn {
			for i := range list {
				list[i].SharedProjectIDs = nil
				list[i].LoginModels = nil
			}
		}
		c.JSON(http.StatusOK, list)
		return
	}
	if !signedIn {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录后查看执行机器"})
		return
	}
	if mine {
		projectID = ""
	}
	list := h.repo.RunnersForUser(user.ID, projectID)
	for i := range list {
		list[i].Runner = runnerViewForUser(list[i].Runner, user.ID)
	}
	c.JSON(http.StatusOK, list)
}

func runnerViewForUser(runner domain.Runner, userID int64) domain.Runner {
	runner = domain.PublicRunner(runner)
	if userID > 0 && runner.OwnerUserID == userID {
		for provider, models := range runner.LoginModels {
			runner.LoginModels[provider] = runnerLoginModelsWithEfforts(provider, models)
		}
		return runner
	}
	runner.SharedProjectIDs = nil
	runner.LoginModels = nil
	runner.Labels = nil
	for i := range runner.Runtimes {
		runner.Runtimes[i].BinaryPath = ""
		runner.Runtimes[i].AuthMode = domain.RuntimeAuthPersonal
	}
	return runner
}

// runnerDefault returns the user's default execution machine (执行机器).
// 路由: g.GET("/api/runners/default", h.runnerDefault)
func (h *runnersHandlers) runnerDefault(c *gin.Context) {
	user, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "请先登录后管理执行机器"})
		return
	}
	projectID := strings.TrimSpace(c.Query("projectId"))
	var defaults repo.RunnerDefaults
	if c.Query("conversation") == "1" {
		defaults = h.repo.ResolveConversationRunnerDefaults(user.ID, projectID)
	} else {
		defaults = h.repo.ResolveRunnerDefaults(user.ID, projectID)
	}
	runner := defaults.Runner
	platform, err := h.platformRunnerStatus(c.Request.Context(), projectID, h.isAccount(c.Request))
	if err != nil {
		respond(c, err)
		return
	}
	result := map[string]any{
		"runnerId":       runner.ID,
		"runner":         runnerViewForUser(runner, user.ID),
		"online":         runner.Status == domain.RunnerStatusOnline,
		"cloudPool":      gin.H{"configured": platform.CloudPool, "online": platform.CloudPoolOnline},
		"platformRunner": platform,
		// minRunnerVersion lets the runners page disable「设为默认」with the
		// same rule PUT /api/runners/default enforces.
		"minRunnerVersion":                  domain.RunnerVersionDefaultSelection,
		"minPrivilegedSandboxRunnerVersion": domain.RunnerVersionPrivilegedSandbox,
	}
	if projectID != "" {
		result["personalRunnerId"] = defaults.PersonalRunnerID
		result["projectRunnerId"] = defaults.ProjectRunnerID
		result["projectRunnerMode"] = defaults.ProjectRunnerMode
	}
	c.JSON(http.StatusOK, result)
}

func (h *runnersHandlers) platformRunnerStatus(ctx context.Context, projectID string, canEdit bool) (domain.PlatformRunnerStatus, error) {
	loads, err := h.repo.LoadSnapshots(nil)
	if err != nil {
		return domain.PlatformRunnerStatus{}, err
	}
	allowed, err := h.repo.PlatformRunnerPrivilegedSandbox(ctx)
	if err != nil {
		return domain.PlatformRunnerStatus{}, err
	}
	load := loads[repo.PlatformAdmissionBucketKey]
	status := domain.PlatformRunnerStatus{
		Name: domain.PlatformRunnerName, Status: domain.RunnerStatusOffline,
		RunLimit: h.repo.PlatformRunnerRunLimit(), CanEditLimit: canEdit, Load: &load,
		AllowPrivilegedSandbox: allowed,
	}
	return status, nil
}

func runnerPreferenceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repo.ErrRunnerNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "执行机器不存在"})
	case errors.Is(err, repo.ErrRunnerShareForbidden), errors.Is(err, repo.ErrRunnerProjectDenied):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// runnerRemove unbinds one of the caller's own offline machines, for a machine
// that is gone for good and can no longer deregister itself.
// 路由: g.DELETE("/api/runners/:id", h.runnerRemove)
func (h *runnersHandlers) runnerRemove(c *gin.Context) {
	user, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "请先登录后管理执行机器"})
		return
	}
	runner, found := h.repo.Runner(strings.TrimSpace(c.Param("id")))
	if !found {
		c.JSON(http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if runner.OwnerUserID != user.ID {
		c.JSON(http.StatusForbidden, map[string]string{"error": "只能移除自己绑定的执行机器"})
		return
	}
	if runner.Status == domain.RunnerStatusOnline {
		c.JSON(http.StatusConflict, map[string]string{"error": "机器在线，请先在该机器上执行卸载命令，或停止后再移除"})
		return
	}
	if err := h.repo.DeregisterRunner(runner.ID, time.Now().UTC()); err != nil && err != repo.ErrRunnerNotFound {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// runnerDefaultSet selects the user's personal default or a project override.
// 路由: g.PUT("/api/runners/default", h.runnerDefaultSet)
func (h *runnersHandlers) runnerDefaultSet(c *gin.Context) {
	user, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "请先登录后管理执行机器"})
		return
	}
	var input struct {
		RunnerID string `json:"runnerId"`
		Mode     string `json:"mode"`
	}
	if !decode(c, &input) {
		return
	}
	runnerID := strings.TrimSpace(input.RunnerID)
	projectID := strings.TrimSpace(c.Query("projectId"))
	mode := strings.TrimSpace(input.Mode)
	switch mode {
	case "", domain.RunnerSelectionModeAuto, domain.RunnerSelectionModePlatform:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的项目执行模式"})
		return
	}
	if mode != "" {
		if projectID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持按项目配置执行模式"})
			return
		}
		if runnerID != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "执行模式与指定执行机器只能选择一项"})
			return
		}
	}
	if runnerID != "" {
		runner, found := h.repo.Runner(runnerID)
		if !found {
			c.JSON(http.StatusNotFound, map[string]string{"error": "执行机器不存在"})
			return
		}
		if projectID == "" && runner.OwnerUserID != user.ID {
			c.JSON(http.StatusForbidden, map[string]string{"error": "只能选择自己绑定的执行机器"})
			return
		}
		if projectID != "" && !h.repo.CanUseRunner(runner, projectID, user.ID) {
			runnerPreferenceError(c, repo.ErrRunnerProjectDenied)
			return
		}
		if reason := runner.DefaultSelectionBlocker(); reason != "" {
			c.JSON(http.StatusConflict, map[string]string{"error": reason})
			return
		}
	}
	if projectID != "" {
		if err := h.repo.SetProjectRunnerPreference(user.ID, projectID, runnerID, mode); err != nil {
			runnerPreferenceError(c, err)
			return
		}
		message := "已设为你在此项目的默认执行机器"
		switch {
		case mode == domain.RunnerSelectionModeAuto:
			message = "已开启此项目 LOOP 自动分配"
		case mode == domain.RunnerSelectionModePlatform:
			message = "已设为此项目使用平台执行机器"
		case runnerID == "":
			message = "已清除此项目的默认执行机器"
		}
		defaults := h.repo.ResolveRunnerDefaults(user.ID, projectID)
		c.JSON(http.StatusOK, gin.H{"runnerId": defaults.Runner.ID, "projectRunnerMode": defaults.ProjectRunnerMode, "message": message})
		return
	}
	if err := h.repo.SetDefaultRunner(user.ID, runnerID, time.Now().UTC()); err != nil {
		if err == repo.ErrRunnerNotFound {
			c.JSON(http.StatusNotFound, map[string]string{"error": "执行机器不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if runnerID == "" {
		c.JSON(http.StatusOK, gin.H{"runnerId": "", "projectRunnerMode": "", "message": "已切回云端执行"})
	} else {
		c.JSON(http.StatusOK, gin.H{"runnerId": runnerID, "projectRunnerMode": "", "message": "已设为默认执行机器，你的任务和沙箱将在这台机器上执行"})
	}
}

// runnerRuntimeStatus updates per-provider runtime status/auth mode. The
// trailing sub-path of the legacy /api/runners/runtimes/... form was always
// ignored, so this is registered as a flat PUT|POST route.
// 路由: g.PUT("/api/runners/runtimes", h.runnerRuntimeStatus)
//
//	g.POST("/api/runners/runtimes", h.runnerRuntimeStatus)
func (h *runnersHandlers) runnerRuntimeStatus(c *gin.Context) {
	var input struct {
		RunnerID string `json:"runnerId"`
		Provider string `json:"provider"`
		Status   string `json:"status"`
		AuthMode string `json:"authMode"`
	}
	if !decode(c, &input) {
		return
	}
	input.RunnerID = strings.TrimSpace(input.RunnerID)
	input.Provider = strings.TrimSpace(input.Provider)
	input.Status = strings.TrimSpace(input.Status)
	input.AuthMode = strings.TrimSpace(input.AuthMode)
	if input.RunnerID == "" || input.Provider == "" {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "runnerId 和 provider 不能为空"})
		return
	}
	if input.Status == "" && input.AuthMode == "" {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "status 和 authMode 至少提供一个"})
		return
	}
	user, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "请先登录后管理执行机器"})
		return
	}
	runner, found := h.repo.Runner(input.RunnerID)
	if !found {
		c.JSON(http.StatusNotFound, map[string]string{"error": "执行机器不存在"})
		return
	}
	if runner.OwnerUserID != user.ID {
		c.JSON(http.StatusForbidden, map[string]string{"error": "只能管理自己绑定的执行机器"})
		return
	}
	now := time.Now().UTC()
	if input.Status != "" {
		if err := h.repo.SetRunnerRuntimeStatus(input.RunnerID, input.Provider, input.Status, now); err != nil {
			if err == repo.ErrRunnerNotFound {
				c.JSON(http.StatusNotFound, map[string]string{"error": "执行机器不存在"})
				return
			}
			c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	if input.AuthMode != "" {
		if err := h.repo.SetRunnerRuntimeAuthMode(input.RunnerID, input.Provider, input.AuthMode, now); err != nil {
			if err == repo.ErrRunnerNotFound {
				c.JSON(http.StatusNotFound, map[string]string{"error": "执行机器不存在"})
				return
			}
			c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	runner, _ = h.repo.Runner(input.RunnerID)
	c.JSON(http.StatusOK, domain.PublicRunner(runner))
}

// runnerDisconnect marks a daemon offline without deleting its identity,
// session, or the user's default-runner selection. A normal service restart
// therefore reconnects with the existing session instead of needing a new
// one-time bind code. Explicit uninstall continues to use deregister.
// 路由: g.POST("/api/runners/disconnect", h.runnerDisconnect)
//
//	g.POST("/runners/disconnect", h.runnerDisconnect)
func (h *runnersHandlers) runnerDisconnect(c *gin.Context) {
	runner, ok := h.runnerSessionAuth(c)
	if !ok {
		return
	}
	if err := h.repo.MarkRunnerOffline(runner.ID, time.Now().UTC()); err != nil {
		if err == repo.ErrRunnerNotFound {
			c.JSON(http.StatusOK, map[string]bool{"ok": true})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// ---------- one-time bind codes ----------

// runnerBindCodeTTL keeps the copy-pasted onboarding command valid long
// enough to reach a terminal without leaving a standing credential behind.
const runnerBindCodeTTL = 10 * time.Minute

const runnerBindCodePrefix = "bind-"

type runnerBindCode struct {
	userID    int64
	expiresAt time.Time
}

// consumeRunnerBindCode resolves and burns a one-time bind code generated by
// the 执行器 page. Burning on match keeps codes strictly single-use: a leaked
// command cannot be replayed, and the user simply refreshes the page for a
// new one.
func (h *runnersHandlers) consumeRunnerBindCode(code string) (int64, bool) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, runnerBindCodePrefix) {
		return 0, false
	}
	h.runnerBindMu.Lock()
	defer h.runnerBindMu.Unlock()
	now := time.Now().UTC()
	entry, exists := h.runnerBindCodes[code]
	if !exists || !entry.expiresAt.After(now) {
		if exists {
			delete(h.runnerBindCodes, code)
		}
		return 0, false
	}
	delete(h.runnerBindCodes, code)
	return entry.userID, true
}

// newRunnerBindCode issues a one-time code that registers the machine under
// the requesting user. Called from the 执行器 page; requires an interactive
// login so the machine's owner is unambiguous.
// 路由: g.POST("/api/runners/bind-codes", h.issueRunnerBindCode)
func (h *runnersHandlers) issueRunnerBindCode(c *gin.Context) {
	user, ok := h.requestUser(c.Request)
	if !ok || user.ID <= 0 {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "请先登录后生成接入命令"})
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "生成绑定码失败"})
		return
	}
	code := runnerBindCodePrefix + base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	h.runnerBindMu.Lock()
	// Opportunistically drop expired codes so the map cannot grow unbounded.
	for key, entry := range h.runnerBindCodes {
		if !entry.expiresAt.After(now) {
			delete(h.runnerBindCodes, key)
		}
	}
	h.runnerBindCodes[code] = runnerBindCode{userID: user.ID, expiresAt: now.Add(runnerBindCodeTTL)}
	h.runnerBindMu.Unlock()
	c.JSON(http.StatusOK, map[string]any{"code": code, "expiresAt": now.Add(runnerBindCodeTTL)})
}

// ---------- registration ----------

// resolveRunnerRegistrant accepts, in order: a one-time bind code from the
// 执行器 page (binds the machine to that user), the shared server bootstrap
// token, or a user personal token (afp_).
func (h *runnersHandlers) resolveRunnerRegistrant(r *http.Request) (ownerUserID int64, ok bool) {
	provided := strings.TrimSpace(r.Header.Get("X-Bootstrap-Token"))
	if provided == "" {
		if raw, bearerOK := bearerToken(r); bearerOK {
			provided = raw
		}
	}
	if provided == "" {
		return 0, false
	}
	if userID, matched := h.consumeRunnerBindCode(provided); matched {
		return userID, true
	}
	if bootstrap := runnerBootstrapToken(); bootstrap != "" && constantTimeEqual(provided, bootstrap) {
		return 0, true
	}
	if user, userOK := h.requestUser(r); userOK && user.ID > 0 {
		return user.ID, true
	}
	return 0, false
}

// runnerDeregister permanently removes an uninstalled machine. Ordinary
// daemon shutdown uses runnerDisconnect so service restarts retain identity.
// 路由: g.POST("/api/runners/deregister", h.runnerDeregister)
//
//	g.POST("/runners/deregister", h.runnerDeregister)
func (h *runnersHandlers) runnerDeregister(c *gin.Context) {
	runner, ok := h.runnerSessionAuth(c)
	if !ok {
		return
	}
	if err := h.repo.DeregisterRunner(runner.ID, time.Now().UTC()); err != nil {
		if err == repo.ErrRunnerNotFound {
			c.JSON(http.StatusOK, map[string]bool{"ok": true})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// 路由: g.POST("/api/runners/register", h.runnerRegister)
//
//	g.POST("/runners/register", h.runnerRegister)
func (h *runnersHandlers) runnerRegister(c *gin.Context) {
	provided := strings.TrimSpace(c.GetHeader("X-Bootstrap-Token"))
	if provided == "" {
		if raw, bearerOK := bearerToken(c.Request); bearerOK {
			provided = raw
		}
	}
	owner, authorized := h.resolveRunnerRegistrant(c.Request)
	if !authorized {
		// No credential at all and no shared bootstrap token configured:
		// registration is closed. A presented-but-invalid credential (for
		// example an expired or replayed bind code) is always 401 so the
		// machine-side message tells the user what to do next.
		if provided == "" && runnerBootstrapToken() == "" {
			c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "runner registration is disabled: set AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN on the server"})
			return
		}
		c.Header("WWW-Authenticate", `Bearer realm="AgentFlow Runner"`)
		if strings.HasPrefix(provided, runnerBindCodePrefix) {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "绑定码无效、已使用或已过期：请在「个人 → 执行器」页面刷新后重新复制命令"})
			return
		}
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid bind token"})
		return
	}
	var input struct {
		RunnerID               string                 `json:"runnerId"`
		Name                   string                 `json:"name"`
		OS                     string                 `json:"os"`
		Arch                   string                 `json:"arch"`
		Version                string                 `json:"version"`
		Labels                 []string               `json:"labels"`
		MaxConcurrency         int                    `json:"maxConcurrency"`
		Runtimes               []domain.RunnerRuntime `json:"runtimes"`
		NativeSandboxAvailable bool                   `json:"nativeSandboxAvailable"`
	}
	if !decode(c, &input) {
		return
	}
	if input.MaxConcurrency < 1 {
		input.MaxConcurrency = 1
	}
	if input.MaxConcurrency > 32 {
		input.MaxConcurrency = 32
	}
	// Only the original binding may reuse an ID. Platform bootstrap
	// credentials can renew platform runners, whose owner is zero.
	now := time.Now().UTC()
	runner := domain.Runner{
		Name:                   strings.TrimSpace(input.Name),
		OS:                     strings.TrimSpace(input.OS),
		Arch:                   strings.TrimSpace(input.Arch),
		RunnerVersion:          strings.TrimSpace(input.Version),
		Labels:                 input.Labels,
		MaxConcurrency:         input.MaxConcurrency,
		Status:                 domain.RunnerStatusOnline,
		Runtimes:               normalizeRunnerRuntimes(input.Runtimes, nil),
		NativeSandboxAvailable: input.NativeSandboxAvailable,
		OwnerUserID:            owner,
		LastSeenAt:             &now,
	}
	existing, found := h.repo.Runner(strings.TrimSpace(input.RunnerID))
	renew := found && existing.OwnerUserID == owner
	if renew {
		runner.ID = existing.ID
		runner.CreatedAt = existing.CreatedAt
		runner.Runtimes = normalizeRunnerRuntimes(input.Runtimes, existing.Runtimes)
	} else {
		runner.ID = "runner-" + randomID()
	}
	plain, hash, err := newRunnerSessionToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "生成 session token 失败"})
		return
	}
	expires := now.Add(runnerSessionTTL)
	runner.SessionTokenHash = hash
	runner.SessionExpiresAt = &expires
	if err := h.repo.RegisterRunner(runner, renew); err != nil {
		if errors.Is(err, repo.ErrRunnerNotFound) {
			c.JSON(http.StatusConflict, gin.H{"error": "runner registration changed; retry"})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"runnerId":          runner.ID,
		"sessionToken":      plain,
		"expiresAt":         expires,
		"heartbeatInterval": 30 * time.Second,
	})
}

func normalizeRunnerRuntimes(reported, existing []domain.RunnerRuntime) []domain.RunnerRuntime {
	preferences := map[string]domain.RunnerRuntime{}
	for _, rt := range existing {
		preferences[strings.ToLower(strings.TrimSpace(rt.Provider))] = rt
	}
	out := append([]domain.RunnerRuntime(nil), reported...)
	for i := range out {
		previous := preferences[strings.ToLower(strings.TrimSpace(out[i].Provider))]
		if previous.Status == domain.RuntimeStatusDisabled {
			out[i].Status = domain.RuntimeStatusDisabled
		}
		if domain.IsAgentCLIKind(out[i].Provider) && !domain.SupportsRunnerLogin(out[i].Provider) {
			out[i].AuthMode = domain.RuntimeAuthPersonal
		} else if previous.AuthMode == domain.RuntimeAuthLocal || previous.AuthMode == domain.RuntimeAuthPersonal {
			out[i].AuthMode = previous.AuthMode
		} else if strings.EqualFold(out[i].Provider, "codebuddy") {
			out[i].AuthMode = domain.RuntimeAuthPersonal
		} else {
			out[i].AuthMode = domain.RuntimeAuthLocal
		}
	}
	return out
}

// ---------- heartbeat ----------

// 路由: g.POST("/api/runners/heartbeat", h.runnerHeartbeat)
//
//	g.POST("/runners/heartbeat", h.runnerHeartbeat)
func (h *runnersHandlers) runnerHeartbeat(c *gin.Context) {
	runner, ok := h.runnerSessionAuth(c)
	if !ok {
		return
	}
	var input struct {
		Status                 string                 `json:"status"`
		RunningTasks           int                    `json:"runningTasks"`
		Version                string                 `json:"version"`
		Runtimes               []domain.RunnerRuntime `json:"runtimes"`
		NativeSandboxAvailable bool                   `json:"nativeSandboxAvailable"`
	}
	if !decode(c, &input) {
		return
	}
	now := time.Now().UTC()
	expires := now.Add(runnerSessionTTL)
	if err := h.repo.RunnerHeartbeat(runner.ID, input.Runtimes, strings.TrimSpace(input.Version), input.NativeSandboxAvailable, now, expires); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"ok": true, "serverTime": now, "expiresAt": expires})
}

func isRunnerAgentPath(path string) bool {
	if strings.HasPrefix(path, "/runners/") {
		return true
	}
	if !strings.HasPrefix(path, "/api/runners/") {
		return false
	}
	rest := strings.Trim(strings.TrimPrefix(path, "/api/runners/"), "/")
	// Only the machine lifecycle endpoints authenticate with a runner
	// credential; everything else stays behind user auth.
	switch rest {
	case "register", "deregister", "disconnect", "heartbeat":
		return true
	}
	return false
}
