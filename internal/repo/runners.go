package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// CLI Runner connection store (cli-connect.md). Task state is always
// durable: a runner disconnect only affects delivery, never the task itself.

var (
	ErrRunnerNotFound = errors.New("runner not found")
)

// ---------- Runner registry ----------

// PutRunner upserts the row as given. It does not move a machine to another
// owner. Registration goes through RegisterRunner, which only renews a row
// within its current owner scope.
func (r *RunnersRepo) PutRunner(v domain.Runner) error {
	values, err := runnerWriteValues(v)
	if err != nil {
		return err
	}
	updateCols := slices.Sorted(maps.Keys(values))
	return upsertColumns(r.DB, "runners", values, updateCols)
}

// RegisterRunner creates a new machine or renews a session in its existing
// owner scope. A renewal cannot replace a row that changed after the lookup.
func (r *RunnersRepo) RegisterRunner(v domain.Runner, renew bool) error {
	values, err := runnerWriteValues(v)
	if err != nil {
		return err
	}
	if !renew {
		return r.DB.Table("runners").Create(values).Error
	}
	q := r.DB.Model(&runnerRow{}).Where("id = ?", v.ID)
	if v.OwnerUserID > 0 {
		q = q.Where("owner_user_id = ?", v.OwnerUserID)
	} else {
		q = q.Where("(owner_user_id IS NULL OR owner_user_id = 0)")
	}
	delete(values, "id")
	delete(values, "owner_user_id")
	delete(values, "created_at")
	result := q.Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRunnerNotFound
	}
	return nil
}

func runnerWriteValues(v domain.Runner) (map[string]any, error) {
	if strings.TrimSpace(v.ID) == "" {
		return nil, fmt.Errorf("runner id is required")
	}
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = domain.RunnerStatusOffline
	}
	return map[string]any{
		"id": v.ID, "name": v.Name, "status": v.Status, "os": v.OS, "arch": v.Arch,
		"runner_version": v.RunnerVersion, "labels": jsonBytes(v.Labels),
		"owner_user_id": nullableInt64(v.OwnerUserID), "session_token_hash": v.SessionTokenHash,
		"session_expires_at": v.SessionExpiresAt, "runtimes": jsonBytes(v.Runtimes),
		"last_seen_at": v.LastSeenAt, "created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	}, nil
}

func (r *RunnersRepo) Runner(id string) (domain.Runner, bool) {
	return r.runnerRow(id)
}

func (r *RunnersRepo) runnerRow(id string) (domain.Runner, bool) {
	var row runnerRow
	if err := r.DB.Where("id = ?", id).Take(&row).Error; err != nil {
		return domain.Runner{}, false
	}
	return runnerFromRow(row), true
}

func runnerFromRow(row runnerRow) domain.Runner {
	v := domain.Runner{
		ID: row.ID, Name: row.Name, Status: row.Status, OS: row.OS, Arch: row.Arch,
		RunnerVersion:    row.RunnerVersion,
		SessionTokenHash: row.SessionTokenHash,
	}
	if row.OwnerUserID != nil {
		v.OwnerUserID = *row.OwnerUserID
	}
	if len(row.LoginModels) > 0 {
		_ = json.Unmarshal(row.LoginModels, &v.LoginModels)
	}
	if len(row.Labels) > 0 {
		_ = json.Unmarshal(row.Labels, &v.Labels)
	}
	if len(row.Runtimes) > 0 {
		_ = json.Unmarshal(row.Runtimes, &v.Runtimes)
	}
	if row.SessionExpiresAt != nil {
		t := *row.SessionExpiresAt
		v.SessionExpiresAt = &t
	}
	if row.LastSeenAt != nil {
		t := *row.LastSeenAt
		v.LastSeenAt = &t
	}
	if row.Created != nil {
		v.CreatedAt = *row.Created
	}
	if row.Updated != nil {
		v.UpdatedAt = *row.Updated
	}
	return v
}

func (r *RunnersRepo) Runners() []domain.Runner {
	var rows []runnerRow
	if err := r.DB.Order("created_at").Find(&rows).Error; err != nil {
		return []domain.Runner{}
	}
	out := make([]domain.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, runnerFromRow(row))
	}
	return out
}

// RunnerSessionByHash resolves a session token hash to its runner. Expired
// sessions are rejected so a leaked old session token stops working.
func (r *RunnersRepo) RunnerSessionByHash(hash string) (domain.Runner, bool) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return domain.Runner{}, false
	}
	var row runnerRow
	if err := r.DB.Where("session_token_hash = ?", hash).Take(&row).Error; err != nil {
		return domain.Runner{}, false
	}
	if row.SessionExpiresAt != nil && !row.SessionExpiresAt.After(time.Now().UTC()) {
		return domain.Runner{}, false
	}
	return runnerFromRow(row), true
}

