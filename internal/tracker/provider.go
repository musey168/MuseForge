package tracker

import (
	"context"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// ProviderIssue is one requirement as an external tracker reports it,
// normalised so the import path is the same for every provider.
type ProviderIssue struct {
	// Number is the provider's own issue number.
	Number int
	// Title is the issue title.
	Title string
	// Body is the issue body; it is the description an import delivers.
	Body string
	// State is the provider's own state, e.g. "open" or "closed".
	State string
	// Kind is the provider's item kind, e.g. "issue" or "bug".
	Kind string
	// URL links back to the issue on the provider.
	URL string
	// ExternalID is the provider's own identifier for the issue.
	ExternalID string
	// CreatedAt is the provider's creation timestamp.
	CreatedAt time.Time
}

// IssueProvider adapts one external tracker to the shared import path.
// GitHub and GitLab implement it; a new tracker reuses the same interface
// instead of adding a second copy of the import.
type IssueProvider interface {
	// Name is the provider key: "github" or "gitlab".
	Name() string
	// Label is the provider's name for the user: "GitHub", "GitLab".
	Label() string
	// IDPrefix is the prefix of the local requirement ids this provider
	// owns, e.g. "gh" for GitHub and "gl" for GitLab.
	IDPrefix() string
	// CredentialHost is the git host whose stored credential authenticates
	// the provider's API calls ("github.com", "gitlab.com").
	CredentialHost() string
	// ListIssues lists the provider's requirements of one repository. Items
	// the provider does not consider a requirement (a GitHub pull request,
	// for instance) are skipped here, so the shared path never needs a
	// provider-specific branch.
	ListIssues(ctx context.Context, repo domain.Repository, token string) ([]ProviderIssue, error)
	// LocalStatus maps a provider state onto the local status vocabulary and
	// the close source that goes with it.
	LocalStatus(state string) (status string, closeSource string)
}

// trackerKind maps a provider's item kind onto the local requirement kind.
// One table, so every provider lands on the same vocabulary.
func trackerKind(providerKind string) string {
	switch strings.ToLower(strings.TrimSpace(providerKind)) {
	case "bug":
		return "bug"
	case "task":
		return "task"
	case "issue", "incident", "test_case":
		return "issue"
	default:
		return "issue"
	}
}
