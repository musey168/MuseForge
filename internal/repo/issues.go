package repo

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// ---------- Issues ----------

// IssueAnalyticsViews loads the columns analytics needs without hydrating the
// full issue payload (clarification, comments, and delivery lock blobs).
func (r *IssuesRepo) IssueAnalyticsViews() []domain.Issue {
	type liteRow struct {
		ID                     string    `gorm:"column:id"`
		ProjectID              string    `gorm:"column:project_id"`
		Title                  string    `gorm:"column:title"`
		Status                 string    `gorm:"column:status"`
		Kind                   string    `gorm:"column:kind"`
		Source                 string    `gorm:"column:source"`
		WorkflowID             string    `gorm:"column:workflow_id"`
		AssigneeUserID         *int64    `gorm:"column:assignee_user_id"`
		ExternalID             string    `gorm:"column:external_id"`
		ExternalURL            string    `gorm:"column:external_url"`
		ClarificationStartedAt string    `gorm:"column:clarification_started_at"`
		DeliveryState          string    `gorm:"column:delivery_state"`
		CreatedAt              time.Time `gorm:"column:created_at"`
	}
	var rows []liteRow
	if err := r.DB.Table("issues").Select(
		"id", "project_id", "title", "status", "kind", "source", "workflow_id", "assignee_user_id", "external_id", "created_at",
		r.sqlPayloadJSONText("externalRef.externalUrl")+" AS external_url",
		r.sqlPayloadJSONText("clarification.startedAt")+" AS clarification_started_at",
		r.sqlPayloadJSONText("deliveryState")+" AS delivery_state",
	).Find(&rows).Error; err != nil {
		return []domain.Issue{}
	}
	out := make([]domain.Issue, 0, len(rows))
	for _, row := range rows {
		issue := domain.Issue{
			ID:            row.ID,
			ProjectID:     row.ProjectID,
			Title:         row.Title,
			Status:        row.Status,
			Kind:          row.Kind,
			Source:        row.Source,
			WorkflowID:    row.WorkflowID,
			DeliveryState: strings.TrimSpace(row.DeliveryState),
			CreatedAt:     row.CreatedAt,
		}
		if started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(row.ClarificationStartedAt)); err == nil {
			issue.Clarification.StartedAt = started
		}
		if row.AssigneeUserID != nil {
			issue.AssigneeUserID = *row.AssigneeUserID
		}
		extID := strings.TrimSpace(row.ExternalID)
		extURL := strings.TrimSpace(row.ExternalURL)
		if extID != "" || extURL != "" {
			issue.ExternalRef = &domain.ExternalRef{Source: issue.Source, ExternalID: extID, ExternalURL: extURL}
		}
		out = append(out, issue)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *IssuesRepo) Issues() []domain.Issue {
	var rows []issueRow
	if err := r.DB.Find(&rows).Error; err != nil {
		return []domain.Issue{}
	}
	out := make([]domain.Issue, 0, len(rows))
	for _, row := range rows {
		var issue domain.Issue
		if jsonUnmarshal(row.Payload, &issue) != nil {
			continue
		}
		if row.AssigneeUserID != nil {
			issue.AssigneeUserID = *row.AssigneeUserID
		} else {
			issue.AssigneeUserID = 0
		}
		out = append(out, issue)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectID == out[j].ProjectID {
			return out[i].Number > out[j].Number
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	if len(out) == 0 {
		// 与 master 保持同一 wire 行为：空结果序列化为 null 而非 []。
		return nil
	}
	return out
}

func (r *IssuesRepo) Issue(id string) (domain.Issue, bool) {
	var row issueRow
	if err := r.DB.Where("id = ?", id).Take(&row).Error; err != nil {
		return domain.Issue{}, false
	}
	var issue domain.Issue
	if jsonUnmarshal(row.Payload, &issue) != nil {
		return domain.Issue{}, false
	}
	if row.AssigneeUserID != nil {
		issue.AssigneeUserID = *row.AssigneeUserID
	} else {
		issue.AssigneeUserID = 0
	}
	return issue, true
}

func (r *IssuesRepo) PutIssue(v domain.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.putIssueLocked(v)
}

// PutSyncedIssue refreshes tracker data without changing an existing issue's
// local lifecycle. Reload under the write lock so an edit made after the
// tracker snapshot was read is preserved too.
func (r *IssuesRepo) PutSyncedIssue(v domain.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveIssueLocked(v, issueWriteSynced, false)
}

// PutImportedIssue writes a requirement a tracker import merged. The status
// and the close source are the merged ones, taken verbatim: the tracker layer
// owns the rule that decides whether the tracker or the user owns the
// status, and the repo layer must not second-guess it. A local write would
// re-force "closed" on every requirement the tracker closed, which would
// undo a status the user set by hand.
func (r *IssuesRepo) PutImportedIssue(v domain.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveIssueLocked(v, issueWriteImported, false)
}

// PatchIssue loads the current row, applies fn, and writes it back under the
// same lock as PutIssue so a clarification job cannot clobber a concurrent save.
func (r *IssuesRepo) PatchIssue(id string, fn func(*domain.Issue) error) (domain.Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Issue{}, errors.New("issue id is required")
	}
	if fn == nil {
		return domain.Issue{}, errors.New("patch function is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	issue, ok := r.Issue(id)
	if !ok {
		return domain.Issue{}, ErrIssueNotFound
	}
	if err := fn(&issue); err != nil {
		return issue, err
	}
	if err := r.saveIssueLocked(issue, issueWriteLocal, true); err != nil {
		return issue, err
	}
	stored, ok := r.Issue(id)
	if ok {
		return stored, nil
	}
	return issue, nil
}

func (r *IssuesRepo) putIssueLocked(v domain.Issue) error {
	return r.saveIssueLocked(v, issueWriteLocal, false)
}

// issueWriteMode says how a write treats the requirement's local lifecycle.
type issueWriteMode int

const (
	// issueWriteLocal is a user edit: the repo layer re-derives the status
	// from the tracker state, as it always has.
	issueWriteLocal issueWriteMode = iota
	// issueWriteSynced is a tracker refresh that must not touch the local
	// lifecycle: the stored status wins.
	issueWriteSynced
	// issueWriteImported is a tracker import whose status the tracker layer
	// already merged: the status and the close source are taken verbatim.
	issueWriteImported
)

func (r *IssuesRepo) saveIssueLocked(v domain.Issue, mode issueWriteMode, existingOnly bool) error {
	v, err := r.prepareIssueForSave(v, mode, existingOnly)
	if err != nil {
		return err
	}
	if existingOnly {
		result := r.DB.Model(&issueRow{}).Where("id = ?", v.ID).Updates(issueValues(v))
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrIssueNotFound
		}
		return result.Error
	}
	return r.writeIssue(v)
}

func (r *IssuesRepo) prepareIssueForSave(v domain.Issue, mode issueWriteMode, existingOnly bool) (domain.Issue, error) {
	now := time.Now().UTC()
	old, ok := r.Issue(v.ID)
	if existingOnly && !ok {
		return v, ErrIssueNotFound
	}
	preserveStatus := mode == issueWriteSynced && ok
	if ok {
		if preserveStatus {
			v.Status = old.Status
			v.CloseSource = old.CloseSource
			v.AutomationState = old.AutomationState
		}
		v.CreatedAt = old.CreatedAt
		if v.Number == 0 {
			v.Number = old.Number
		}
		if strings.TrimSpace(v.AutomationState) == "" {
			v.AutomationState = old.AutomationState
			// sopMode=sop lifts the no-LOOP choice, and with it the "ignored"
			// that choice set, as binding or running a LOOP from the page does.
			if old.WithoutSOP() && strings.EqualFold(strings.TrimSpace(v.SOPMode), domain.IssueSOPUse) && v.AutomationState == "ignored" {
				v.AutomationState = "eligible"
			}
		}
		if len(v.Operations) == 0 && len(old.Operations) > 0 {
			v.Operations = old.Operations
		}
		if strings.TrimSpace(v.ConversationSessionID) == "" {
			v.ConversationSessionID = old.ConversationSessionID
		}
		if strings.TrimSpace(v.SOPMode) == "" {
			v.SOPMode = old.SOPMode
		}
		if !old.Clarification.StartedAt.IsZero() && (v.Clarification.StartedAt.IsZero() || v.Clarification.StartedAt.After(old.Clarification.StartedAt)) {
			v.Clarification.StartedAt = old.Clarification.StartedAt
		}
		if strings.TrimSpace(v.DeliveryState) == "" {
			v.DeliveryState = old.DeliveryState
		}
	} else {
		v.CreatedAt = now
		if v.Number == 0 {
			v.Number = 1
			var maxNumber sql.NullInt64
			if err := r.DB.Raw(`SELECT MAX(number) FROM issues WHERE project_id=?`, v.ProjectID).Scan(&maxNumber).Error; err != nil {
				return v, err
			}
			if maxNumber.Valid {
				v.Number = int(maxNumber.Int64) + 1
			}
		}
	}
	if !preserveStatus {
		// An import carries the status the tracker layer already merged, so
		// the repo layer must not re-force "closed" on a requirement the
		// user moved to another status.
		if mode != issueWriteImported {
			if v.TrackerClosed() && v.Status != "closed" {
				v.Status = "closed"
				v.CloseSource = "external"
			}
			if v.Status == "closed" {
				if v.CloseSource == "" {
					if ok && old.Status == "closed" {
						v.CloseSource = old.CloseSource
						if v.CloseSource == "" {
							v.CloseSource = "manual" // pre-CloseSource manual close
						}
					} else if v.TrackerClosed() {
						v.CloseSource = "external"
					} else {
						v.CloseSource = "manual"
					}
				}
			} else {
				v.CloseSource = ""
			}
		}
		if v.Status == "done" || v.Status == "closed" || v.Status == "cancelled" {
			v.AutomationState = "ignored"
		}
	}
	if v.AssigneeUserID == 0 {
		if project, found := r.projects.Project(v.ProjectID); found {
			v.AssigneeUserID = project.OwnerID
		}
	}
	// Completion is a lifecycle event, not an editable client timestamp. Keep
	// legacy unknown times unknown on ordinary edits, and clear on reopening.
	v.CompletedAt = time.Time{}
	if ok && v.Completed() {
		if old.Completed() {
			v.CompletedAt = old.CompletedAt
		} else {
			v.CompletedAt = now
		}
	}
	if v.WithoutSOP() {
		v.SOPMode = domain.IssueSOPNone
		v.WorkflowID, v.PlaybookID, v.SOPBindings = "", "", nil
		v.AutomationState = "ignored"
	} else {
		v.SOPMode = ""
	}
	v.UpdatedAt = now
	return v, nil
}

func (r *IssuesRepo) writeIssue(v domain.Issue) error {
	return upsert(r.DB, "issues", issueValues(v))
}

func issueValues(v domain.Issue) map[string]any {
	ext := ""
	if v.ExternalRef != nil {
		ext = v.ExternalRef.ExternalID
	}
	var assignee any
	if v.AssigneeUserID > 0 {
		assignee = v.AssigneeUserID
	}
	return map[string]any{
		"id": v.ID, "project_id": v.ProjectID, "number": v.Number, "title": v.Title,
		"status": v.Status, "kind": v.Kind, "source": v.Source, "workflow_id": v.WorkflowID,
		"assignee_user_id": assignee, "external_id": ext, "payload": mustJSON(v),
		"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	}
}

// ResponsibleUser resolves the explicit issue responsibility first and falls
// back to the project's separately configured owner.
func (r *IssuesRepo) ResponsibleUser(issue domain.Issue) (domain.User, error) {
	userID := issue.AssigneeUserID
	if userID == 0 {
		project, ok := r.projects.Project(issue.ProjectID)
		if !ok || project.OwnerID == 0 {
			return domain.User{}, errors.New("项目未配置 Owner，需求也未配置责任人")
		}
		userID = project.OwnerID
	}
	user, ok := r.users.User(userID)
	if !ok {
		return domain.User{}, errors.New("需求责任人或项目 Owner 未关联有效用户")
	}
	return user, nil
}

// ---------- Issue comments ----------

func (r *IssuesRepo) Comments(issueID string) []domain.IssueComment {
	out := payloadList[domain.IssueComment](r.DB, "issue_comments", "issue_id = ?", issueID)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (r *IssuesRepo) PutComment(v domain.IssueComment) error {
	v.CreatedAt = time.Now().UTC()
	return upsert(r.DB, "issue_comments", map[string]any{
		"id": v.ID, "issue_id": v.IssueID, "payload": mustJSON(v), "created_at": timePtr(v.CreatedAt),
	})
}
