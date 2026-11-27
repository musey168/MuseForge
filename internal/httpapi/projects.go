package httpapi

import (
	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type projectsHandlers struct{ *deps }

func (h *projectsHandlers) projects(c *gin.Context) {
	r := c.Request
	if r.Method == http.MethodGet {
		c.JSON(200, domain.PublicProjects(h.repo.Projects()))
		return
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		var body struct {
			domain.Project
			TAPDClear bool `json:"tapdClear"`
		}
		if !decode(c, &body) {
			return
		}
		if body.ID == "" || body.Name == "" {
			c.JSON(400, map[string]string{"error": "id and name are required"})
			return
		}
		v := body.Project
		existing, exists := h.repo.Project(v.ID)
		if !exists {
			if err := domain.ValidateResourceID(v.ID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "项目" + err.Error()})
				return
			}
		}
		if exists && existing.ArchivedAt != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "项目「" + existing.Name + "」已删除，标识 " + existing.ID + " 仍被占用；请换一个标识，或先恢复该项目"})
			return
		}
		v.ArchivedAt, v.ArchivedBy = nil, 0
		if authRequired() {
			user, ok := h.requestUser(r, true)
			if !ok {
				c.JSON(401, gin.H{"error": "请先登录"})
				return
			}
			if !h.isPlatformAdmin(r) {
				if exists && existing.OwnerID != user.ID {
					c.JSON(403, gin.H{"error": "只有项目 Owner 或平台管理员可以修改项目设置"})
					return
				}
				if !exists {
					if v.OwnerID == 0 {
						v.OwnerID = user.ID
					}
					if v.OwnerID != user.ID {
						c.JSON(403, gin.H{"error": "新项目 Owner 必须为当前用户"})
						return
					}
				}
			}
		}
		isNew := true
		if existing, ok := h.repo.Project(v.ID); ok {
			isNew = false
			if v.OwnerID == 0 {
				v.OwnerID = existing.OwnerID
			}
			if v.SOPBindings == nil {
				v.SOPBindings = existing.SOPBindings
			}
			if v.SOPPlaybooks == nil {
				v.SOPPlaybooks = existing.SOPPlaybooks
			}
		}
		if err := h.applyProjectPlaybook(&v); err != nil {
			c.JSON(400, map[string]string{"error": err.Error()})
			return
		}
		// A brand-new project starts with the built-in LoopForge backend
		// delivery LOOP when none was picked, so synced and newly created
		// requirements run out of the box (see repo.bindDefaultWorkflow).
		if isNew && strings.TrimSpace(v.WorkflowID) == "" {
			v.WorkflowID = repo.GenericBackendDeliveryWorkflowID
		}
		if v.OwnerID > 0 {
			if _, ok := h.repo.User(v.OwnerID); !ok {
				c.JSON(400, map[string]string{"error": "project owner must reference an existing user"})
				return
			}
		}
		if v.DefaultRepositoryID != "" {
			repo, ok := h.repo.Repository(v.DefaultRepositoryID)
			if !ok || repo.ProjectID != v.ID {
				c.JSON(400, map[string]string{"error": "default repository must belong to project"})
				return
			}
		}
		if v.Key == "" {
			v.Key = strings.ToUpper(v.ID)
		}
		v.TAPDWorkspaceID = strings.TrimSpace(v.TAPDWorkspaceID)
		v.TAPDIterationID = strings.TrimSpace(v.TAPDIterationID)
		if existing, ok := h.repo.Project(v.ID); ok {
			if body.TAPDClear {
				v.TAPDWorkspaceID = ""
				v.TAPDIterationID = ""
			} else {
				if v.TAPDWorkspaceID == "" {
					v.TAPDWorkspaceID = existing.TAPDWorkspaceID
				}
				if v.TAPDIterationID == "" {
					v.TAPDIterationID = existing.TAPDIterationID
				}
			}
		} else {
			if body.TAPDClear {
				v.TAPDWorkspaceID = ""
				v.TAPDIterationID = ""
			}
		}
		if existing, ok := h.repo.Project(v.ID); ok && v.RunEval.IsZero() {
			v.RunEval = existing.RunEval
		} else if !v.RunEval.IsZero() {
			v.RunEval = domain.EffectiveRunEvalPolicy(v.RunEval)
		}
		if err := h.repo.PutProject(v); err != nil {
			respond(c, err)
			return
		}
		if isNew {
			if err := ensureDefaultProjectMCPs(h.repo, v.ID); err != nil {
				log.Printf("ensure default MCPs for project %s: %v", v.ID, err)
			} else {
				h.configureSync(h.repo.SyncConfig())
			}
		}
		stored, _ := h.repo.Project(v.ID)
		c.JSON(200, domain.PublicProject(stored))
		return
	}
}

