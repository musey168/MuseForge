package repo

import (
	"errors"

	"agentflow/internal/domain"

	"gorm.io/gorm"
)

var ErrIssueNotFound = errors.New("issue not found")

// IssueDeletionBlock explains active work that prevents deleting a requirement.
// The personal build runs nothing, so no requirement is ever blocked.
type IssueDeletionBlock struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type IssueDeletionState struct {
	Block *IssueDeletionBlock
}

// RequiresForce reports whether the caller has to confirm the deletion.
func (s *IssueDeletionState) RequiresForce() bool { return false }

// IssueDeletionStates reports the deletion state of every requirement.
func (r *IssuesRepo) IssueDeletionStates(issues []domain.Issue) (map[string]*IssueDeletionState, error) {
	states := make(map[string]*IssueDeletionState, len(issues))
	for _, issue := range issues {
		states[issue.ID] = &IssueDeletionState{}
	}
	return states, nil
}

// DeleteIssue removes a requirement together with its comments and images in
// one transaction.
func (r *IssuesRepo) DeleteIssue(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var row issueRow
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIssueNotFound
			}
			return err
		}
		if err := tx.Where("issue_id = ?", id).Delete(&issueCommentRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("issue_id = ?", id).Delete(&issueImageRow{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&issueRow{}).Error
	})
}

// IssueClosureAllowed reports whether a requirement may be closed.
func (r *IssuesRepo) IssueClosureAllowed(issues []domain.Issue) (map[string]bool, error) {
	states, err := r.IssueDeletionStates(issues)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(states))
	for id, state := range states {
		allowed[id] = state.Block == nil
	}
	return allowed, nil
}
