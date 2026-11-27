package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

// requireAccountWrite admits a write that comes from the platform's single
// account.
//
// The personal build runs as one person. There is no project membership, no
// member role and no platform administrator, so the guard asks only "is this
// the account" — once, here, for project-scoped and platform-wide writes
// alike. Callers with no project IDs get the platform-wide form.
func (d deps) requireAccountWrite(c *gin.Context, projectIDs ...string) bool {
	if !authRequired() {
		return true
	}
	user, ok := d.requestUser(c.Request)
	if !ok {
		// A request without a user identity carries its own authorization
		// (service tokens, the ingestion endpoint).
		return true
	}
	for _, id := range projectIDs {
		if !d.accountHoldsProject(id, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "只有项目负责人可以修改该项目的内容"})
			return false
		}
	}
	return true
}

// accountHoldsProject reports whether the platform's single account holds the
// project. A project nobody owns stays open to the account: there is no other
// account it could belong to.
func (d deps) accountHoldsProject(projectID string, userID int64) bool {
	if projectID == "" || userID <= 0 {
		return true
	}
	p, found := d.repo.Project(projectID)
	if !found {
		return true
	}
	return p.AccountOwns(userID)
}

// Read access does not grant permission to execute agents in a run workspace.
// Keep the same predicate for HTTP writes and assistant execution preparation.
func (d deps) projectWriterAllowed(c *gin.Context, projectID string, userID int64) bool {
	if !authRequired() || projectID == "" {
		return true
	}
	return d.accountHoldsProject(projectID, userID)
}

// speakerAccountWriter applies the project write predicate to the speaker of an
// agent tool session, which carries the user instead of an HTTP request.
func (d deps) speakerAccountWriter(projectID string, user domain.User, userID int64) bool {
	if !authRequired() || projectID == "" {
		return true
	}
	return d.accountHoldsProject(projectID, userID)
}

// requireWriteOrigin admits a same-origin browser write; the session cookie
// alone is not enough.
func requireWriteOrigin(c *gin.Context) bool {
	origin, err := url.Parse(c.GetHeader("Origin"))
	if err != nil || origin.Host != c.Request.Host || (origin.Scheme != "http" && origin.Scheme != "https") {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid request origin"})
		return false
	}
	return true
}

// assetWriteGuard applies project membership to catalog assets, project MCP
// servers and schedule jobs, and limits platform-wide assets and sync settings
// to administrators.
func (d deps) assetWriteGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || !authRequired() {
			c.Next()
			return
		}
		path := c.FullPath()
		if !d.allowAssetWrite(c, path) {
			c.Abort()
			return
		}
		c.Next()
	}
}

func (d deps) allowAssetWrite(c *gin.Context, path string) bool {
	switch path {
	case "/api/sync/config", "/api/sync/now":
		return d.requireAccountWrite(c)
	case "/api/sync/create":
		var body struct {
			IssueID string `json:"issueId"`
		}
		peekJSON(c, &body)
		issue, _ := d.repo.Issue(strings.TrimSpace(body.IssueID))
		return d.requireAccountWrite(c, issue.ProjectID)
	case "/api/tracker/import":
		// A project-scoped import needs a writer of that project; an import
		// without one covers every project, so it needs the account.
		var body struct {
			ProjectID string `json:"projectId"`
		}
		peekJSON(c, &body)
		if project := strings.TrimSpace(body.ProjectID); project != "" {
			return d.requireAccountWrite(c, project)
		}
		return d.requireAccountWrite(c, "")
	case "/api/mcp-market/install", "/api/mcp-servers/custom":
		var body struct {
			Scope     string `json:"scope"`
			ProjectID string `json:"projectId"`
		}
		peekJSON(c, &body)
		if strings.TrimSpace(body.Scope) == "personal" {
			return true
		}
		if strings.TrimSpace(body.Scope) != "project" {
			return d.requireAccountWrite(c)
		}
		return d.requireAccountWrite(c, strings.TrimSpace(body.ProjectID))
	}
	return true
}

// peekJSON decodes the request body without consuming it for the handler.
func peekJSON(c *gin.Context, v any) {
	if c.Request.Body == nil {
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err == nil {
		_ = json.Unmarshal(raw, v)
	}
}
