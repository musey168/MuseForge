package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/gitwork"

	"github.com/gin-gonic/gin"
)

// repositoriesHandlers owns the repositories a project is connected to. A
// repository is a provider-agnostic git remote: GitHub, GitLab or a plain
// HTTP(S) host. The personal build keeps repositories — a requirement links
// to one — but nothing dispatches work through them.
type repositoriesHandlers struct {
	*deps
}

// canManageProjectRepositories mirrors project settings: only the project
// owner may change its repositories.
func (h *repositoriesHandlers) canManageProjectRepositories(c *gin.Context, projectID string) bool {
	if !authRequired() {
		return true
	}
	user, ok := h.requestUser(c.Request)
	if !ok {
		c.JSON(401, gin.H{"error": "请先登录"})
		return false
	}
	if project, ok := h.repo.Project(projectID); ok && project.AccountOwns(user.ID) {
		return true
	}
	c.JSON(403, gin.H{"error": "只有项目 Owner 可以管理项目仓库"})
	return false
}

func (h *repositoriesHandlers) repositories(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.JSON(200, domain.PublicRepositories(h.repo.Repositories()))
		return
	}
	if c.Request.Method == http.MethodDelete {
		id := strings.TrimSpace(c.Query("id"))
		if id == "" {
			c.JSON(400, map[string]string{"error": "id is required"})
			return
		}
		if existing, ok := h.repo.Repository(id); ok && !h.canManageProjectRepositories(c, existing.ProjectID) {
			return
		}
		if err := h.repo.DeleteRepository(id); err != nil {
			respond(c, err)
			return
		}
		// 清除引用了该仓库的项目默认仓库设置
		for _, p := range h.repo.Projects() {
			if p.DefaultRepositoryID == id {
				p.DefaultRepositoryID = ""
				_ = h.repo.PutProject(p)
			}
		}
		c.JSON(200, map[string]string{"status": "deleted"})
		return
	}
	if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut {
		return
	}
	var body struct {
		domain.Repository
		RepoURL string `json:"repoUrl"`
		URL     string `json:"url"`
	}
	if !decode(c, &body) {
		return
	}
	v := body.Repository
	if v.ID == "" {
		v.ID = fmt.Sprintf("repo-%d", time.Now().UnixNano())
	}
	v.ID = strings.TrimSpace(v.ID)
	v.ProjectID = strings.TrimSpace(v.ProjectID)
	v.Name = strings.TrimSpace(v.Name)
	v.GongfengProjectID = strings.TrimSpace(v.GongfengProjectID)
	v.DefaultBranch = strings.TrimSpace(v.DefaultBranch)
	v.WebURL = strings.TrimSpace(v.WebURL)
	v.CloneURL = strings.TrimSpace(v.CloneURL)
	v.IntegrateBranch = domain.NormalizeIntegrateBranch(v.IntegrateBranch)
	existing, exists := h.repo.Repository(v.ID)
	if exists && !h.canManageProjectRepositories(c, existing.ProjectID) {
		return
	}
	if !h.canManageProjectRepositories(c, v.ProjectID) {
		return
	}
	// localPath copies a directory from the executing host into the task
	// workspace, so only platform administrators may set it.
	if authRequired() && !h.isAccount(c.Request) {
		v.LocalPath = ""
		if existing, ok := h.repo.Repository(v.ID); ok {
			v.LocalPath = existing.LocalPath
		}
	}
	primary := strings.TrimSpace(body.RepoURL)
	if primary == "" {
		primary = strings.TrimSpace(body.URL)
	}
	sshReference := primary
	if sshReference == "" {
		sshReference = v.CloneURL
	}
	// An explicitly supplied SSH clone address keeps SSH transport.
	githubShorthand := primary != "" && strings.HasPrefix(strings.TrimPrefix(primary, "git clone "), "github:")
	if v.Provider == domain.RepositoryProviderSSH || (gitwork.IsSSHReference(sshReference) && !githubShorthand) {
		remote, err := gitwork.ParseSSHRemote(sshReference)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Repository definitions can be saved before SSH is configured. Actual
		// Git operations validate the user's key and the pinned host keys.
		if body.RepoURL != "" && body.URL != "" {
			if other, err := gitwork.ParseSSHRemote(body.URL); err != nil || other.URL != remote.URL {
				c.JSON(http.StatusBadRequest, gin.H{"error": "repoUrl 与 url 指向不同仓库"})
				return
			}
		}
		if !validCLIBaseURL(v.WebURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仓库网页地址必须是无凭据的 HTTP(S) 地址"})
			return
		}
		v.Provider, v.CloneURL, v.GongfengProjectID = domain.RepositoryProviderSSH, remote.URL, ""
		if v.Name == "" {
			v.Name = strings.TrimSuffix(remote.Path[strings.LastIndex(remote.Path, "/")+1:], ".git")
		}
		if v.DefaultBranch == "" {
			v.DefaultBranch = "main"
		}
		h.saveRepository(c, v)
		return
	}
	// One pasted repository reference (web / SSH / clone / git clone)
	// A generic web Git URL uses Git transport without a hosting adapter.
	reference, cloneCommand := strings.CutPrefix(sshReference, "git clone ")
	reference = strings.ToLower(strings.TrimSpace(reference))
	plainHTTP := strings.HasPrefix(reference, "http://")
	_, knownHost := domain.ParseRepositoryURL(sshReference)
	if v.Provider == domain.RepositoryProviderHTTPS || v.Provider == domain.RepositoryProviderHTTP ||
		plainHTTP && cloneCommand || !knownHost && (plainHTTP || strings.HasPrefix(reference, "https://")) {
		remote, name, err := gitwork.GitHTTPRepositoryURL(sshReference)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, candidate := range []string{body.RepoURL, body.URL, v.CloneURL} {
			if candidate == "" {
				continue
			}
			other, _, err := gitwork.GitHTTPRepositoryURL(candidate)
			if err != nil || other != remote {
				c.JSON(http.StatusBadRequest, gin.H{"error": "仓库地址与 cloneUrl 不一致"})
				return
			}
		}
		if !validCLIBaseURL(v.WebURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仓库网页地址必须是不含凭据的 HTTP(S) 地址"})
			return
		}
		endpoint, _ := gitwork.GitHTTPRemoteEndpoint(remote)
		if (v.Provider == domain.RepositoryProviderHTTPS || v.Provider == domain.RepositoryProviderHTTP) && v.Provider != endpoint.Scheme {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仓库地址与所选 HTTP(S) 协议不一致"})
			return
		}
		v.Provider, v.CloneURL, v.GongfengProjectID = endpoint.Scheme, remote, ""
		if v.WebURL == "" {
			v.WebURL = strings.TrimSuffix(remote, ".git")
		}
		if v.Name == "" {
			v.Name = name
		}
		if v.DefaultBranch == "" {
			v.DefaultBranch = "main"
		}
		h.saveRepository(c, v)
		return
	}
	// One pasted repository reference (web / SSH / clone / git clone)
	// is enough. Prefer repoUrl so an SSH paste keeps an SSH clone URL.
	var parsed domain.GongfengRepoInfo
	var parsedOK bool
	for _, candidate := range []string{strings.TrimSpace(body.RepoURL), strings.TrimSpace(body.URL), v.CloneURL, v.WebURL, v.GongfengProjectID} {
		if info, ok := domain.ParseRepositoryURL(candidate); ok {
			parsed, parsedOK = info, true
			break
		}
	}
	// An explicit pasted URL is authoritative. Generated identity/web/clone
	// fields must not silently redirect a correct URL to another repository.
	if primary != "" {
		info, ok := domain.ParseRepositoryURL(primary)
		if !ok {
			c.JSON(400, gin.H{"error": "请填写完整的 GitHub、HTTP(S) 或 Git SSH 仓库地址"})
			return
		}
		if body.RepoURL != "" && body.URL != "" {
			other, ok := domain.ParseRepositoryURL(body.URL)
			if !ok || other.Project != info.Project {
				c.JSON(400, gin.H{"error": "repoUrl 与 url 指向不同仓库"})
				return
			}
		}
		parsed, parsedOK = info, true
		v.GongfengProjectID = info.Project
		v.WebURL = info.WebURL
		v.CloneURL = info.CloneURL
	}
	// A link-shaped project id that fails to parse (github.com, malformed URL)
	// is rejected loudly: storing it as a project path would silently break
	// sync, branching and MR calls. Bare paths and numeric ids stay compatible.
	if v.GongfengProjectID != "" {
		if info, ok := domain.ParseRepositoryURL(v.GongfengProjectID); ok {
			v.GongfengProjectID = info.Project
		} else if info, ok := domain.ParseGongfengRepoURL(v.GongfengProjectID); ok && !strings.ContainsAny(v.GongfengProjectID, ":@") {
			// An explicit legacy project ID is different from a pasted repoUrl.
			v.GongfengProjectID = info.Project
			if !parsedOK {
				parsed, parsedOK = info, true
			}
		} else if !strings.HasPrefix(v.GongfengProjectID, "github:") && !strings.HasPrefix(v.GongfengProjectID, domain.MRProjectGitLab) && !domain.IsSafeGongfengProjectID(v.GongfengProjectID) {
			c.JSON(400, map[string]string{"error": "仓库链接无法识别：请提供有效的 GitHub 或 GitLab 仓库地址"})
			return
		}
	}
	if parsedOK {
		if v.GongfengProjectID == "" {
			v.GongfengProjectID = parsed.Project
		}
		if v.WebURL == "" {
			v.WebURL = parsed.WebURL
		}
		if v.CloneURL == "" {
			v.CloneURL = parsed.CloneURL
		}
		if v.Name == "" {
			v.Name = parsed.Name
		}
	}
	if strings.HasPrefix(v.GongfengProjectID, domain.MRProjectGitLab) {
		// A GitLab project key is "gitlab:<host>/<group>/<repo>". The web and
		// clone addresses name the same host, so the tracker import can reach
		// the project's issues.
		_, host, path, ok := domain.ExternalMRProject(v.GongfengProjectID)
		if !ok {
			c.JSON(400, map[string]string{"error": "GitLab 项目地址无法识别：请提供 gitlab:<主机>/<分组>/<仓库>"})
			return
		}
		if v.WebURL == "" {
			v.WebURL = "https://" + host + "/" + path
		}
		if v.CloneURL == "" {
			v.CloneURL = v.WebURL + ".git"
		}
		if v.Name == "" {
			v.Name = path[strings.LastIndex(path, "/")+1:]
		}
	} else if v.GongfengProjectID != "" {
		if v.WebURL == "" {
			v.WebURL = "https://" + domain.GongfengRepositoryHost() + "/" + v.GongfengProjectID
		}
		if v.CloneURL == "" {
			v.CloneURL = v.WebURL + ".git"
		}
		if v.Name == "" {
			v.Name = v.GongfengProjectID[strings.LastIndex(v.GongfengProjectID, "/")+1:]
		}
	}
	if v.ProjectID == "" || v.Name == "" || v.GongfengProjectID == "" {
		c.JSON(400, map[string]string{"error": "请提供 projectId 和 GitHub 仓库地址 repoUrl，名称和仓库标识会自动识别"})
		return
	}
	if strings.HasPrefix(v.GongfengProjectID, "github:") {
		v.Provider = "github"
		if v.DefaultBranch == "" {
			v.DefaultBranch = "main"
		}
	}
	if strings.HasPrefix(v.GongfengProjectID, domain.MRProjectGitLab) {
		// A GitLab project key names the host and the group path, so the
		// tracker import can find the project's issues.
		v.Provider = "gitlab"
		if v.DefaultBranch == "" {
			v.DefaultBranch = "main"
		}
	}
	if v.Provider == "" {
		v.Provider = "gongfeng"
	}
	if v.Provider != "gongfeng" && v.Provider != "github" && v.Provider != "gitlab" {
		c.JSON(400, map[string]string{"error": "provider must be github, gitlab or gongfeng"})
		return
	}
	if v.DefaultBranch == "" {
		v.DefaultBranch = "master"
	}
	if domain.URLHasCredentials(v.WebURL) {
		c.JSON(400, map[string]string{"error": "webUrl 不能包含账号、密码或 Token，请去掉 @ 前的部分"})
		return
	}
	if v.CloneURL != "" {
		if domain.URLHasCredentials(v.CloneURL) {
			c.JSON(400, map[string]string{"error": "cloneUrl 不能包含账号、密码或 Token，请去掉 @ 前的部分，并在个人配置中填写 Git Token"})
			return
		}
		if err := gitwork.ValidateRemoteURL(v.CloneURL, !authRequired() || h.isAccount(c.Request)); err != nil {
			c.JSON(400, map[string]string{"error": "cloneUrl 无效：" + err.Error()})
			return
		}
	}
	h.saveRepository(c, v)
}

func (h *repositoriesHandlers) saveRepository(c *gin.Context, v domain.Repository) {
	if _, ok := h.repo.Project(v.ProjectID); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project not found"})
		return
	}
	if err := h.repo.PutRepository(v); err != nil {
		respond(c, err)
		return
	}
	stored, _ := h.repo.Repository(v.ID)
	c.JSON(200, domain.PublicRepository(stored))
}

func (h *repositoriesHandlers) repository(c *gin.Context) {
	repository, ok := h.repo.Repository(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}
	c.JSON(http.StatusOK, domain.PublicRepository(repository))
}
