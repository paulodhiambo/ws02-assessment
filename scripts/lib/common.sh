# Shared helpers for scripts/*.sh. Source, don't execute.
# shellcheck shell=bash

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "${SCRIPT_NAME:-jamii}" "$*" >&2; }
warn() { printf '\033[1;33m[%s] WARN\033[0m %s\n' "${SCRIPT_NAME:-jamii}" "$*" >&2; }
die()  { printf '\033[1;31m[%s] ERROR\033[0m %s\n' "${SCRIPT_NAME:-jamii}" "$*" >&2; exit 1; }

# load_env <name>: exports infrastructure/config/<name>.env (prod: prod.env).
# Values already set in the environment (e.g. by Jenkins) win.
load_env() {
  local name="$1" file="$REPO_ROOT/infrastructure/config/$1.env"
  [[ -f "$file" ]] || die "environment file not found: $file (for prod, copy prod.env.example to prod.env)"
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    [[ "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
    key="${BASH_REMATCH[1]}"; value="${BASH_REMATCH[2]}"
    value="${value%"${value##*[![:space:]]}"}"
    [[ -n "${!key:-}" ]] || export "$key=$value"
  done < "$file"
  log "environment: $name"
}

require() { command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not installed"; }

# json_get <python expression over `d`>: reads JSON on stdin.
json_get() { python3 -c "import sys, json; d = json.load(sys.stdin); print($1)"; }

compose() { docker compose -f "$REPO_ROOT/docker-compose.yml" "$@"; }

# wait_for <description> <timeout-seconds> <command...>
wait_for() {
  local what="$1" timeout="$2"; shift 2
  local deadline=$((SECONDS + timeout))
  until "$@" >/dev/null 2>&1; do
    (( SECONDS < deadline )) || return 1
    sleep 3
  done
  log "$what: ready"
}
