package repo

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"agentflow/internal/domain"

	"gorm.io/gorm/clause"
)

// ErrTrackerImportRunMissing is returned when a run names no provider, so
// there is nothing to store it under.
var ErrTrackerImportRunMissing = errors.New("tracker import run needs a provider")

// TrackerRepo owns the tracker import state: the outcome of one provider's
// last requirement import, including the outcome of every repository that
// took part in it.
//
// The state is persisted, not held in memory, so a restart does not lose the
// last run.
type TrackerRepo struct {
	*deps
}

// trackerImportPayload is the part of a run that is not worth a column of
// its own: the reason and the per-repository outcome.
type trackerImportPayload struct {
	Reason       string                           `json:"reason,omitempty"`
	Repositories []domain.TrackerImportRepository `json:"repositories,omitempty"`
}

// PutTrackerImportRun stores the outcome of one provider's import run. The
// run replaces the provider's previous one, so the table always holds the
// last outcome.
func (r *TrackerRepo) PutTrackerImportRun(run domain.TrackerImportRun) error {
	run.ID = strings.TrimSpace(run.ID)
	run.Provider = strings.TrimSpace(run.Provider)
	if run.ID == "" {
		run.ID = run.Provider
	}
	if run.Provider == "" {
		run.Provider = run.ID
	}
	if run.ID == "" {
		return ErrTrackerImportRunMissing
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if run.FinishedAt.IsZero() {
		run.FinishedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(trackerImportPayload{Reason: run.Reason, Repositories: run.Repositories})
	if err != nil {
		return err
	}
	row := trackerImportRunRow{
		ID:         run.ID,
		Provider:   run.Provider,
		ProjectID:  run.ProjectID,
		Status:     run.Status,
		Imported:   run.Imported,
		Failed:     run.Failed,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		Payload:    payload,
	}
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider", "project_id", "status", "imported", "failed", "started_at", "finished_at", "payload"}),
	}).Create(&row).Error
}

// LatestTrackerImportRun returns the stored outcome of one provider's last
// import run.
func (r *TrackerRepo) LatestTrackerImportRun(provider string) (domain.TrackerImportRun, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return domain.TrackerImportRun{}, false
	}
	var row trackerImportRunRow
	if err := r.DB.Where("id = ?", provider).Take(&row).Error; err != nil {
		return domain.TrackerImportRun{}, false
	}
	run := domain.TrackerImportRun{
		ID:         row.ID,
		Provider:   row.Provider,
		ProjectID:  row.ProjectID,
		Status:     row.Status,
		Imported:   row.Imported,
		Failed:     row.Failed,
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt,
	}
	var payload trackerImportPayload
	if json.Unmarshal(row.Payload, &payload) == nil {
		run.Reason = payload.Reason
		run.Repositories = payload.Repositories
	}
	return run, true
}
