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

var guardedRunWrites = map[string]bool{
	"/api/runs/:id":                    true,
	"/api/runs/:id/approve":            true,
	"/api/runs/:id/reject":             true,
	"/api/runs/:id/retry":              true,
	"/api/runs/:id/decline-retry":      true,
	"/api/runs/:id/cancel":             true,
	"/api/runs/:id/pause":              true,
	"/api/runs/:id/resume":             true,
	"/api/runs/:id/refresh-sop":        true,
	"/api/runs/:id/pass":               true,
	"/api/runs/:id/eval":               true,
	"/api/runs/:id/eval/apply":         true,
	"/api/runs/:id/workspace/takeover": true,
	"/api/runs/:id/workspace/release":  true,
	"/api/runs/:id/workspace/restore":  true,
	"/api/runs/:id/workspace/file":     true,
	"/api/runs/:id/workspace/terminal": true,
	"/api/runs/:id/preview/backend":    true,
	"/api/runs/:id/preview":            true,
}

// runWriteGuard limits personal workspace routes to their owner and applies
// project write access to run controls, workspace edits and terminals.
func (d deps) runWriteGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, path := c.Param("id"), c.FullPath()
		if id == "" || !strings.HasPrefix(path, "/api/runs/:id") {
			c.Next()
			return
		}
		run, err := d.repo.RunHeader(id)
		if err != nil {
			respond(c, err)
			c.Abort()
			return
		}
		if run.IsPersonalWorkspace() {
			user, ok := d.requestUser(c.Request)
			if !ok || user.ID <= 0 || user.ID != run.ExecutionContext.ResponsibleUserID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问个人工作区"})
				return
			}
		}
		if run.ID == "" || c.Request.Method == http.MethodGet || !guardedRunWrites[path] {
			c.Next()
			return
		}
		issueProject := ""
		if issue, found := d.repo.Issue(run.ExecutionContext.IssueID); found {
			issueProject = issue.ProjectID
		}
		if !d.requireAccountWrite(c, domain.RunProjectID(run, issueProject)) {
			c.Abort()
			return
		}
		c.Next()
	}
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

// assetOwner returns the project of a stored asset and whether it is a
// platform-wide asset that only administrators may change.
func (d deps) assetOwner(kind, id string) (project string, global bool, found bool) {
	if id == "" {
		return "", false, false
	}
	switch kind {
	case "rules":
		v, ok := d.repo.Rule(id)
		return v.ProjectID, v.ProjectID == "", ok
	case "skills":
		v, ok := d.repo.Skill(id)
		return v.ProjectID, v.ProjectID == "", ok
	case "sandboxes":
		if strings.EqualFold(id, repo.DefaultSandboxProfileID) {
			return "", true, true
		}
		v, ok := d.repo.Sandbox(id)
		return v.ProjectID, v.ProjectID == "" || v.ID == repo.DefaultSandboxProfileID, ok
	case "schedule-jobs":
		v, ok := d.repo.ScheduleJob(id)
		return v.ProjectID, false, ok
	// TODO(knowledge-legacy-compat): remove with legacy /api/knowledge/submissions routes.
	case "knowledge":
		v, ok := d.repo.KnowledgeSubmission(id)
		return v.ProjectID, false, ok
	}
	return "", false, false
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
	case "/api/schedule-jobs/:id", "/api/schedule-jobs/:id/run":
		project, _, _ := d.assetOwner("schedule-jobs", c.Param("id"))
		return d.requireAccountWrite(c, project)
	// TODO(knowledge-legacy-compat): remove with legacy /api/knowledge/submissions routes.
	case "/api/knowledge/submissions":
		var body struct {
			ProjectID string `json:"projectId"`
		}
		peekJSON(c, &body)
		return d.requireAccountWrite(c, strings.TrimSpace(body.ProjectID))
	case "/api/knowledge/submissions/:id", "/api/knowledge/submissions/:id/publish", "/api/knowledge/submissions/:id/reject":
		project, _, _ := d.assetOwner("knowledge", c.Param("id"))
		return d.requireAccountWrite(c, project)
	case "/api/skills/:id/files", "/api/skills/:id/files/*path":
		return d.allowStoredAsset(c, "skills", c.Param("id"))
	}
	kind := strings.TrimPrefix(path, "/api/")
	switch kind {
	case "rules", "skills", "sandboxes", "schedule-jobs":
	default:
		return true
	}
	if c.Request.Method == http.MethodDelete {
		return d.allowStoredAsset(c, kind, strings.TrimSpace(c.Query("id")))
	}
	var body struct {
		ID        string `json:"id"`
		ProjectID string `json:"projectId"`
	}
	peekJSON(c, &body)
	id, project := strings.TrimSpace(body.ID), strings.TrimSpace(body.ProjectID)
	if kind == "sandboxes" {
		reserved, err := d.repo.IsReservedSandboxID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return false
		}
		if reserved {
			return d.requireAccountWrite(c)
		}
	}
	if !d.allowStoredAsset(c, kind, id) {
		return false
	}
	if project == "" && kind != "schedule-jobs" {
		if storedProject, _, found := d.assetOwner(kind, id); found && storedProject != "" {
			return true
		}
		return d.requireAccountWrite(c)
	}
	return d.requireAccountWrite(c, project)
}

func (d deps) allowStoredAsset(c *gin.Context, kind, id string) bool {
	project, global, found := d.assetOwner(kind, id)
	if !found {
		return true
	}
	if global {
		return d.requireAccountWrite(c)
	}
	return d.requireAccountWrite(c, project)
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
