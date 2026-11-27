package httpapi

import (
	"net/http"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

// modelCatalogEngines lists the CLIs the caller may run with: their own keys.
func modelCatalogEngines(user domain.User) []assistantEngineOption {
	return assistantEngineOptions(user)
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
	catalog.Engines = modelCatalogEngines(user)
	if user.ExecutorDefaults != (domain.ExecutorDefaults{}) {
		catalog.Preferences = &user.ExecutorDefaults
	}
	models, _ := assistantEngineModels(ctx, user, nil)
	catalog.Models = append(selectableCodeBuddyModels(catalog.Models), models...)
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, catalog)
}
