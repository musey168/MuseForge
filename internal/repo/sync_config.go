package repo

import (
	"time"

	"agentflow/internal/domain"
)

// SyncConfig reads the tracker sync configuration.
func (r *SyncRepo) SyncConfig() domain.SyncConfig {
	cfg, _ := payloadOne[domain.SyncConfig](r.DB, "sync_config", mysqlSyncConfigID)
	return cfg
}

// PutSyncConfig stores the tracker sync configuration.
func (r *SyncRepo) PutSyncConfig(cfg domain.SyncConfig) error {
	current := r.SyncConfig()
	cfg.PlaybookSeedRev = current.PlaybookSeedRev
	return upsert(r.DB, "sync_config", map[string]any{
		"id": mysqlSyncConfigID, "payload": mustJSON(cfg), "updated_at": timePtr(time.Now().UTC()),
	})
}
