#!/usr/bin/env bash
# Start MuseForge from this checkout, on one SQLite file, bound to the loopback
# interface. There is no sign-in: the console records the account on first use.
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/configure.sh
set -a
source .env
set +a
source scripts/datasource.sh
mkdir -p .local/bin
bash scripts/build-binaries.sh
printf 'Open http://127.0.0.1:%s — there is no sign-in.\n' "$AGENTFLOW_HTTP_PORT"
printf '%s\n' "$$" > .local/platform.pid
exec .local/bin/agentflow \
  -addr "127.0.0.1:$AGENTFLOW_HTTP_PORT" \
  -datasource "$(platform_datasource)"