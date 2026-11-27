package domain

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// TrackerImportRun records one requirement import run of an external tracker
// provider. The record is persisted, so the outcome of the last run survives
// a restart of the platform.
type TrackerImportRun struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	ProjectID  string    `json:"projectId,omitempty"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	Imported   int       `json:"imported"`
	Failed     int       `json:"failed"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	// Repositories holds the outcome per repository, so a broken repository
	// stays visible next to the ones that imported fine.
	Repositories []TrackerImportRepository `json:"repositories,omitempty"`
}

// TrackerImportRepository is the outcome of importing one repository.
type TrackerImportRepository struct {
	RepositoryID string `json:"repositoryId"`
	ProjectID    string `json:"projectId,omitempty"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	Imported     int    `json:"imported"`
}

// Import run outcomes.
const (
	// TrackerImportSuccess means every repository of the provider imported.
	TrackerImportSuccess = "success"
	// TrackerImportPartial means some repositories imported and some failed.
	TrackerImportPartial = "partial"
	// TrackerImportFailed means no repository imported.
	TrackerImportFailed = "failed"
)

// SourceContentHash is the hash of the tracker body an import last delivered.
// It lets a later import tell a remote edit from a local one.
func SourceContentHash(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
}

// DescriptionEdited reports whether the requirement's description differs
// from the tracker body the last import delivered, i.e. the user edited it.
// A tracker refresh must leave such a description untouched.
func (i Issue) DescriptionEdited() bool {
	if i.ExternalRef == nil || strings.TrimSpace(i.ExternalRef.SourceContentHash) == "" {
		return false
	}
	return SourceContentHash(i.Description) != i.ExternalRef.SourceContentHash
}
