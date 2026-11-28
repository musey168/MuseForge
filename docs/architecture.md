# Architecture

MuseForge keeps the upstream Go module name `agentflow`, the package boundaries and
the `AGENTFLOW_` environment variables, so the code that survived the trim still
reads like the code it grew out of.

## What the personal build is made of

| Directory | Responsibility |
| --- | --- |
| `cmd/agentflow` | The API process. It opens the datasource, serves the HTTP API and the embedded console, and nothing else — there is no worker role and no execution engine. |
| `cmd/agentflow-mcp` | The public MCP server: the platform's own HTTP API exposed as MCP tools. |
| `cmd/agentflow-runner` | The machine daemon. It registers, heartbeats and reports the agent CLIs it finds. It never receives work. |
| `internal/repo` | Persistence, one repository per business domain, over SQLite (and still MySQL). |
| `internal/httpapi` | The HTTP handlers and the MCP mount, one handler struct per business domain. |
| `internal/domain` | The entities: the requirement, the project, the user and its credentials, the runner, the reported execution. |
| `internal/tracker` | The external tracker adapters: GitHub and GitLab. One listing path, one credential lookup, one local-write path. |
| `internal/agentcli` | The agent CLI vocabulary and the login-model probe the machine inventory reports. |
| `internal/observability` | The telemetry the platform still emits about itself. |
| `web` | The embedded console: the requirement list, the project list, personal settings, the machine inventory and the usage console. |
| `omp-extension` | The omp extension that reports a finished session's usage. It is an installable artifact, not part of the platform binary. |
| `migrations` | The data migrations an operator applies by hand. A fresh install creates its schema from the model manifest alone. |
| `docs/personal-scope` | The specification, the scope decision record and the measured test baseline. |

## The shape of the thing

**One account.** There is no login. The platform resolves its one account
deterministically and personal settings hang off it. The reporting credential the omp
extension uses is a separate thing with its own lifecycle — revocable and expiring.

**One datasource.** SQLite is the first-class datasource: a file, not a server. The
same gorm code paths serve MySQL, which stays supported for the deployments that
need it. A fresh database is created from the model manifest (`allModels()` in
`internal/repo/models.go`) — no migration step, no seed.

**One exposure.** The platform binds to the loopback interface. An unauthenticated
service on a public interface would expose the requirements, the credentials and the
cost data, so a non-loopback bind is refused at startup.

**No execution.** The LOOP engine, the sandbox runtime, the runner task queue, the
previews, the scheduled tasks, the knowledge base, the enterprise IM bots and the
chat workbench are gone from the code, not disabled behind a flag. The machine
inventory survives as an inventory: the daemon reports which CLIs a machine has.

**A ledger, not a workshop.** The usage console aggregates exclusively from the
executions reported from outside. It reads `insight_executions` and never the run
records, because there are no run records.

## Where the upstream vocabulary survives

The upstream names survive where the concept survives — `domain.Issue` is still the
requirement's entity, `IssueStoreAdapter` is still the tracker's port. The concepts
that are gone took their names with them: there is no run, no workflow, no
sandbox, no executor, no preview, no domain, no member and no approval left in the
code.

`GLOSSARY.md` is the authority on the personal build's vocabulary, and
`docs/personal-scope/decision.md` is the authority on scope.
