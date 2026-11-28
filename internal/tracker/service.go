package tracker

import (
	"context"
	"maps"

	"agentflow/internal/domain"
)

// Service imports requirements from the external trackers the platform
// supports: GitHub and GitLab.
//
// Every import is authenticated with a stored Git HTTP credential of the user
// who owns the project the repository belongs to.
type Service struct {
	githubAPIBase  string // Tests inject an HTTP server; production uses api.github.com.
	gitlabAPIBase  string // Tests inject an HTTP server; production uses the project's GitLab API root.
	gitHostAPIBase string // Tests inject an HTTP server for every Git HTTP credential host.
	projectAuth    ProjectAuthStore
}

// IssueStoreAdapter abstracts the store operations the import path needs.
type IssueStoreAdapter interface {
	Repositories() []domain.Repository
	Repository(id string) (domain.Repository, bool)
	Issue(id string) (domain.Issue, bool)
	// PutImportedIssue writes a requirement a tracker import merged. Its
	// status is the merged one and must not be re-derived.
	PutImportedIssue(v domain.Issue) error
	// PutTrackerImportRun stores the outcome of one provider's import run.
	PutTrackerImportRun(run domain.TrackerImportRun) error
}

// ProjectAuthStore provides the ownership and credential context needed to
// authenticate a project-scoped import.
type ProjectAuthStore interface {
	Projects() []domain.Project
	Project(id string) (domain.Project, bool)
	Repositories() []domain.Repository
	User(id int64) (domain.User, bool)
}

func NewService() *Service {
	return &Service{}
}

// ConfigureWithStore enables project-aware authentication: an import then
// resolves the owning project and authenticates as its Owner.
func (s *Service) ConfigureWithStore(store ProjectAuthStore) {
	s.projectAuth = store
}

// applyExistingIssue folds the stored requirement into the freshly imported
// one: platform-owned fields survive, tracker-owned fields are refreshed.
func applyExistingIssue(dst *domain.Issue, existing domain.Issue) {
	// Tracker state is metadata after intake. Local status belongs to the
	// platform and must survive imports.
	dst.Status = existing.Status
	dst.CloseSource = existing.CloseSource
	dst.ID = existing.ID
	dst.CreatedAt = existing.CreatedAt
	if existing.Number > 0 {
		dst.Number = existing.Number
	}
	if existing.Kind != "" {
		dst.Kind = existing.Kind
	}
	if len(existing.Tags) > 0 {
		// Labels are local: the tracker has no equivalent, so an import keeps
		// the ones the user set.
		dst.Tags = append([]string(nil), existing.Tags...)
	}
	if existing.ExternalRef != nil && existing.ExternalRef.Imported {
		if dst.ExternalRef == nil {
			ref := *existing.ExternalRef
			dst.ExternalRef = &ref
		} else {
			dst.ExternalRef.Imported = true
		}
	}
	if existing.ProjectID != "" {
		dst.ProjectID = existing.ProjectID
	}
	if existing.RepositoryID != "" {
		dst.RepositoryID = existing.RepositoryID
	}
	if len(existing.RepositoryIDs) > 0 {
		// Remote trackers are single-repository; keep the local multi-repo
		// binding intact.
		dst.RepositoryIDs = append([]string(nil), existing.RepositoryIDs...)
		if dst.RepositoryID == "" {
			dst.RepositoryID = existing.RepositoryIDs[0]
		}
	}
	if existing.BaseBranch != "" {
		dst.BaseBranch = existing.BaseBranch
	}
	dst.RepositoryBranches = maps.Clone(existing.RepositoryBranches)
}

// githubTokenForUser is the stored GitHub credential of one user.
func (s *Service) githubTokenForUser(userID int64) string {
	if credentials, ok := s.projectAuth.(GitHTTPCredentialStore); ok {
		if credential, ok := credentials.GitHTTPCredentialForHost(context.Background(), userID, "github.com"); ok {
			return credential.Token
		}
	}
	return ""
}
