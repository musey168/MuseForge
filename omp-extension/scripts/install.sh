#!/usr/bin/env bash
# Install the MuseForge usage-reporting extension for omp.
#
# The extension is a plain file: omp loads `*.ts` files found in its extension
# directories, so installing is copying one file and configuring a credential.
set -euo pipefail

usage() {
  cat <<'EOF'
Install the MuseForge usage-reporting extension for omp.

Usage:
  install.sh --user              install for this user:
                                 ${OMP_HOME:-~/.omp}/agent/extensions/museforge-usage.ts
  install.sh --project [DIR]     install for one project (DIR defaults to .):
                                 DIR/.omp/extensions/museforge-usage.ts
  install.sh --dir PATH          install into an explicit extension directory
  install.sh --print-config      print the configuration template
  install.sh --help              show this message

The installed file path is printed on stdout. Configure a data-source token in
~/.config/museforge/omp-usage.json (or DIR/.omp/museforge-usage.json); see
omp-extension/README.md.
EOF
}

print_config() {
  cat <<'EOF'
{
  "url": "http://127.0.0.1:8080",
  "token": "afi_XXXXXXXXXXXXXXXXXXXXXXXX",
  "statsDb": "~/.omp/stats.db",
  "timeoutMs": 1500
}
EOF
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_file="$root/src/museforge-usage.ts"
[ -f "$source_file" ] || {
  echo "missing extension source: $source_file" >&2
  exit 1
}

target=""
while [ $# -gt 0 ]; do
  case "$1" in
    --user)
      target="${OMP_HOME:-$HOME/.omp}/agent/extensions"
      shift
      ;;
    --project)
      shift
      dir="."
      if [ $# -gt 0 ] && [ "${1#--}" = "$1" ]; then
        dir="$1"
        shift
      fi
      target="$dir/.omp/extensions"
      ;;
    --dir)
      shift
      [ $# -gt 0 ] || {
        echo "--dir needs a path" >&2
        exit 2
      }
      target="$1"
      shift
      ;;
    --print-config)
      print_config
      exit 0
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ -z "$target" ]; then
  usage >&2
  exit 2
fi

mkdir -p "$target"
install -m 0644 "$source_file" "$target/museforge-usage.ts"
echo "$(cd "$target" && pwd)/museforge-usage.ts"
