package tracker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// ImportTracker imports the requirements of one tracker provider's
// repositories and records the run. The record is persisted, so the outcome
// of the last run survives a restart.
//
// projectID scopes the run to one project; an empty project imports every
// repository the provider is bound to.
func (s *Service) ImportTracker(store IssueStoreAdapter, providerName, projectID string) domain.TrackerImportRun {
	provider, ok := s.trackerProvider(providerName)
	if !ok {
		return domain.TrackerImportRun{
			ID:        strings.ToLower(strings.TrimSpace(providerName)),
			Provider:  strings.ToLower(strings.TrimSpace(providerName)),
			ProjectID: strings.TrimSpace(projectID),
			Status:    domain.TrackerImportFailed,
			Reason:    fmt.Sprintf("unknown tracker provider %q", providerName),
			StartedAt: time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
		}
	}
	return s.importProviderIssues(store, provider, projectID)
}

// TrackerProviders lists the tracker providers the platform can import
// requirements from.
func TrackerProviders() []string {
	return []string{"github"}
}

// trackerProvider returns the adapter of a provider name. GitLab adds its own
// case here and reuses the shared import path.
func (s *Service) trackerProvider(name string) (IssueProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "github":
		return &githubProvider{service: s}, true
	default:
		return nil, false
	}
}

// importProviderIssues is the shared import path: one credential lookup, one
// listing path and one local write path, parameterised by the provider.
//
// One broken repository must not abort the run. Its outcome is recorded with
// the reason and the other repositories still import.
func (s *Service) importProviderIssues(store IssueStoreAdapter, provider IssueProvider, projectID string) domain.TrackerImportRun {
	started := time.Now().UTC()
	run := domain.TrackerImportRun{
		ID:        provider.Name(),
		Provider:  provider.Name(),
		ProjectID: strings.TrimSpace(projectID),
		Status:    domain.TrackerImportSuccess,
		StartedAt: started,
	}
	var reasons []string
	matched := 0
	for _, repo := range store.Repositories() {
		if repo.Provider != provider.Name() {
			continue
		}
		if run.ProjectID != "" && repo.ProjectID != run.ProjectID {
			continue
		}
		matched++
		outcome := domain.TrackerImportRepository{RepositoryID: repo.ID, ProjectID: repo.ProjectID, Name: repo.Name, Status: domain.TrackerImportSuccess}
		imported, err := s.importProviderRepository(store, provider, repo)
		outcome.Imported = imported
		if err != nil {
			outcome.Status = domain.TrackerImportFailed
			outcome.Reason = err.Error()
			run.Failed++
			reasons = append(reasons, fmt.Sprintf("%s：%s", repo.Name, err.Error()))
			run.Repositories = append(run.Repositories, outcome)
			continue
		}
		run.Imported += imported
		run.Repositories = append(run.Repositories, outcome)
	}
	if matched == 0 {
		run.Status = domain.TrackerImportFailed
		run.Reason = fmt.Sprintf("没有绑定 %s 仓库的项目", provider.Name())
	} else if run.Failed > 0 {
		run.Reason = strings.Join(reasons, "；")
		if run.Failed == matched {
			run.Status = domain.TrackerImportFailed
		} else {
			run.Status = domain.TrackerImportPartial
		}
	}
	run.FinishedAt = time.Now().UTC()
	_ = store.PutTrackerImportRun(run)
	return run
}

