#!/usr/bin/env bash
# Start MuseForge from the image this checkout builds.
#
# Usage:
#   ./start.sh            install/start in the background (same as -d)
#   ./start.sh -d         install/start in the background
#   ./start.sh logs       follow platform logs
#   ./start.sh stop [service]
#                         stop containers and keep their data
#   ./start.sh down       remove containers, keep data volumes
#   ./start.sh restart    restart the platform container
#   ./start.sh pull       rebuild the image and recreate changed containers
#   ./start.sh build      rebuild the image without restarting
#   ./start.sh dev        build this checkout (compose.dev.yaml) and start it
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
source scripts/docker-access.sh
source scripts/datasource.sh

compose() {
  [[ -f .env ]] || docker_access_fail 'Not installed yet. Run ./start.sh first.'
  select_docker_access || docker_access_fail 'Docker is not running, or this user cannot access it.'
  docker_access compose --profile platform "$@"
}

lock_compose_upgrade() {
  (umask 077 && mkdir -p .local)
  # Keep the lock on the shell's file descriptor until the command exits.
  exec 9>>.local/compose-upgrade.lock
  python3 -c 'import fcntl; fcntl.flock(9, fcntl.LOCK_EX | fcntl.LOCK_NB)' 2>/dev/null ||
    docker_access_fail 'Could not lock the installation. Wait for any running upgrade to finish and retry.'
}

cmd="${1:--d}"
shift || true
case "$cmd" in
  up|-d|detach) bash scripts/install.sh ;;
  dev) LOOPFORGE_BUILD_FROM_SOURCE=1 bash scripts/install.sh ;;
  release)
    [[ -f .env ]] && python3 - <<'PY'
import pathlib,re
p=pathlib.Path('.env')
p.write_text(re.sub(r'(?m)^COMPOSE_FILE=.*\n?','',p.read_text()))
PY
    COMPOSE_FILE=compose.yaml bash scripts/install.sh
    ;;
  pull)
    [[ -f .env ]] || docker_access_fail 'Not installed yet. Run ./start.sh first.'
    set -a; source .env; set +a
    select_docker_access || docker_access_fail 'Docker is not running, or this user cannot access it.'
    lock_compose_upgrade
    bash scripts/configure.sh --project-credential-key
    # An exported empty value would override the configured key in Compose.
    [[ -n "${AGENTFLOW_PROJECT_CREDENTIAL_KEY:-}" ]] || unset AGENTFLOW_PROJECT_CREDENTIAL_KEY
    compose build platform
    compose up -d --no-build --wait --wait-timeout 180
    ;;
  logs) compose logs -f "${@:-platform}" ;;
  stop) compose stop "$@" ;;
  down) compose down --remove-orphans ;;
  restart) compose restart platform ;;
  build)
    compose build platform
    ;;
  *)
    echo "unknown: $cmd" >&2
    echo 'usage: ./start.sh [-d|logs|stop|down|restart|pull|build|dev]' >&2
    exit 1
    ;;
esac
