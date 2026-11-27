package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

const runnerRuntimeTestTimeout = 30 * time.Second

func (h *runnersHandlers) runnerRuntimeTest(c *gin.Context) {
	current, authenticated := h.requestUser(c.Request)
	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录后测试执行机器"})
		return
	}
	var input struct {
		RunnerID string `json:"runnerId"`
		Provider string `json:"provider"`
	}
	if !decode(c, &input) {
		return
	}
	provider := strings.TrimSpace(input.Provider)
	if provider != assistantEngineCodeBuddy && !slices.Contains(assistantCLIEngines(), provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 CLI"})
		return
	}
	runner, found := h.repo.Runner(strings.TrimSpace(input.RunnerID))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "执行机器不存在"})
		return
	}
	if runner.OwnerUserID != current.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "只能测试自己绑定的执行机器"})
		return
	}
	if runner.Status != domain.RunnerStatusOnline {
		c.JSON(http.StatusConflict, gin.H{"error": "执行机器离线，请启动 Runner 后重试"})
		return
	}
	if !runner.SupportsCLISettings() {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("执行机器需升级至 %s 才能测试 CLI", domain.RunnerVersionCLISettings)})
		return
	}
	if !runner.SupportsProvider(provider) {
		c.JSON(http.StatusConflict, gin.H{"error": "该 CLI 未安装或已停用"})
		return
	}
	user, found := h.repo.User(current.ID)
	if !found {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录后测试执行机器"})
		return
	}
	runnerLogin := runnerUsesLocalLogin(runner, provider)
	if key, _ := assistantEngineCredential(user, provider); key == "" && !runnerLogin {
		c.JSON(http.StatusBadRequest, gin.H{"error": assistantEngineMissingKey(provider)})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), runnerRuntimeTestTimeout)
	defer cancel()
	if err := testRunnerCLI(ctx, h.runnerControls, runner, provider, runnerLogin, user); err != nil {
		c.JSON(runnerCLITestFailure(ctx, err), gin.H{"error": runnerCLITestMessage(ctx, err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
