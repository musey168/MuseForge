package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"agentflow/internal/domain"
	"agentflow/internal/tracker"

	"github.com/gin-gonic/gin"
)

// syncHandlers 暴露外部同步（Gongfeng/TAPD）的配置与触发端点。
type syncHandlers struct {
	*deps
}

func (h *syncHandlers) syncConfig(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.JSON(200, h.repo.SyncConfig())
		return
	}
	if c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPost {
		var cfg domain.SyncConfig
		if !decode(c, &cfg) {
			return
		}
		if cfg.AutoSyncIntervalSec <= 0 {
			cfg.AutoSyncIntervalSec = 300
		}
		current := h.repo.SyncConfig()
		cfg.LastSyncAt = current.LastSyncAt
		cfg.LastSyncStatus = current.LastSyncStatus
		cfg.LastSyncError = current.LastSyncError
		cfg.PlaybookSeedRev = current.PlaybookSeedRev
		// TAPD workspace/iteration live on projects; sync_config keeps platform MCP + auto-sync only.
		cfg.TAPDWorkspaceID = ""
		cfg.TAPDIterationID = ""
		h.configureSync(cfg)
		h.sync.StartAutoSync(h.repo)
		if err := h.repo.PutSyncConfig(cfg); err != nil {
			respond(c, err)
			return
		}
		c.JSON(200, h.repo.SyncConfig())
		return
	}
}

func (h *syncHandlers) syncNow(c *gin.Context) {
	// Reload MCP auth tokens from ~/.codebuddy/mcp.json in case they refreshed
	cfg := h.repo.SyncConfig()
	h.configureSync(cfg)
	go func() {
		if err := h.sync.PullAll(h.repo); err != nil {
			_ = err // logged in sync service
		}
	}()
	c.JSON(200, map[string]any{"status": "started", "syncing": true})
}

func (h *syncHandlers) syncLogs(c *gin.Context) {
	c.JSON(200, h.sync.Logs())
}

// trackerImport triggers a requirement import from an external tracker and
// reports the outcome of the last run.
//
// GET  /api/tracker/import?provider=github — the stored outcome of the last
// POST /api/tracker/import, so the outcome survives a restart.
// POST /api/tracker/import — imports now and answers with the run.
func (h *syncHandlers) trackerImport(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		provider := strings.ToLower(strings.TrimSpace(c.Query("provider")))
		if provider == "" {
			provider = "github"
		}
		run, ok := h.repo.LatestTrackerImportRun(provider)
		if !ok {
			c.JSON(http.StatusOK, map[string]any{"run": nil})
			return
		}
		c.JSON(http.StatusOK, map[string]any{"run": run})
		return
	}
	var req struct {
		Provider  string `json:"provider"`
		ProjectID string `json:"projectId"`
	}
	if !decode(c, &req) {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "github"
	}
	if !slices.Contains(tracker.TrackerProviders(), provider) {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported tracker provider " + provider})
		return
	}
	if h.sync == nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "外部同步未配置"})
		return
	}
	h.configureSync(h.repo.SyncConfig())
	run := h.sync.ImportTracker(h.repo, provider, req.ProjectID)
	c.JSON(http.StatusOK, map[string]any{"run": run})
}

func (h *syncHandlers) syncStatus(c *gin.Context) {
	c.JSON(200, map[string]any{"syncing": h.sync.IsSyncing(), "config": h.repo.SyncConfig(), "logs": h.sync.Logs()})
}

func (h *syncHandlers) syncCreate(c *gin.Context) {
	var req struct {
		IssueID string `json:"issueId"`
		Target  string `json:"target"` // gongfeng, tapd
	}
	if !decode(c, &req) {
		return
	}
	if req.IssueID == "" || req.Target == "" {
		c.JSON(400, map[string]string{"error": "issueId and target are required"})
		return
	}
	issue, ok := h.repo.Issue(req.IssueID)
	if !ok {
		c.JSON(404, map[string]string{"error": "issue not found"})
		return
	}
	if (req.Target == "tapd" || req.Target == "gongfeng") && strings.TrimSpace(issue.TapdID) != "" {
		c.JSON(http.StatusConflict, map[string]string{"error": "该平台需求已手动关联 TAPD ID；如需新建外部单据，请先清空统计关联 ID"})
		return
	}
	if req.Target == "gongfeng" && issue.RepositoryID == "" {
		if project, found := h.repo.Project(issue.ProjectID); found {
			issue.RepositoryID = project.DefaultRepositoryID
		}
	}
	var updated domain.Issue
	var err error
	// Reload auth tokens before pushing
	h.configureSync(h.repo.SyncConfig())
	switch req.Target {
	case "gongfeng":
		updated, err = h.sync.CreateGongfengIssue(h.repo, issue)
	case "tapd":
		updated, err = h.sync.CreateTAPDStory(h.repo, issue)
	default:
		c.JSON(400, map[string]string{"error": "target must be gongfeng or tapd"})
		return
	}
	if err != nil {
		respond(c, err)
		return
	}
	stored, _ := h.repo.Issue(updated.ID)
	c.JSON(200, stored)
}
