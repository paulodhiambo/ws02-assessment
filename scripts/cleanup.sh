#!/usr/bin/env bash
# Tears down the local environment.
#
#   scripts/cleanup.sh            stop and remove containers (volumes kept)
#   scripts/cleanup.sh --all      also remove volumes (DB data, deployed CARs) and build output
set -euo pipefail
SCRIPT_NAME=cleanup
source "$(dirname "$0")/lib/common.sh"

if [[ "${1:-}" == "--all" ]]; then
  compose --profile apim down -v --remove-orphans
  rm -rf "$REPO_ROOT/mi/target" "$REPO_ROOT/dist" "$REPO_ROOT/target" "$REPO_ROOT/tests/postman/apim.env.json"
  log "containers, volumes and build output removed"
else
  compose --profile apim down --remove-orphans
  log "containers removed (volumes kept; use --all to remove them)"
fi
