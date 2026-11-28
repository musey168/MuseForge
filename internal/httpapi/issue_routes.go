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

// reopenIssue reopens a closed requirement. The personal build runs nothing,
// so reopening only moves the requirement out of the closed state and clears
// the close source. A requirement the tracker closed stays closed: the
// tracker owns that state.
func (h issuesHandlers) reopenIssue(c *gin.Context, issue domain.Issue) {
	if issue.TrackerClosed() {
		c.JSON(http.StatusConflict, map[string]string{"error": "外部工作项已关闭，请先在外部系统重新打开"})
		return
	}
	if !issue.ManuallyClosed() {
		c.JSON(http.StatusConflict, map[string]string{"error": "需求未关闭，无需重新打开"})
		return
	}
	reopened, err := h.repo.PatchIssue(issue.ID, func(v *domain.Issue) error {
		v.Status = "todo"
		v.CloseSource = ""
		return nil
	})
	if err != nil {
		respond(c, err)
		return
	}
	saved, err := h.appendStoredOperation(c, reopened.ID, "reopen", "重新打开需求")
	if err != nil {
		respond(c, err)
		return
	}
	h.writeIssueView(c, 200, saved)
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
