package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/domain"
)

type githubClient struct {
	base, path, token string
	http              *http.Client
}

func (s *Service) githubClient(project string) (*githubClient, error) {
	path := strings.TrimPrefix(project, "github:")
	info, ok := domain.ParseRepositoryURL("https://github.com/" + path)
	if !ok || info.Project != project {
		return nil, fmt.Errorf("invalid GitHub repository")
	}
	var token string
	if s.projectAuth != nil {
		for _, repo := range s.projectAuth.Repositories() {
			if repo.Provider != "github" || repo.GongfengProjectID != project {
				continue
			}
			if p, found := s.projectAuth.Project(repo.ProjectID); found {
				token = s.githubTokenForUser(p.OwnerID)
			}
			break
		}
	}
	base := s.githubAPIBase
	if base == "" {
		// A GitHub Enterprise install serves its own API root.
		base = strings.TrimSpace(os.Getenv("AGENTFLOW_GITHUB_API_BASE"))
	}
	if base == "" {
		base = "https://api.github.com"
	}
	return &githubClient{base: base, path: path, token: token, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// githubClientWithToken builds the client of one repository with an explicit
// credential. The import path resolves the credential itself, so the token
// the client would pick on its own is replaced, not merged.
func (s *Service) githubClientWithToken(project, token string) (*githubClient, error) {
	g, err := s.githubClient(project)
	if err != nil {
		return nil, err
	}
	g.token = strings.TrimSpace(token)
	return g, nil
}

func (g *githubClient) request(method, suffix string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, g.base+"/repos/"+g.path+suffix, body)
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	r.Header.Set("Content-Type", "application/json")
	if g.token != "" {
		r.Header.Set("Authorization", "Bearer "+g.token)
	}
	res, err := g.http.Do(r)
	if err != nil {
		return fmt.Errorf("GitHub request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GitHub returned HTTP %d; check the project owner's Git token and repository permissions", res.StatusCode)
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(output)
}

// githubProvider adapts GitHub to the shared tracker import path.
type githubProvider struct {
	service *Service
}

func (p *githubProvider) Name() string { return "github" }

func (p *githubProvider) Label() string { return "GitHub" }

func (p *githubProvider) IDPrefix() string { return "gh" }

func (p *githubProvider) CredentialHost() string { return "github.com" }

// LocalStatus maps GitHub's own state onto the local status vocabulary.
func (p *githubProvider) LocalStatus(state string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		return "todo", ""
	case "closed":
		return "closed", "external"
	default:
		return "backlog", ""
	}
}

// ListIssues lists the GitHub issues of one repository. GitHub returns pull
// requests on the issues listing; they are not requirements, so they are
// skipped here and the shared path never learns about them.
func (p *githubProvider) ListIssues(ctx context.Context, repo domain.Repository, token string) ([]ProviderIssue, error) {
	g, err := p.service.githubClientWithToken(repo.GongfengProjectID, token)
	if err != nil {
		return nil, err
	}
	var out []ProviderIssue
	for page := 1; page <= 100; page++ {
		var list []struct {
			ID          int64           `json:"id"`
			Number      int             `json:"number"`
			Title       string          `json:"title"`
			Body        string          `json:"body"`
			State       string          `json:"state"`
			URL         string          `json:"html_url"`
			CreatedAt   time.Time       `json:"created_at"`
			PullRequest json.RawMessage `json:"pull_request"`
		}
		if err := g.request("GET", "/issues?state=all&per_page=100&page="+strconv.Itoa(page), nil, &list); err != nil {
			return nil, err
		}
		for _, item := range list {
			if len(item.PullRequest) > 0 {
				continue
			}
			out = append(out, ProviderIssue{
				Number:     item.Number,
				Title:      item.Title,
				Body:       item.Body,
				State:      item.State,
				Kind:       "issue",
				URL:        item.URL,
				ExternalID: strconv.FormatInt(item.ID, 10),
				CreatedAt:  item.CreatedAt,
			})
		}
		if len(list) < 100 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("GitHub import exceeded 100 pages; narrow the repository scope")
}
