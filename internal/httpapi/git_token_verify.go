package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/gitwork"

	"github.com/gin-gonic/gin"
)

type gitTokenVerifyResult struct {
	Valid              bool     `json:"valid"`
	RepositoryReadable bool     `json:"repositoryReadable,omitempty"`
	Login              string   `json:"login,omitempty"`
	Scopes             []string `json:"scopes,omitempty"`
	Missing            []string `json:"missing,omitempty"`
	Error              string   `json:"error,omitempty"`
}

// verifyGitToken 校验当前登录用户的 GitHub Git Token 是否生效。
// GET 校验已保存的令牌；POST 可在 body.token 里带上尚未保存的令牌，
// 让用户填完后直接点校验。令牌只发往固定的 GitHub API 地址，不落库。
// 始终返回 200 + 结构化结果，前端据此展示校验结论。
func (h *usersHandlers) verifyGitToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gitTokenVerifyResult{Error: "valid user id is required"})
		return
	}
	current, ok := h.requestUser(c.Request)
	if !ok || current.ID != id {
		c.JSON(http.StatusForbidden, gitTokenVerifyResult{Error: "只能校验当前登录用户的凭据"})
		return
	}
	token := ""
	if c.Request.Method == http.MethodPost {
		var req struct {
			Token string `json:"token"`
		}
		if !decode(c, &req) {
			return
		}
		token = strings.TrimSpace(req.Token)
	}
	if token == "" {
		if h.repo == nil {
			c.JSON(http.StatusOK, gitTokenVerifyResult{Error: "存储不可用"})
			return
		}
		if credential, ok := h.repo.GitHTTPCredentialForHost(c.Request.Context(), id, "github.com"); ok {
			token = credential.Token
		}
	}
	_, token = gitwork.SplitToken(token)
	if token == "" {
		c.JSON(http.StatusOK, gitTokenVerifyResult{Error: "尚未配置 Git Token，请先在个人配置中填写"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, verifyGitHubToken(ctx, token))
}

func verifyGitHubToken(ctx context.Context, token string) gitTokenVerifyResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return gitTokenVerifyResult{Error: "Invalid verification request"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return gitTokenVerifyResult{Error: "Could not reach GitHub"}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return gitTokenVerifyResult{Error: fmt.Sprintf("GitHub returned HTTP %d; check your token", res.StatusCode)}
	}
	var user struct {
		Login string `json:"login"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&user) != nil || user.Login == "" {
		return gitTokenVerifyResult{Error: "Invalid GitHub response"}
	}
	// Fine-grained token permissions are repository-specific; /user validates
	// identity only. Repository access is verified when cloning or importing.
	return gitTokenVerifyResult{Valid: true, Login: user.Login}
}
