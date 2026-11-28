# Deployment

MuseForge is one container and one SQLite file. There is no database server to
install, no worker role, and no login.

## Running it

### From source

```bash
make build
./bin/agentflow -datasource file:./museforge.db -addr 127.0.0.1:8080
```

The platform creates its schema from the model manifest on first start — there is no
migration step and no seed. Open <http://127.0.0.1:8080>.

### With Docker

```bash
bash scripts/configure.sh          # writes .env: the datasource, the session secret
docker compose up -d --wait
./start.sh logs                     # follow the platform log
./start.sh stop                     # stop; ./start.sh down removes the containers
```

`scripts/configure.sh` writes a SQLite datasource by default
(`AGENTFLOW_DATASOURCE_DSN=file:<data dir>/museforge.db`). MySQL is still supported —
set `AGENTFLOW_MYSQL_DSN` instead and bring your own server — but nothing in the
personal build needs it.

## The settings that matter

| setting | meaning |
|---|---|
| `-datasource` / `AGENTFLOW_DATASOURCE_DSN` | `file:/path/to/museforge.db`, a bare path, or a MySQL DSN. The platform tells the three forms apart. |
| `-addr` / `AGENTFLOW_HTTP_PORT` | the listen address. **Loopback only.** A non-loopback bind is refused at startup — the platform is unauthenticated, so a public interface would expose your requirements, credentials and cost data. The one exception is a container deployment, which sets `AGENTFLOW_CONTAINER_BIND=1`: inside the container's own network namespace the wildcard reaches only the container, and the published port on the host's loopback interface is the exposure boundary. A host deployment never sets it. |
| `AGENTFLOW_SESSION_SECRET` | required. |
| `AGENTFLOW_AUTH_REQUIRED` | `false` for the personal build. There is no login. |
| `AGENTFLOW_CREDENTIAL_KEYS` | optional; without it the platform stores credentials unencrypted and reports `credential_key_missing` when a new secret is saved. |
| `AGENTFLOW_MCP_TOKEN` | the bearer token the MCP server accepts. |
| `AGENTFLOW_GITHUB_CLIENT_ID` | the OAuth app the GitHub device-code authorization uses. |
| `AGENTFLOW_GIT_SSH_KNOWN_HOSTS` | the pinned SSH host keys the Git credentials are checked against. |

## Backing it up

One file. Copy it:

```bash
cp "$LOOPFORGE_DATA_DIR/museforge.db" /somewhere/museforge-$(date +%F).db
```

Stop the platform first, or use SQLite's own `.backup` — copying a live database file
can capture a torn write.

## The machine daemon

A machine that runs agent CLIs installs the daemon so MuseForge can list what it has:

```bash
curl -fsSL http://127.0.0.1:8080/runners/install.sh | bash
```

The daemon registers, heartbeats and reports the CLIs it finds — their paths,
versions, login state and the models they can reach. It never receives work.

**What it keeps on disk.** One state file, `~/.agentflow-runner/state.json` (or
`-state`), and — only when you set `AGENTFLOW_MANAGED_CLI_ROOT` — the CLIs it
manages there. It creates no per-session directory and copies no helper
distribution: a start that leaves anything else behind is a defect, and
`internal/runner/daemon_test.go` fails if one does. An older team-build runner
did keep a full git distribution per start under
`~/Library/Caches/LoopForge/git-helpers` — several hundred megabytes each, never
reclaimed. If that directory exists on a machine, nothing in this build reads
it, and `rm -rf ~/Library/Caches/LoopForge` reclaims it.

## The omp extension

The extension that reports a finished session's usage is installed separately, and
it is not part of the platform image. See
[`omp-extension/README.md`](../omp-extension/README.md).

## Upgrading

The platform's schema only ever gains columns and tables it needs. There is no
downgrade path: an older binary that predates a column will not read a database
that has it. Take the backup above before upgrading.

## Migrating from the team build

The personal build reads the same SQLite and MySQL code paths, but it does not read
the team build's data: the accounts, the projects' rosters, the runs and the
deliveries are gone. There is nothing to migrate — start a fresh database and import
your requirements from GitHub or GitLab.
