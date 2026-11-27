package httpapi

import (
	"net/http"
	"strings"

	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// modelCatalogEngines lists the CLIs the caller may run with: their own keys
// and — for a scheduled AI job — the account login on their default machine.
func modelCatalogEngines(r *repo.Store, user domain.User, projectID string, schedule bool) []assistantEngineOption {
	options := assistantEngineOptions(user)
	if schedule {
		for i := range options {
			if scheduleLoginRunner(r, user.ID, projectID, options[i].ID).ID != "" {
				options[i].Configured = true
			}
		}
	}
	return options
}

// modelCatalog serves the personal model catalogue: the CLIs the caller may
// run with and the models each of them offers. It is the catalogue the
// personal settings and the task pickers read.
func (h *Server) modelCatalog(c *gin.Context) {
	authUser, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	user, ok := h.repo.User(authUser.ID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	ctx := c.Request.Context()
	catalog := personalCodeBuddyCatalog(ctx, user)
	catalog.Engines = modelCatalogEngines(h.repo, user, strings.TrimSpace(c.Query("projectId")), strings.TrimSpace(c.Query("schedule")) == "1")
	if user.ExecutorDefaults != (domain.ExecutorDefaults{}) {
		catalog.Preferences = &user.ExecutorDefaults
	}
	models, _ := assistantEngineModels(ctx, user, nil)
	catalog.Models = append(selectableCodeBuddyModels(catalog.Models), models...)
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, catalog)
}
