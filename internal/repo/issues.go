package repo

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// ---------- Requirements ----------

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
		issue.Tags = issueTagsFromColumn(row.Tags, issue.Tags)
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
	issue.Tags = issueTagsFromColumn(row.Tags, issue.Tags)
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
// same lock as PutIssue so a concurrent save cannot clobber the edit.
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
		}
		v.CreatedAt = old.CreatedAt
		if v.Number == 0 {
			v.Number = old.Number
		}
		if len(v.Operations) == 0 && len(old.Operations) > 0 {
			v.Operations = old.Operations
		}
		if v.Tags == nil {
			// The writer left the labels out: the stored ones stay. An
			// explicit empty list clears them.
			v.Tags = old.Tags
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
	v.Tags = domain.NormalizeTags(v.Tags)
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
	return map[string]any{
		"id": v.ID, "project_id": v.ProjectID, "number": v.Number, "title": v.Title,
		"status": v.Status, "kind": v.Kind, "source": v.Source,
		"tags":        issueTagsJSON(v.Tags),
		"external_id": ext, "payload": mustJSON(v),
		"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	}
}

// issueTagsJSON stores the label list as the JSON array the API returns.
func issueTagsJSON(tags []string) any {
	if len(tags) == 0 {
		return nil
	}
	return string(mustJSON(tags))
}

// issueTagsFromColumn reads the label list back from the tags column. The
// column is the queryable copy of the list the payload carries; a row written
// before the column existed falls back to the payload.
func issueTagsFromColumn(raw string, fallback []string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var tags []string
	if jsonUnmarshal([]byte(raw), &tags) != nil {
		return fallback
	}
	return domain.NormalizeTags(tags)
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
