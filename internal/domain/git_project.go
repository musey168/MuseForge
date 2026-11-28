package domain

import (
	"regexp"
	"strings"
)

// MR project keys of pull requests / merge requests on hosts other than
// github.com: "ghe:<host>/<owner>/<repo>" and "gitlab:<host>/<path>".
const (
	MRProjectGHE    = "ghe:"
	MRProjectGitLab = "gitlab:"
)

var (
	externalMRHost   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+(:[1-9][0-9]{0,4})?$`)
	externalPathPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func externalMRHostScope(host string) (string, bool) {
	if strings.HasPrefix(host, "[") {
		normalized, err := HTTPSHostScope("https://" + host)
		return normalized, err == nil
	}
	return host, host != "github.com" && externalMRHost.MatchString(host)
}

// ExternalMRProject splits a ghe: or gitlab: project key into its platform,
// host and repository path.
func ExternalMRProject(project string) (platform, host, path string, ok bool) {
	rest := ""
	switch {
	case strings.HasPrefix(project, MRProjectGHE):
		platform, rest = GitPlatformGitHub, strings.TrimPrefix(project, MRProjectGHE)
	case strings.HasPrefix(project, MRProjectGitLab):
		platform, rest = GitPlatformGitLab, strings.TrimPrefix(project, MRProjectGitLab)
	default:
		return "", "", "", false
	}
	host, path, found := strings.Cut(rest, "/")
	host, validHost := externalMRHostScope(host)
	if !found || !validHost {
		return "", "", "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || (platform == GitPlatformGitHub && len(parts) != 2) {
		return "", "", "", false
	}
	for _, part := range parts {
		if !externalPathPart.MatchString(part) || part == "." || part == ".." {
			return "", "", "", false
		}
	}
	return platform, host, path, true
}
