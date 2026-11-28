package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"agentflow/internal/tracker"

	"github.com/gin-gonic/gin"
)

// syncHandlers exposes the requirement import from the external trackers the
// platform supports.
type syncHandlers struct {
	*deps
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
	run := h.sync.ImportTracker(h.repo, provider, req.ProjectID)
	c.JSON(http.StatusOK, map[string]any{"run": run})
}
