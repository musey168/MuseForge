package httpapi

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"agentflow/internal/auth"
	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

const (
	issueListMaxLimit     = 200
	issueListDefaultLimit = 50
)

type issueListPage struct {
	Items   []issueDeletionView `json:"items"`
	Total   int                 `json:"total"`
	Offset  int                 `json:"offset"`
	Limit   int                 `json:"limit"`
	HasMore bool                `json:"hasMore"`
}

// issuesHandlers 提供服务端的个人需求表。
type issuesHandlers struct {
	*deps
}

func (h issuesHandlers) issues(c *gin.Context) {
	r := c.Request
	if r.Method == http.MethodGet {
		items := filterIssues(h.repo.Issues(), issueListQuery{
			ProjectID: strings.TrimSpace(c.Query("projectId")),
			Status:    strings.TrimSpace(c.Query("status")),
			Source:    strings.TrimSpace(c.Query("source")),
			Tag:       strings.TrimSpace(c.Query("tag")),
			Q:         strings.TrimSpace(c.Query("q")),
		})
		if _, ok := c.GetQuery("limit"); !ok {
			views, err := h.issueDeletionViews(items)
			if err != nil {
				respond(c, err)
				return
			}
			c.JSON(200, views)
			return
		}
		limit, err := parseIssueListInt(c.Query("limit"), issueListDefaultLimit)
		if err != nil {
			c.JSON(400, gin.H{"error": "limit 必须是整数"})
			return
		}
		if limit < 1 {
			limit = issueListDefaultLimit
		}
		if limit > issueListMaxLimit {
			limit = issueListMaxLimit
		}
		offset := 0
		if raw, exists := c.GetQuery("offset"); exists && strings.TrimSpace(raw) != "" {
			offset, err = parseIssueListInt(raw, 0)
			if err != nil || offset < 0 {
				c.JSON(400, gin.H{"error": "offset 必须是非负整数"})
				return
			}
		}
		total := len(items)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := items[offset:end]
		if page == nil {
			page = []domain.Issue{}
		}
		views, err := h.issueDeletionViews(page)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(200, issueListPage{Items: views, Total: total, Offset: offset, Limit: limit, HasMore: end < total})
		return
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		var req issueWriteRequest
		files, ok := decodeIssueWrite(c, &req)
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if !ok {
			return
		}
		v := req.Issue
		branchesProvided := v.RepositoryBranches != nil
		existing, found := h.repo.Issue(v.ID)
		if found {
			if !h.requireAccountWrite(c, v.ProjectID, existing.ProjectID) {
				return
			}
			v = mergeIssueEdit(v, existing)
		} else if !h.requireAccountWrite(c, v.ProjectID) {
			return
		}
		if len(files) > 0 && v.ID != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "图片只能随新需求一起保存到平台"})
			return
		}
		// Clients such as the AI assistant echo a fetched issue back. Resending
		// the stored branch is not a new selection, so legacy records holding a
		// shared branch normalize below instead of failing validation.
		if found && req.BaseBranch != nil && strings.TrimSpace(*req.BaseBranch) != "" && strings.TrimSpace(*req.BaseBranch) == strings.TrimSpace(existing.BaseBranch) {
			req.BaseBranch = nil
		}
		// An explicit empty branch restores automatic branch creation; an omitted
		// field preserves the selection on edits made by older clients.
		if req.BaseBranch != nil {
			v.BaseBranch = *req.BaseBranch
			if !branchesProvided {
				v.RepositoryBranches = nil
			}
		}
		if found && req.BaseBranch == nil && !branchesProvided {
			h.inheritIssueRepositoryBranches(&v, existing)
		}
		if found && req.BaseBranch == nil {
			h.normalizeLegacyIssueBaseBranch(&v)
		}
		if v.ProjectID == "" || v.Title == "" {
			c.JSON(400, map[string]string{"error": "projectId and title are required"})
			return
		}
		if v.Status == "" {
			v.Status = "backlog"
		}
		if v.Priority == "" {
			v.Priority = "medium"
		}
		if _, ok := h.repo.Project(v.ProjectID); !ok {
			c.JSON(400, map[string]string{"error": "project not found"})
			return
		}
		if err := h.bindIssueRepositories(&v); err != nil {
			c.JSON(400, map[string]string{"error": err.Error()})
			return
		}
		if found {
			keepExistingTrackerBinding(&v, existing)
		} else {
			// Only the server records audit entries for a newly created issue.
			v.Operations = nil
			keepExistingTrackerBinding(&v, domain.Issue{})
		}
		if !found && !repo.IssueBoundToTracker(v) {
			if v.Kind == "" {
				v.Kind = "requirement"
			}
			v.Source = "local"
			v.ExternalRef = nil
		}
		if v.ID == "" {
			v.ID = fmt.Sprintf("issue-%d", time.Now().UnixNano())
		}
		if !found {
			h.recordIssueOperation(c, &v, "create", "创建需求")
		} else if detail := issueChangeDetail(existing, v); detail != "" {
			h.recordIssueOperation(c, &v, "update", detail)
		}
		var err error
		if len(files) > 0 {
			images, ok := readIssueImages(c, files)
			if !ok {
				return
			}
			err = h.repo.CreateIssueWithImages(v, images)
		} else {
			err = h.repo.PutIssue(v)
		}
		if err != nil {
			respond(c, err)
			return
		}
		stored, _ := h.repo.Issue(v.ID)
		h.writeIssueView(c, 200, stored)
		return
	}
}

