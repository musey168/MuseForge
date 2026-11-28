package application

// Operation is a user-facing MuseForge capability exposed through both HTTP
// and MCP. Path parameters use the {name} form and are supplied separately
// from query parameters and JSON bodies.
type Operation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	ReadOnly    bool   `json:"readOnly"`
}

// PublicOperations is the canonical MCP capability manifest: the routes the
// personal build serves. Internal signals — OTLP ingestion, runner control
// sockets, image bytes — are intentionally excluded.
//
// Every operation here must name a route the platform registers; a test in
// internal/httpapi pins that.
func PublicOperations() []Operation {
	return []Operation{
		{Name: "health_get", Description: "Check the MuseForge service health", Method: "GET", Path: "/api/health", ReadOnly: true},
		{Name: "capabilities_get", Description: "List the public API and MCP operations", Method: "GET", Path: "/api/capabilities", ReadOnly: true},
		{Name: "openapi_get", Description: "Read the MuseForge OpenAPI 3.1 contract", Method: "GET", Path: "/api/openapi.yaml", ReadOnly: true},
		{Name: "bootstrap_get", Description: "Get the console bootstrap: the account, the projects and the repositories", Method: "GET", Path: "/api/bootstrap", ReadOnly: true},
		{Name: "account_save", Description: "Record the platform's single account. The personal build has no sign-in: the console calls this once, and an existing account is returned unchanged.", Method: "POST", Path: "/api/account"},

		{Name: "projects_list", Description: "List the projects with secrets redacted", Method: "GET", Path: "/api/projects", ReadOnly: true},
		{Name: "projects_save", Description: "Create or update a project: its name, identifier, key, colour, owner and default repository.", Method: "PUT", Path: "/api/projects"},
		{Name: "projects_get", Description: "Get one project with secrets redacted", Method: "GET", Path: "/api/projects/{id}", ReadOnly: true},
		{Name: "projects_delete", Description: "Delete a project. It is archived: hidden from lists, its requirements and settings kept, its identifier still taken, restorable with projects_restore.", Method: "DELETE", Path: "/api/projects/{id}"},
		{Name: "projects_restore", Description: "Restore a deleted project", Method: "POST", Path: "/api/projects/{id}/restore"},
		{Name: "project_credentials_list", Description: "List a project's credentials with the secrets redacted", Method: "GET", Path: "/api/projects/{id}/credentials", ReadOnly: true},
		{Name: "project_credentials_save", Description: "Create a project credential. The value is sealed at rest and never returned.", Method: "POST", Path: "/api/projects/{id}/credentials"},

		{Name: "repositories_list", Description: "List the repositories with secrets redacted", Method: "GET", Path: "/api/repositories", ReadOnly: true},
		{Name: "repositories_save", Description: "Create or update a repository. Pass projectId and a GitHub or GitLab web, SSH or clone address; the name and the repository identity are derived from it. A GitLab project key has the form gitlab:<host>/<group>/<repo>.", Method: "PUT", Path: "/api/repositories"},
		{Name: "repositories_delete", Description: "Delete a repository", Method: "DELETE", Path: "/api/repositories"},
		{Name: "repositories_get", Description: "Get one repository with secrets redacted", Method: "GET", Path: "/api/repositories/{id}", ReadOnly: true},

		{Name: "issues_list", Description: "List the requirements. Query: projectId, status, source, tag, q, limit, offset. Omit limit to get Issue[]; with limit returns {items,total,offset,limit,hasMore} (max 200).", Method: "GET", Path: "/api/issues", ReadOnly: true},
		{Name: "issues_save", Description: "Create or update a requirement: its title, description, status, priority, project, tags and the repository and issue it links to.", Method: "PUT", Path: "/api/issues"},
		{Name: "issues_get", Description: "Get one requirement with its comments", Method: "GET", Path: "/api/issues/{id}", ReadOnly: true},
		{Name: "issues_delete", Description: "Delete a requirement and its comments. Nothing runs in the personal build, so no requirement has history to keep.", Method: "DELETE", Path: "/api/issues/{id}"},
		{Name: "issues_comments_add", Description: "Add a comment to a requirement", Method: "POST", Path: "/api/issues/{id}/comments"},
		{Name: "issues_close", Description: "Close a requirement", Method: "POST", Path: "/api/issues/{id}/close"},
		{Name: "issues_reopen", Description: "Reopen a closed requirement", Method: "POST", Path: "/api/issues/{id}/reopen"},

		{Name: "runners_list", Description: "List the registered machines: their operating system, their CLIs, each CLI's version and login mode, and when each was last seen.", Method: "GET", Path: "/api/runners", ReadOnly: true},
		{Name: "runner_models_probe", Description: "Probe the models a machine's CLI can reach", Method: "POST", Path: "/api/runners/{id}/models/probe"},
		{Name: "runner_maintenance_get", Description: "Read a machine's CLI maintenance state", Method: "GET", Path: "/api/runners/{id}/maintenance", ReadOnly: true},
		{Name: "runner_maintenance_apply", Description: "Update or roll back a machine's CLI", Method: "POST", Path: "/api/runners/{id}/maintenance"},

		{Name: "usage_get", Description: "Get the omp usage console: the tokens, cost, cache hits, trend and reported executions of the work reported from outside the platform", Method: "GET", Path: "/api/usage", ReadOnly: true},
		{Name: "usage_sources_list", Description: "List the reporting sources of a project. A source's token is shown once, when it is created.", Method: "GET", Path: "/api/insights/v1/sources", ReadOnly: true},
		{Name: "usage_source_create", Description: "Create a reporting source for a project and return its token once", Method: "POST", Path: "/api/insights/v1/sources"},
		{Name: "usage_source_revoke", Description: "Revoke a reporting source", Method: "DELETE", Path: "/api/insights/v1/sources/{id}"},

		{Name: "tracker_import_get", Description: "Read the tracker import's last run and its outcome", Method: "GET", Path: "/api/tracker/import", ReadOnly: true},
		{Name: "tracker_import_run", Description: "Import the open issues of every bound GitHub or GitLab repository. One repository whose credential fails is recorded with its reason; the others still import.", Method: "POST", Path: "/api/tracker/import"},

		{Name: "model_catalog_get", Description: "Get the personal model catalogue: the CLIs the account may run with and the models each offers", Method: "GET", Path: "/api/model-catalog", ReadOnly: true},
		{Name: "executor_models_list", Description: "List the selectable model IDs, the default model and the reasoning-effort levels of each built-in CLI", Method: "GET", Path: "/api/executor-models", ReadOnly: true},
		{Name: "observability_spec_get", Description: "Get the MuseForge observability contract", Method: "GET", Path: "/api/observability/spec", ReadOnly: true},
	}
}
