#!/usr/bin/env bash
# Explicit local installation, schema initialization and catalog installation.
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/configure.sh
set -a
source .env
set +a
source scripts/datasource.sh
mkdir -p .local/bin .local/workspaces
# The default datasource is one SQLite file: no database server is started.
# MySQL installations still get their server, their schema extensions and
# their starter catalog.
if datasource_is_mysql; then
  docker compose --profile mysql up -d --wait mysql
  docker compose --profile mysql run --rm --no-deps artifact-migration
fi
bash scripts/build-binaries.sh
if [[ ! -f .local/admin-password ]]; then
  (umask 077; python3 -c 'import secrets; print(secrets.token_urlsafe(18))' > .local/admin-password)
fi
.local/bin/loopforge-account -user admin -admin < .local/admin-password
if datasource_is_mysql; then
  bash scripts/install-observability-schema.sh
  docker compose exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u loopforge loopforge' < migrations/seed/001_public_catalog.sql
  docker compose exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u loopforge loopforge' < migrations/seed/002_welcome_project.sql
fi
printf 'Open http://localhost:%s — account: admin — password: .local/admin-password\n' "$AGENTFLOW_HTTP_PORT"
export AGENTFLOW_RUNNER_BINARY="$PWD/.local/bin/agentflow-runner-{os}-{arch}"
export AGENTFLOW_GATE_MCP_BINARY="$PWD/.local/bin/agentflow-gate-mcp-{os}-{arch}"
printf '%s\n' "$$" > .local/platform.pid
exec .local/bin/agentflow -addr "${LOOPFORGE_BIND:-0.0.0.0}:$AGENTFLOW_HTTP_PORT" -workspaces "$PWD/.local/workspaces" -datasource "$(platform_datasource)"