// mergeIssueEdit fills fields the editor left empty from the stored issue.
// The editor payload does not round-trip the audit log, so the server copy
// of Operations always wins.
func mergeIssueEdit(v, existing domain.Issue) domain.Issue {
	if v.ProjectID == "" {
		v.ProjectID = existing.ProjectID
	}
	if v.Title == "" {
		v.Title = existing.Title
	}
	if v.Source == "" {
		v.Source = existing.Source
	}
	if v.ExternalRef == nil {
		v.ExternalRef = existing.ExternalRef
	}
	if len(v.RepositoryIDs) == 0 && v.RepositoryID == "" {
		v.RepositoryIDs = existing.RepositoryIDs
		v.RepositoryID = existing.RepositoryID
	}
	if v.BaseBranch == "" {
		v.BaseBranch = existing.BaseBranch
	}
	if v.Kind == "" {
		v.Kind = existing.Kind
	}
	v.Operations = append([]domain.IssueOperation(nil), existing.Operations...)
	return v
}

// issueRoute is one /api/issues/:id sub-resource. Routes are registered with
// Any, so unsupported methods fall back to http.NotFound (404 text/plain).
type issueRoute struct {
	handle  func(issuesHandlers, *gin.Context, domain.Issue)
	methods []string
}

var issueRoutes = map[string]issueRoute{
	"/api/issues/:id":                 {handle: issuesHandlers.issueDetail, methods: []string{http.MethodGet, http.MethodDelete}},
	"/api/issues/:id/images/:imageId": {handle: issuesHandlers.issueImage, methods: []string{http.MethodGet}},
	"/api/issues/:id/comments":        {handle: issuesHandlers.addIssueComment, methods: []string{http.MethodPost}},
	"/api/issues/:id/close":           {handle: issuesHandlers.closeIssue, methods: []string{http.MethodPost}},
	"/api/issues/:id/reopen":          {handle: issuesHandlers.reopenIssue, methods: []string{http.MethodPost}},
}

func (h issuesHandlers) issue(c *gin.Context) {
	issue, ok := h.repo.Issue(c.Param("id"))
	if !ok {
		c.JSON(404, map[string]string{"error": "issue not found"})
		return
	}
	route, ok := issueRoutes[c.FullPath()]
	if !ok || !slices.Contains(route.methods, c.Request.Method) {
		http.NotFound(c.Writer, c.Request)
		return
	}
	if c.Request.Method != http.MethodGet && !h.requireAccountWrite(c, issue.ProjectID) {
		return
	}
	if c.FullPath() == "/api/issues/:id" && c.Request.Method == http.MethodDelete {
		h.deleteIssue(c, issue)
		return
	}
	route.handle(h, c, issue)
}

func actorLabel(user *auth.User) (string, int64) {
	if user == nil {
		return "", 0
	}
	name := strings.TrimSpace(user.LoginName)
	cn := strings.TrimSpace(user.ChineseName)
	switch {
	case name != "" && cn != "":
		return name + "(" + cn + ")", user.ID
	case cn != "":
		return cn, user.ID
	default:
		return name, user.ID
	}
}

// recordIssueOperation appends one audit entry. Actor is always the platform
// account whose permissions were used.
func (h issuesHandlers) recordIssueOperation(c *gin.Context, issue *domain.Issue, action, detail string) {
	if issue == nil {
		return
	}
	user, _ := h.requestUser(c.Request)
	actor, id := actorLabel(user)
	op := domain.IssueOperation{Actor: actor, ActorUserID: id, Action: action, Detail: detail}
	issue.AppendOperation(op)
}

func (h issuesHandlers) appendStoredOperation(c *gin.Context, issueID, action, detail string) (domain.Issue, error) {
	stored, ok := h.repo.Issue(issueID)
	if !ok {
		return domain.Issue{}, fmt.Errorf("issue not found")
	}
	h.recordIssueOperation(c, &stored, action, detail)
	if err := h.repo.PutIssue(stored); err != nil {
		return domain.Issue{}, err
	}
	saved, _ := h.repo.Issue(issueID)
	return saved, nil
}

