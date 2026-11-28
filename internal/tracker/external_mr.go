package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"agentflow/internal/domain"
	"agentflow/internal/gitegress"
)

// GitHTTPCredentialStore is implemented by a ProjectAuthStore that keeps
// users' per-host Git HTTP credentials.
type GitHTTPCredentialStore interface {
	GitHTTPCredentialForHost(ctx context.Context, userID int64, host string) (domain.GitHTTPCredentialSecret, bool)
}

// gitHostGet calls one Git host's REST API with a stored credential.
func (s *Service) gitHostGet(ctx context.Context, lookup gitegress.HTTPSHostLookup, cred domain.GitHTTPCredentialSecret, path string, out any) error {
	base, client := cred.APIBase(), gitegress.Client(lookup)
	if cred.Host == "github.com" && s.githubAPIBase != "" {
		base, client = s.githubAPIBase, http.DefaultClient
	}
	if s.gitHostAPIBase != "" {
		base, client = s.gitHostAPIBase, http.DefaultClient
	}
	if base == "" {
		return fmt.Errorf("%s has no API the platform can call", cred.Host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set(cred.APIAuthHeader())
	if cred.Platform == domain.GitPlatformGitHub {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", cred.Host, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s returned HTTP %d; check the registering user's Git access token for this host", cred.Host, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}
