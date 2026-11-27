package httpapi

import (
	"errors"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"agentflow/internal/domain"
	"agentflow/internal/repo"

	"github.com/gin-gonic/gin"
)

type usersHandlers struct {
	*deps
}

func (h *usersHandlers) users(c *gin.Context) {
	if h.repo == nil {
		c.JSON(http.StatusOK, []domain.User{})
		return
	}
	c.JSON(http.StatusOK, domain.PublicUsers(h.repo.Users()))
}

func (h *usersHandlers) user(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "valid user id is required"})
		return
	}
	current, ok := h.requestUser(c.Request)
	if !ok || current.ID != id {
		c.JSON(http.StatusForbidden, map[string]string{"error": "只能配置当前登录用户的凭据"})
		return
	}
	var body repo.UserCredentialUpdate
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if raw := body.CodeBuddyInternetEnvironment; raw != nil {
		environment, ok := domain.NormalizeCodeBuddyInternetEnvironment(*raw)
		if !ok {
			c.JSON(http.StatusBadRequest, map[string]string{"error": "CodeBuddy 网络环境只能是国际站（留空）、国内站（internal）或 iOA（ioa）"})
			return
		}
		body.CodeBuddyInternetEnvironment = &environment
	}
	baseURLs := []*string{body.CodeBuddyBaseURL, body.CodexBaseURL, body.ClaudeCodeBaseURL}
	for _, cred := range body.AgentCLICredentials {
		baseURLs = append(baseURLs, cred.BaseURL)
	}
	for _, raw := range baseURLs {
		if raw != nil && !validCLIBaseURL(*raw) {
			c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidCLIBaseURL})
			return
		}
	}
	if defaults := body.ExecutorDefaults; defaults != nil {
		defaults.Executor = strings.TrimSpace(defaults.Executor)
		defaults.Model = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(defaults.Model), defaults.Executor+":"))
		defaults.Effort = strings.TrimSpace(defaults.Effort)
		if defaults.Model != "" && (!assistantValidEngineModel(defaults.Model) || defaults.Model == assistantDefaultModel ||
			defaults.Executor == assistantEngineCodeBuddy && codeBuddyIsDefaultModel(defaults.Model)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择具体的默认模型"})
			return
		}
		if defaults.Effort != "" {
			if defaults.Model == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "请先选择默认模型"})
				return
			}
			user, _ := h.repo.User(id)
			if err := validateAssistantEffort(c.Request.Context(), user, defaults.Executor+":"+defaults.Model, defaults.Effort); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}
	saved, err := h.repo.UpdateUserCredentials(id, body)
	if err != nil {
		if errors.Is(err, repo.ErrAssistantDefaultExecutorUnavailable) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, domain.PublicUser(saved))
}

const errInvalidCLIBaseURL = "Base URL 必须为 HTTP(S) 服务地址，不能包含账号密码、查询参数或片段"

// validCLIBaseURL accepts an empty value (the CLI's official endpoint) or a
// plain HTTP(S) service address.
func validCLIBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && len(raw) <= 2048
}