func (h *projectsHandlers) applyProjectPlaybook(project *domain.Project) error {
	if project == nil {
		return nil
	}
	if project.SOPPlaybooks != nil {
		books := domain.NormalizeSOPPlaybooks(project.SOPPlaybooks)
		for i := range books {
			bindings, err := normalizeSOPBindings(h.repo, project.ID, books[i].Bindings)
			if err != nil {
				return err
			}
			books[i].Bindings = bindings
		}
		project.SOPPlaybooks = books
		if book, ok := project.DefaultPlaybook(); ok && len(book.Bindings) > 0 {
			project.WorkflowID = book.Bindings[0].WorkflowID
		}
		return nil
	}
	if project.SOPBindings == nil {
		return nil
	}
	bindings, err := normalizeSOPBindings(h.repo, project.ID, project.SOPBindings)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		project.SOPBindings = nil
		return nil
	}
	project.SOPBindings = bindings
	project.WorkflowID = bindings[0].WorkflowID
	return nil
}

func (h *projectsHandlers) project(c *gin.Context) {
	project, ok := h.repo.Project(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	c.JSON(http.StatusOK, domain.PublicProject(project))
}

// deleteProject hides a project (soft delete). Requirements, runs and
// settings are kept so restoreProject can bring it back. Running work and
// enabled schedules must be stopped first so nothing keeps acting on a
// project nobody can see.
func (h *projectsHandlers) deleteProject(c *gin.Context) {
	p, uid, ok := h.projectForArchive(c)
	if !ok {
		return
	}
	if p.ArchivedAt != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在或已删除"})
		return
	}
	if blockers := h.projectDeleteBlockers(p.ID); len(blockers) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "项目「" + p.Name + "」还有" + strings.Join(blockers, "、") + "，请先停止后再删除"})
		return
	}
	now := time.Now().UTC()
	if err := h.repo.SetProjectArchived(p.ID, &now, uid); err != nil {
		respond(c, err)
		return
	}
	stored, _ := h.repo.Project(p.ID)
	c.JSON(http.StatusOK, domain.PublicProject(stored))
}

func (h *projectsHandlers) restoreProject(c *gin.Context) {
	p, _, ok := h.projectForArchive(c)
	if !ok {
		return
	}
	if p.ArchivedAt != nil {
		if err := h.repo.SetProjectArchived(p.ID, nil, 0); err != nil {
			respond(c, err)
			return
		}
	}
	stored, _ := h.repo.Project(p.ID)
	c.JSON(http.StatusOK, domain.PublicProject(stored))
}

func (h *projectsHandlers) projectForArchive(c *gin.Context) (domain.Project, int64, bool) {
	p, ok := h.repo.Project(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return domain.Project{}, 0, false
	}
	if !authRequired() {
		return p, 0, true
	}
	user, ok := h.requestUser(c.Request, true)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return domain.Project{}, 0, false
	}
	if p.OwnerID != user.ID && !h.isPlatformAdmin(c.Request) {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有项目 Owner 或平台管理员可以删除或恢复项目"})
		return domain.Project{}, 0, false
	}
	return p, user.ID, true
}

func (h *projectsHandlers) projectDeleteBlockers(projectID string) []string {
	var out []string
	runs, err := h.repo.RunSummariesFor(domain.AnalyticsFilter{ProjectIDs: []string{projectID}}, "")
	if err != nil {
		return []string{"运行记录无法读取"}
	}
	active := 0
	for _, run := range h.repo.FilterRunsByProject(runs, projectID) {
		if !domain.TerminalRunStatus(run.Status) {
			active++
		}
	}
	if active > 0 {
		out = append(out, strconv.Itoa(active)+" 个未结束的运行")
	}
	var jobs []string
	for _, job := range h.repo.ScheduleJobs() {
		if job.Enabled && job.ProjectID == projectID {
			jobs = append(jobs, "「"+job.Name+"」")
		}
	}
	if len(jobs) > 0 {
		out = append(out, "启用中的定时任务"+strings.Join(jobs, ""))
	}
	return out
}
