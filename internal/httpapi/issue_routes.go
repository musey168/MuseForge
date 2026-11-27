package httpapi

import (
	"agentflow/internal/domain"
	"agentflow/internal/repo"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h issuesHandlers) issueDetail(c *gin.Context, issue domain.Issue) {
	views, err := h.issueDeletionViews([]domain.Issue{issue})
	if err != nil {
		respond(c, err)
		return
	}
	images, err := h.repo.IssueImages(issue.ID)
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(200, map[string]any{"issue": views[0], "comments": h.repo.Comments(issue.ID), "images": images})
}

func (h issuesHandlers) addIssueComment(c *gin.Context, issue domain.Issue) {
	var v domain.IssueComment
	if decode(c, &v) && v.Body != "" {
		v.ID = fmt.Sprintf("comment-%d", time.Now().UnixNano())
		v.IssueID = issue.ID
		if v.AuthorType == "" {
			v.AuthorType = "member"
		}
		respond(c, h.repo.PutComment(v))
	}
}

func (h issuesHandlers) closeIssue(c *gin.Context, issue domain.Issue) {
	alreadyClosed := issue.ManuallyClosed()
	closed, err := h.repo.CloseIssue(issue.ID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repo.ErrIssueCloseBusy) {
			status = http.StatusConflict
		}
		c.JSON(status, map[string]string{"error": err.Error()})
		return
	}
	if !alreadyClosed {
		saved, err := h.appendStoredOperation(c, closed.ID, "close", "手动关闭需求")
		if err != nil {
			respond(c, err)
			return
		}
		closed = saved
	}
	h.writeIssueView(c, 200, closed)
}

// deleteIssue removes a requirement and its notes. Nothing executes it, so
// there is no run history to check first.
func (h issuesHandlers) deleteIssue(c *gin.Context, issue domain.Issue) {
	if err := h.repo.DeleteIssue(issue.ID); err != nil {
		if errors.Is(err, repo.ErrIssueNotFound) {
			c.JSON(http.StatusNotFound, map[string]string{"error": "issue not found"})
			return
		}
		respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