// importProviderRepository imports one repository. The credential is the one
// of the project that owns the repository.
func (s *Service) importProviderRepository(store IssueStoreAdapter, provider IssueProvider, repo domain.Repository) (int, error) {
	token := s.providerToken(provider, repo)
	items, err := provider.ListIssues(context.Background(), repo, token)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range items {
		if err := s.writeTrackerIssue(store, provider, repo, item); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// providerToken resolves the credential the import authenticates with: the
// stored git credential of the owner of the project that owns the repository,
// when that project binds the repository; otherwise the owner of the first
// project that bound it.
//
// Borrowing the token of whichever project bound the repository first would
// import another project's private issues, so the importing project's owner
// wins whenever that project binds the repository itself.
func (s *Service) providerToken(provider IssueProvider, repo domain.Repository) string {
	if s.projectAuth == nil {
		return ""
	}
	for _, binding := range s.projectAuth.Repositories() {
		if binding.Provider != provider.Name() || binding.GongfengProjectID != repo.GongfengProjectID {
			continue
		}
		if binding.ProjectID == repo.ProjectID {
			return s.credentialToken(provider.CredentialHost(), s.projectOwnerID(binding.ProjectID))
		}
	}
	for _, binding := range s.projectAuth.Repositories() {
		if binding.Provider != provider.Name() || binding.GongfengProjectID != repo.GongfengProjectID {
			continue
		}
		return s.credentialToken(provider.CredentialHost(), s.projectOwnerID(binding.ProjectID))
	}
	return ""
}

// projectOwnerID is the owner of a project, or 0 when it is unknown.
func (s *Service) projectOwnerID(projectID string) int64 {
	if s.projectAuth == nil {
		return 0
	}
	project, found := s.projectAuth.Project(projectID)
	if !found {
		return 0
	}
	return project.OwnerID
}

// credentialToken is the stored git credential of a user for one host.
func (s *Service) credentialToken(host string, userID int64) string {
	if s.projectAuth == nil || userID <= 0 {
		return ""
	}
	credentials, ok := s.projectAuth.(GitHTTPCredentialStore)
	if !ok {
		return ""
	}
	credential, ok := credentials.GitHTTPCredentialForHost(context.Background(), userID, host)
	if !ok {
		return ""
	}
	return strings.TrimSpace(credential.Token)
}

// writeTrackerIssue merges one tracker item into the requirement list and
// writes it. It is the one local write path every provider goes through.
func (s *Service) writeTrackerIssue(store IssueStoreAdapter, provider IssueProvider, repo domain.Repository, item ProviderIssue) error {
	status, closeSource := provider.LocalStatus(item.State)
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%s-%d", provider.IDPrefix(), repo.ID, item.Number)
	issue := domain.Issue{
		ID:           id,
		Number:       item.Number,
		ProjectID:    repo.ProjectID,
		RepositoryID: repo.ID,
		BaseBranch:   repo.DefaultBranch,
		Kind:         trackerKind(item.Kind),
		Title:        item.Title,
		Description:  item.Body,
		Priority:     "medium",
		Source:       provider.Name(),
		Status:       status,
		CloseSource:  closeSource,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    now,
		ExternalRef: &domain.ExternalRef{
			Source:            provider.Name(),
			ExternalID:        item.ExternalID,
			ExternalIID:       item.Number,
			ExternalURL:       item.URL,
			ExternalState:     item.State,
			ExternalType:      trackerKind(item.Kind),
			SyncedAt:          now,
			SourceContentHash: domain.SourceContentHash(item.Body),
		},
	}
	if old, ok := store.Issue(id); ok {
		mergeTrackerIssue(provider, &issue, old)
	}
	return store.PutImportedIssue(issue)
}

// mergeTrackerIssue merges the tracker-built requirement with the stored one.
//
// Issue-backed fields — title, body, external reference — are refreshed on
// every import. Locally owned fields are never overwritten: a description the
// user edited stays, and a status the user moved stays.
//
// The status rule, in one place: the tracker owns the status only while the
// local status is still the one the last import wrote. The last import's
// provider state lives in ExternalRef.ExternalState, so the status the
// tracker wrote is provider.LocalStatus(that state). As soon as the local
// status differs from it, the user moved the requirement and the local
// status wins — a requirement the user set to "done" is never reopened or
// closed by a later import, while a requirement nobody touched follows the
// tracker.
func mergeTrackerIssue(provider IssueProvider, dst *domain.Issue, existing domain.Issue) {
	applyExistingIssue(dst, existing)
	// A description the user edited survives, and so does one a running
	// clarification is about to rewrite.
	if existing.Clarification.Status != "" || existing.DescriptionEdited() {
		dst.Description = existing.Description
	}
	if !trackerOwnsStatus(provider, existing) {
		dst.Status = existing.Status
		dst.CloseSource = existing.CloseSource
		return
	}
	status, closeSource := provider.LocalStatus(dst.ExternalRef.ExternalState)
	dst.Status = status
	dst.CloseSource = closeSource
}

// trackerOwnsStatus reports whether the tracker still owns the requirement's
// status, i.e. the local status is still the one the last import wrote.
func trackerOwnsStatus(provider IssueProvider, existing domain.Issue) bool {
	if existing.ExternalRef == nil || strings.TrimSpace(existing.ExternalRef.ExternalState) == "" {
		// A requirement the tracker never imported is the user's own.
		return false
	}
	if strings.TrimSpace(existing.Status) == "" {
		return true
	}
	owned, _ := provider.LocalStatus(existing.ExternalRef.ExternalState)
	return existing.Status == owned
}