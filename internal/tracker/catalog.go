package tracker

import (
	"fmt"
	"maps"
	"strings"

	"agentflow/internal/domain"
)

// The personal build keeps no MCP catalogue: the capability assets (MCP
// servers) are gone. A tracker client is therefore bound from the sync
// configuration's own URL and the deployment's MCP token.

type mcpBinding struct {
	URL     string
	Headers map[string]string
	Source  string
}

// bindTrackerMCP resolves the client of one tracker kind: the URL from the
// sync configuration, the Authorization header from the deployment's MCP
// token.
func bindTrackerMCP(kind string, cfg domain.SyncConfig, envToken string) mcpBinding {
	kind = strings.ToLower(strings.TrimSpace(kind))
	cfgURL := strings.TrimSpace(cfg.GongfengMCPURL)
	if kind == "tapd" {
		cfgURL = strings.TrimSpace(cfg.TAPDMCPURL)
	}

	out := mcpBinding{URL: cfgURL, Headers: map[string]string{}}
	if out.URL == "" {
		return out
	}
	if !headerHas(out.Headers, "Authorization") {
		token := strings.TrimSpace(envToken)
		if token != "" {
			out.Headers["Authorization"] = bearerHeader(token)
			out.Source = "env:AGENTFLOW_MCP_TOKEN"
		} else {
			out.Source = "none"
		}
	}
	return out
}

// bindProjectOwnerTrackerMCP resolves the client of one tracker kind and
// calls it as that project's owner.
func bindProjectOwnerTrackerMCP(kind string, cfg domain.SyncConfig, projectID string, owner domain.User) (mcpBinding, error) {
	binding, err := bindProjectUserTrackerMCP(kind, cfg, projectID, owner, "Owner")
	if err != nil {
		return binding, err
	}
	binding.Source = "project-owner:" + strings.TrimSpace(owner.LoginName)
	return binding, nil
}

func bindProjectUserTrackerMCP(kind string, cfg domain.SyncConfig, projectID string, user domain.User, role string) (mcpBinding, error) {
	projectID = strings.TrimSpace(projectID)
	role = strings.TrimSpace(role)
	if role == "" {
		role = "用户"
	}
	binding := bindTrackerMCP(kind, cfg, "")
	if strings.TrimSpace(binding.URL) == "" {
		return binding, fmt.Errorf("项目 %s 未配置 %s MCP 地址", projectID, trackerDisplayName(kind))
	}
	if !headerHas(binding.Headers, "Authorization") {
		return binding, fmt.Errorf("项目 %s 的 %s MCP 未配置认证：请在 MCP Headers 中提供 Authorization（%s %s）", projectID, trackerDisplayName(kind), role, strings.TrimSpace(user.LoginName))
	}
	binding.Headers = maps.Clone(binding.Headers)
	binding.Source = "project-user:" + strings.TrimSpace(user.LoginName)
	return binding, nil
}

func trackerDisplayName(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "tapd") {
		return "TAPD"
	}
	return "工蜂"
}

func bearerHeader(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}

// headerHas reports whether headers carries a non-blank value for name,
// matching the key case-insensitively and ignoring surrounding whitespace.
func headerHas(headers map[string]string, name string) bool {
	for k, v := range headers {
		if strings.EqualFold(strings.TrimSpace(k), name) && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
