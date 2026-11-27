#!/usr/bin/env bash
# The platform's datasource rule, mirrored for the shell scripts.
#
# The platform (internal/repo/repo.go) reads the datasource in this order:
#   1. the -datasource flag
#   2. AGENTFLOW_DATASOURCE_DSN
#   3. AGENTFLOW_MYSQL_DSN
# and tells the two datasources apart by shape:
#   "file:" prefix            → SQLite (a file path, or a "file:" URI)
#   contains "@", "(" or "://" → MySQL DSN
#   anything else             → SQLite file path

# platform_datasource prints the datasource the platform should use.
platform_datasource() {
  local dsn="${AGENTFLOW_DATASOURCE_DSN:-}"
  if [[ -z "$dsn" ]]; then
    dsn="${AGENTFLOW_MYSQL_DSN:-}"
  fi
  printf '%s' "$dsn"
}

# datasource_is_mysql reports whether the configured datasource is MySQL, and
# so whether the database server and its SQL scripts are needed at all.
datasource_is_mysql() {
  local dsn
  dsn="$(platform_datasource)"
  if [[ -z "$dsn" ]]; then
    return 1
  fi
  if [[ "$dsn" == file:* ]]; then
    return 1
  fi
  if [[ "$dsn" == *@* || "$dsn" == *'('* || "$dsn" == *'://'* ]]; then
    return 0
  fi
  return 1
}
