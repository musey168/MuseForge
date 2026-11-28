# Acceptance matrix

Which automated cases protect each capability of the personal build, and what they
need.

| Capability | Automated coverage | Needs |
| --- | --- | --- |
| The platform starts on a SQLite file and creates its schema from the model manifest | `internal/repo/sqlite_datasource_test.go`, `internal/httpapi/sqlite_platform_test.go` | — |
| A restart against the same file keeps the requirements, projects, credentials and reported usage | `internal/repo/sqlite_datasource_test.go` | — |
| The datasource forms — a `file:` URI, a bare path, a MySQL DSN — are told apart | `internal/repo/sqlite_datasource_test.go` | — |
| Requirements: create, edit, comment, tag, close, reopen, delete | `internal/httpapi/issues_test.go`, `internal/httpapi/issue_routes_test.go` | — |
| The requirement list is filtered by project, status and tag | `internal/httpapi/issues_test.go` | — |
| A requirement's detail shows its comments and its links, and no flow progress | `internal/httpapi/issues_test.go` | — |
| Projects: create, update, archive, restore — with no roster | `internal/httpapi/project_admin_layout_test.go`, `web/project_admin_layout_test.js` | — |
| Personal settings: the model keys, the GitHub authorization and the Git credentials are stored and read back, and no secret is returned | `internal/httpapi/users_test.go`, `web/workbench-credentials.test.mjs` | — |
| The reporting credential: grant, list, revoke, refuse an expired one | `internal/httpapi/insights_test.go`, `internal/httpapi/insights_usage_test.go` | — |
| The usage console: tokens, cost, cache-hit rate, execution count, model and provider breakdowns, the day/week/month trend, the execution list, and an explicit empty state | `internal/repo/insights_console_test.go`, `internal/httpapi/insights_usage_test.go`, `webtest/tests/observability-views.test.mjs` | — |
| A reported cost wins over the price table; a missing one is priced and marked estimated | `internal/repo/insights_console_test.go` | — |
| A malformed report is counted and skipped without breaking the console | `internal/httpapi/insights_usage_test.go` | — |
| The GitHub import: one requirement per issue, pull requests skipped, the issue's number, title, body, state and link | `internal/tracker/import_test.go`, `internal/httpapi/tracker_import_test.go` | a stubbed GitHub endpoint |
| A re-import refreshes the issue-backed fields and leaves a locally edited description alone | `internal/tracker/import_test.go` | a stubbed GitHub endpoint |
| The GitLab import: one requirement per issue, the state and type mapped, the link back | `internal/tracker/gitlab_import_test.go` | a stubbed GitLab endpoint |
| The import can be triggered on demand and its last run and outcome are visible | `internal/httpapi/tracker_import_test.go`, `webtest/tests/tracker-import.test.mjs` | — |
| A repository whose credential is missing fails visibly while the others still import | `internal/tracker/import_test.go` | a stubbed endpoint |
| The machine inventory: register, heartbeat, report the CLIs, deregister | `internal/runner/daemon_test.go` | — |
| The CLI catalogue shows the binary path, version, enabled state, login mode and models | `internal/runner/client_test.go`, `webtest/tests/runner-codebuddy-environment.test.mjs` | — |
| The control socket answers the CLI maintenance and the model probe, and refuses anything else | `internal/runner/control_test.go` | — |
| The platform refuses to bind a non-loopback address | `internal/config` bind check test | — |
| Every retired route is not registered and does not answer; every retired table is not created | `internal/httpapi/removed_routes_test.go`, `internal/repo/removed_tables_test.go` | — |
| The route surface holds only the personal build's routes | `internal/httpapi/removed_routes_test.go` | — |
| The console's DOM behaviour: the requirement list, the drawer, the usage console, the project page | `webtest/tests/*.test.mjs` | — |
| The console's modules parse and carry the expected markup | `web/syntax_test.go`, `web/*_test.js` | node |
