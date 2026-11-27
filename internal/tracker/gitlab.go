package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// gitlabClient talks to one GitLab project's REST API. GitLab scopes its
// issues under the project, so the project path is part of the client.
type gitlabClient struct {
	base, path, host, token string
	http                    *http.Client
}

// gitlabClient builds the client of one repository's GitLab project. The
// project key is either "gitlab:<host>/<group>/<repo>" or the bare project
// path on gitlab.com.
func (s *Service) gitlabClient(repo domain.Repository) (*gitlabClient, error) {
	host, path, ok := gitlabProjectOf(repo.GongfengProjectID)
	if !ok {
		return nil, fmt.Errorf("invalid GitLab project %q", repo.GongfengProjectID)
	}
	base := s.gitlabAPIBase
	if base == "" {
		// A self-hosted GitLab serves its own API root.
		base = strings.TrimSpace(os.Getenv("AGENTFLOW_GITLAB_API_BASE"))
	}
	if base == "" {
		base = "https://" + host + "/api/v4"
	}
	return &gitlabClient{
		base: strings.TrimRight(base, "/"),
		path: path,
		host: host,
		http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// gitlabProjectOf splits a repository's GitLab project key into the host and
// the project path. A key without the "gitlab:" prefix is a project on
// gitlab.com, so an older bare binding still imports.
func gitlabProjectOf(key string) (host, path string, ok bool) {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, domain.MRProjectGitLab) {
		platform, host, path, valid := domain.ExternalMRProject(key)
		if !valid || platform != domain.GitPlatformGitLab {
			return "", "", false
		}
		return host, path, true
	}
	path = strings.Trim(strings.TrimSpace(key), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", false
		}
	}
	return "gitlab.com", path, true
}

// request calls the project API. GitLab shows a project a credential cannot
// reach as 404 and rejects a bad token with 401; both surface with the status
// so the run names the failure.
func (g *gitlabClient) request(method, suffix string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, g.base+"/projects/"+url.PathEscape(g.path)+suffix, body)
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/json")
	if g.token != "" {
		r.Header.Set("PRIVATE-TOKEN", g.token)
	}
	res, err := g.http.Do(r)
	if err != nil {
		return fmt.Errorf("GitLab request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GitLab returned HTTP %d; check the project owner's GitLab access token and project permissions", res.StatusCode)
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(output)
}

// gitlabProvider adapts GitLab to the shared tracker import path.
type gitlabProvider struct {
	service *Service
}

func (p *gitlabProvider) Name() string { return "gitlab" }

func (p *gitlabProvider) Label() string { return "GitLab" }

func (p *gitlabProvider) IDPrefix() string { return "gl" }

func (p *gitlabProvider) CredentialHost() string { return "gitlab.com" }

// LocalStatus maps GitLab's own state onto the local status vocabulary.
func (p *gitlabProvider) LocalStatus(state string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "opened", "open":
		return "todo", ""
	case "closed":
		return "closed", "external"
	default:
		return "backlog", ""
	}
}

// ListIssues lists one GitLab project's issues. GitLab keeps merge requests
// in their own API, so the listing holds requirements only.
func (p *gitlabProvider) ListIssues(ctx context.Context, repo domain.Repository, token string) ([]ProviderIssue, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("GitLab 凭据缺失：项目 Owner 未在个人设置里配置 GitLab 访问令牌")
	}
	g, err := p.service.gitlabClient(repo)
	if err != nil {
		return nil, err
	}
	g.token = token
	var out []ProviderIssue
	for page := 1; page <= 100; page++ {
		var list []struct {
			ID          int64     `json:"id"`
			IID         int       `json:"iid"`
			Title       string    `json:"title"`
			Description string    `json:"description"`
			State       string    `json:"state"`
			IssueType   string    `json:"issue_type"`
			Type        string    `json:"type"`
			WebURL      string    `json:"web_url"`
			CreatedAt   time.Time `json:"created_at"`
		}
		if err := g.request("GET", "/issues?state=all&per_page=100&page="+strconv.Itoa(page), nil, &list); err != nil {
			return nil, err
		}
		for _, item := range list {
			kind := strings.TrimSpace(item.IssueType)
			if kind == "" {
				kind = strings.TrimSpace(item.Type)
			}
			if kind == "" {
				kind = "issue"
			}
			out = append(out, ProviderIssue{
				Number:     item.IID,
				Title:      item.Title,
				Body:       item.Description,
				State:      item.State,
				Kind:       kind,
				URL:        item.WebURL,
				ExternalID: strconv.FormatInt(item.ID, 10),
				CreatedAt:  item.CreatedAt,
			})
		}
		if len(list) < 100 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("GitLab import exceeded 100 pages; narrow the project scope")
}
