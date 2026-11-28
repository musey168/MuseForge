package httpapi

import (
	"net/http"
	"strings"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

// accountHandlers owns the platform's single account. The personal build has
// no sign-in: the console asks the platform for its account once, on the first
// visit, and the account is what the personal settings, the model catalogue
// and the reporting credentials hang off.
type accountHandlers struct {
	*deps
}

// account names the platform's one account, creating it when the platform has
// none yet. Calling it again returns the account that already exists.
func (h accountHandlers) account(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "只支持 POST"})
		return
	}
	var input struct {
		LoginName string `json:"loginName"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是合法的 JSON"})
			return
		}
	}
	loginName := strings.TrimSpace(input.LoginName)
	if len(loginName) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号名最长 64 字节"})
		return
	}
	account, err := h.repo.CreateAccount(loginName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, domain.PublicUsers([]domain.User{account})[0])
}
