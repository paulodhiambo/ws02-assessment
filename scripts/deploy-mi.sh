#!/usr/bin/env bash
# Deploys all MI CARs as one unit: either every CAR ends up active, or the
# previous set is restored and the script exits non-zero.
#
#   scripts/deploy-mi.sh [env] [--dry-run]     env: dev (default) | prod
#
# Steps:
#   1. preflight: CARs built, MI healthy, management API login works
#   2. back up the Jamii*.car files currently deployed
#   3. stage the new CARs inside the container, then swap them in with mv
#      (an atomic rename, so the hot deployer never sees a half-copied file)
#   4. poll the management API until every expected app/version is active;
#      any faulty app, or a timeout, triggers a rollback to the backup
#
# Env: CAR_DIR (default mi/target/cars), MI_ADMIN_USER / MI_ADMIN_PASSWORD.
set -euo pipefail
SCRIPT_NAME=deploy-mi
source "$(dirname "$0")/lib/common.sh"

ENV_NAME_ARG="dev"; DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) ENV_NAME_ARG="$arg" ;;
  esac
done
load_env "$ENV_NAME_ARG"

CAR_DIR="${CAR_DIR:-$REPO_ROOT/mi/target/cars}"
MI_USER="${MI_ADMIN_USER:-admin}"; MI_PASS="${MI_ADMIN_PASSWORD:-admin}"
CARBONAPPS="$MI_HOME/repository/deployment/server/carbonapps"
RUN_ID="$(date +%Y%m%d%H%M%S)-$$"
STAGE_DIR="$MI_HOME/tmp/jamii-stage-$RUN_ID"
BACKUP_DIR="$MI_HOME/tmp/jamii-backup-$RUN_ID"

shopt -s nullglob
CARS=("$CAR_DIR"/*.car)
(( ${#CARS[@]} > 0 )) || die "no CARs in $CAR_DIR - run scripts/build.sh first"

# app name/version from file name: JamiiCommon_1.0.42.car -> JamiiCommon 1.0.42
EXPECTED=()
for car in "${CARS[@]}"; do
  base="$(basename "$car" .car)"
  EXPECTED+=("${base%_*}:${base##*_}")
done
log "deploying ${#CARS[@]} CARs to $ENV_NAME: ${EXPECTED[*]}"

if $DRY_RUN || [[ -z "${MI_CONTAINER_SERVICE:-}" ]]; then
  $DRY_RUN || warn "no MI_CONTAINER_SERVICE for '$ENV_NAME': this environment is not wired up, showing the plan only"
  for car in "${CARS[@]}"; do log "would deploy $(basename "$car") -> $MI_MGMT_URL"; done
  exit 0
fi

require docker; require curl; require python3
mi_sh() { compose exec -T "$MI_CONTAINER_SERVICE" sh -c "$1"; }

mi_token() {
  curl -skf -u "$MI_USER:$MI_PASS" "$MI_MGMT_URL/login" | json_get "d['AccessToken']"
}

applications() {
  curl -skf -H "Authorization: Bearer $TOKEN" "$MI_MGMT_URL/applications"
}

# Prints "ok", "faulty:<names>" or "pending:<names>" for the expected set.
deployment_state() {
  applications | python3 -c '
import sys, json
d = json.load(sys.stdin)
active = {(a["name"], a["version"]) for a in d.get("activeList", [])}
faulty_raw = json.dumps(d.get("faultyList", []))
expected = [tuple(x.split(":", 1)) for x in sys.argv[1:]]
faulty = [n for n, v in expected if n in faulty_raw]
pending = [f"{n}:{v}" for n, v in expected if (n, v) not in active]
print("faulty:" + ",".join(faulty) if faulty else "pending:" + ",".join(pending) if pending else "ok")
' "${EXPECTED[@]}"
}

wait_for_expected() {
  local deadline=$((SECONDS + ${MI_DEPLOY_TIMEOUT_SECONDS:-120})) state
  while (( SECONDS < deadline )); do
    state="$(deployment_state)"
    case "$state" in
      ok) return 0 ;;
      faulty:*) log "MI reports faulty applications: ${state#faulty:}"; return 1 ;;
    esac
    sleep 3
  done
  log "timed out waiting for: ${state#pending:}"
  return 1
}

rollback() {
  warn "rolling back to the previously deployed CARs"
  mi_sh "rm -f '$CARBONAPPS'/Jamii*.car; if ls '$BACKUP_DIR'/*.car >/dev/null 2>&1; then mv '$BACKUP_DIR'/*.car '$CARBONAPPS'/; fi" || true
  sleep 20
  warn "rollback finished; currently active: $(applications | json_get "', '.join(a['name'] + ':' + a['version'] for a in d.get('activeList', []))" || echo unknown)"
}

# 1. preflight
wait_for "MI management API" 180 mi_token || die "MI management API not reachable at $MI_MGMT_URL"
TOKEN="$(mi_token)"

# 2. back up current CARs, 3. stage new ones
mi_sh "mkdir -p '$STAGE_DIR' '$BACKUP_DIR' && for f in '$CARBONAPPS'/Jamii*.car; do [ -e \"\$f\" ] && cp \"\$f\" '$BACKUP_DIR'/; done; true"
for car in "${CARS[@]}"; do
  compose cp "$car" "$MI_CONTAINER_SERVICE:$STAGE_DIR/" >/dev/null 2>&1 || die "could not copy $(basename "$car") into the MI container"
done
mi_sh "chown -R wso2carbon '$STAGE_DIR' 2>/dev/null; true"
log "staged; swapping into $CARBONAPPS"
mi_sh "rm -f '$CARBONAPPS'/Jamii*.car && mv '$STAGE_DIR'/*.car '$CARBONAPPS'/"

# 4. verify all-or-nothing
if ! wait_for_expected; then
  compose logs --since 3m "$MI_CONTAINER_SERVICE" 2>&1 | grep -E 'ERROR' | grep -v '^\s*\S*\s*|\s+at ' | tail -20 >&2 || true
  rollback
  die "MI deployment failed; previous version restored"
fi

mi_sh "rm -rf '$STAGE_DIR' '$BACKUP_DIR'"
log "all ${#CARS[@]} CARs active on $ENV_NAME"
curl -skf -H "Authorization: Bearer $TOKEN" "$MI_MGMT_URL/apis" \
  | json_get "'\n'.join('  ' + a['name'] + '  ' + a['url'] for a in d['list'])" >&2