// RunnerHeartbeat records liveness and the latest runtime inventory. The
// server-side disabled flag survives: a CLI disabled from the 执行器 page
// stays disabled even though the daemon keeps reporting it online.
func (r *RunnersRepo) RunnerHeartbeat(id string, runtimes []domain.RunnerRuntime, version string, now, sessionExpiresAt time.Time) error {
	current, ok := r.runnerRow(id)
	disabled := map[string]bool{}
	authModes := map[string]string{}
	if ok {
		for _, rt := range current.Runtimes {
			if rt.Status == domain.RuntimeStatusDisabled {
				disabled[rt.Provider] = true
			}
			if rt.AuthMode == domain.RuntimeAuthLocal || rt.AuthMode == domain.RuntimeAuthPersonal {
				authModes[rt.Provider] = rt.AuthMode
			}
		}
	}
	for i := range runtimes {
		if disabled[runtimes[i].Provider] {
			runtimes[i].Status = domain.RuntimeStatusDisabled
		}
		if domain.IsAgentCLIKind(runtimes[i].Provider) && !domain.SupportsRunnerLogin(runtimes[i].Provider) {
			runtimes[i].AuthMode = domain.RuntimeAuthPersonal
		} else if mode := authModes[runtimes[i].Provider]; mode != "" {
			runtimes[i].AuthMode = mode
		} else if strings.EqualFold(runtimes[i].Provider, "codebuddy") {
			runtimes[i].AuthMode = domain.RuntimeAuthPersonal
		} else {
			runtimes[i].AuthMode = domain.RuntimeAuthLocal
		}
	}
	raw, err := json.Marshal(runtimes)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status": domain.RunnerStatusOnline, "last_seen_at": now, "runtimes": string(raw),
		"session_expires_at": sessionExpiresAt, "updated_at": now,
	}
	if version = strings.TrimSpace(version); version != "" {
		updates["runner_version"] = version
	}
	res := r.DB.Model(&runnerRow{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRunnerNotFound
	}
	return nil
}

// SetRunnerRuntimeAuthMode selects local CLI login or the responsible user's
// saved provider credential for one runtime. The daemon inventory heartbeat
// cannot overwrite this server-managed preference.
func (r *RunnersRepo) SetRunnerRuntimeAuthMode(runnerID, provider, authMode string, now time.Time) error {
	if authMode != domain.RuntimeAuthLocal && authMode != domain.RuntimeAuthPersonal {
		return fmt.Errorf("invalid runtime auth mode %q", authMode)
	}
	if strings.EqualFold(provider, "codebuddy") && authMode != domain.RuntimeAuthPersonal {
		return fmt.Errorf("CodeBuddy 仅支持个人凭证")
	}
	if domain.IsAgentCLIKind(provider) && !domain.SupportsRunnerLogin(provider) && authMode != domain.RuntimeAuthPersonal {
		return fmt.Errorf("%s 仅支持个人凭证", provider)
	}
	return r.updateRunnerRuntimes(runnerID, func(runtimes []domain.RunnerRuntime) (bool, error) {
		found := false
		for i := range runtimes {
			if strings.EqualFold(runtimes[i].Provider, provider) {
				runtimes[i].AuthMode = authMode
				found = true
			}
		}
		if !found {
			return false, fmt.Errorf("runtime %q not found on runner %s", provider, runnerID)
		}
		return true, nil
	}, now)
}

// SetRunnerRuntimeStatus enables or disables one discovered CLI on a runner.
// Disabled CLIs stop receiving tasks of that provider.
func (r *RunnersRepo) SetRunnerRuntimeStatus(runnerID, provider, status string, now time.Time) error {
	if status != domain.RuntimeStatusOnline && status != domain.RuntimeStatusDisabled {
		return fmt.Errorf("invalid runtime status %q", status)
	}
	return r.updateRunnerRuntimes(runnerID, func(runtimes []domain.RunnerRuntime) (bool, error) {
		found := false
		for i := range runtimes {
			if strings.EqualFold(runtimes[i].Provider, provider) {
				runtimes[i].Status = status
				found = true
			}
		}
		if !found {
			return false, fmt.Errorf("runtime %q not found on runner %s", provider, runnerID)
		}
		return true, nil
	}, now)
}

func (r *RunnersRepo) updateRunnerRuntimes(runnerID string, mutate func([]domain.RunnerRuntime) (bool, error), now time.Time) error {
	current, ok := r.runnerRow(runnerID)
	if !ok {
		return ErrRunnerNotFound
	}
	changed, err := mutate(current.Runtimes)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	raw, err := json.Marshal(current.Runtimes)
	if err != nil {
		return err
	}
	return r.DB.Model(&runnerRow{}).Where("id = ?", runnerID).Updates(map[string]any{
		"runtimes": string(raw), "updated_at": now,
	}).Error
}

// MarkOfflineRunners flags runners whose heartbeat is older than threshold.
// Callers decide their own thresholds; the store only persists the state.
func (r *RunnersRepo) MarkOfflineRunners(threshold time.Duration, now time.Time) {
	if r.DB == nil {
		return
	}
	cutoff := now.Add(-threshold)
	_ = r.DB.Model(&runnerRow{}).
		Where("status <> ? AND last_seen_at IS NOT NULL AND last_seen_at < ?", domain.RunnerStatusOffline, cutoff).
		Updates(map[string]any{"status": domain.RunnerStatusOffline, "updated_at": now})
}

// MarkRunnerOffline records an intentional daemon disconnect while retaining
// the runner identity, session, and user preference for the next restart.
func (r *RunnersRepo) MarkRunnerOffline(id string, now time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrRunnerNotFound
	}
	res := r.DB.Model(&runnerRow{}).Where("id = ?", id).Updates(map[string]any{
		"status": domain.RunnerStatusOffline, "updated_at": now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRunnerNotFound
	}
	return nil
}

// DeregisterRunner removes a runner and everything that points at it: the
// machine record and the default-execution-machine selections that target
// it. Called when the daemon uninstalls or stops gracefully.
func (r *RunnersRepo) DeregisterRunner(id string, now time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("runner id is required")
	}
	res := r.DB.Where("id = ?", id).Delete(&runnerRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRunnerNotFound
	}
	return nil
}

// ---------- helpers ----------

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