func issueStatusName(status string) string {
	switch status {
	case "backlog":
		return "待规划"
	case "todo":
		return "待处理"
	case "in_progress":
		return "进行中"
	case "in_review":
		return "评审中"
	case "done":
		return "已完成"
	case "closed":
		return "已关闭"
	case "blocked":
		return "已阻塞"
	case "cancelled":
		return "已取消"
	default:
		if strings.TrimSpace(status) == "" {
			return "未设置"
		}
		return status
	}
}

func issueChangeDetail(before, after domain.Issue) string {
	var parts []string
	if before.Status != after.Status && after.Status != "" {
		parts = append(parts, "状态 "+issueStatusName(before.Status)+" → "+issueStatusName(after.Status))
	}
	if strings.TrimSpace(before.Title) != strings.TrimSpace(after.Title) && strings.TrimSpace(after.Title) != "" {
		parts = append(parts, "标题")
	}
	if strings.TrimSpace(before.Description) != strings.TrimSpace(after.Description) {
		parts = append(parts, "描述")
	}
	if before.BaseBranch != after.BaseBranch {
		if after.BaseBranch == "" {
			parts = append(parts, "工作分支改为自动新建")
		} else {
			parts = append(parts, "继续已有工作分支 "+after.BaseBranch)
		}
	}
	if !maps.Equal(before.RepositoryBranches, after.RepositoryBranches) {
		parts = append(parts, "各仓库继续开发分支")
	}
	if before.Priority != after.Priority && after.Priority != "" {
		parts = append(parts, "优先级")
	}
	if !slices.Equal(before.Tags, after.Tags) {
		parts = append(parts, "标签")
	}
	return strings.Join(parts, "、")
}

func (h issuesHandlers) bindIssueRepositories(v *domain.Issue) error {
	ids := v.EffectiveRepositoryIDs()
	if len(ids) == 0 {
		if project, ok := h.repo.Project(v.ProjectID); ok && strings.TrimSpace(project.DefaultRepositoryID) != "" {
			ids = []string{project.DefaultRepositoryID}
		}
	}
	repositories := make([]domain.Repository, 0, len(ids))
	for _, id := range ids {
		repository, ok := h.repo.Repository(id)
		if !ok || repository.ProjectID != v.ProjectID {
			return fmt.Errorf("repository must belong to issue project")
		}
		repositories = append(repositories, repository)
	}
	if err := normalizeIssueRepositoryBranches(v, repositories); err != nil {
		return err
	}
	if len(ids) > 0 {
		v.RepositoryIDs = ids
		v.RepositoryID = ids[0]
	}
	return nil
}

type issueListQuery struct {
	ProjectID string
	Status    string
	Source    string
	Tag       string
	Q         string
}

func filterIssues(items []domain.Issue, q issueListQuery) []domain.Issue {
	out := make([]domain.Issue, 0, len(items))
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	for _, item := range items {
		if q.ProjectID != "" && item.ProjectID != q.ProjectID {
			continue
		}
		if !csvContainsFold(q.Status, item.Status) {
			continue
		}
		if !csvContainsFold(q.Source, item.Source) {
			continue
		}
		if tag := strings.TrimSpace(q.Tag); tag != "" && !item.HasTag(tag) {
			continue
		}
		if q.Q != "" && !issueMatchesQuery(item, needle) {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		if out[i].Number != out[j].Number {
			return out[i].Number > out[j].Number
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// issueMatchesQuery reports whether the lower-cased, trimmed needle appears in
// the issue's searchable text or fuzzily matches its title.
func issueMatchesQuery(item domain.Issue, needle string) bool {
	hay := strings.ToLower(item.Title + " " + item.ID + " " + strconv.Itoa(item.Number))
	if item.ExternalRef != nil {
		hay += " " + strings.ToLower(item.ExternalRef.ExternalID)
	}
	return strings.Contains(hay, needle) || fuzzyIssueTitleMatch(item.Title, needle)
}

func fuzzyIssueTitleMatch(title, query string) bool {
	normalizedTitle := normalizeIssueTitleSearch(title)
	normalizedQuery := normalizeIssueTitleSearch(query)
	if normalizedQuery != "" && strings.Contains(normalizedTitle, normalizedQuery) {
		return true
	}
	tokens := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if normalized := normalizeIssueTitleSearch(token); normalized == "" || !strings.Contains(normalizedTitle, normalized) {
			return false
		}
	}
	return true
}

func normalizeIssueTitleSearch(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func csvContainsFold(csv, value string) bool {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, part := range strings.Split(csv, ",") {
		if strings.ToLower(strings.TrimSpace(part)) == value {
			return true
		}
	}
	return false
}

func parseIssueListInt(raw string, defaultVal int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultVal, nil
	}
	return strconv.Atoi(raw)
}

func keepExistingTrackerBinding(v *domain.Issue, existing domain.Issue) {
	repo.RestoreTrackerBinding(v, existing)
}
