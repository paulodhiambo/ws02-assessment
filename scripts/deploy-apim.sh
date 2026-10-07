#!/usr/bin/env bash
# Imports/updates the three APIs and the API Product in API Manager with
# apictl, as one unit: if any import fails, APIs already updated in this run
# are restored from a pre-deploy export (or deleted if they are new), and the
# script exits non-zero.
#
#   scripts/deploy-apim.sh [env] [--dry-run]     env: dev (default) | prod
#
# Needs: apictl 4.6.x on PATH, packaged projects in dist/apim (scripts/build.sh).
# Env:   APIM_ADMIN_USER / APIM_ADMIN_PASSWORD, APICTL_CONFIG_DIR (optional,
#        keeps apictl state out of $HOME in CI).
set -euo pipefail
SCRIPT_NAME=deploy-apim
source "$(dirname "$0")/lib/common.sh"

ENV_NAME_ARG="dev"; DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) ENV_NAME_ARG="$arg" ;;
  esac
done
load_env "$ENV_NAME_ARG"

DIST="$REPO_ROOT/dist/apim"
# project dir : API name : MI resource path
APIS=(
  "accounts-api:JamiiAccountsAPI:/accounts"
  "customers-api:JamiiCustomersAPI:/customers"
  "loan-eligibility-api:JamiiLoanEligibilityAPI:/loans"
)
API_VERSION=v1
PRODUCT_NAME=JamiiCoreBankingProduct; PRODUCT_VERSION=1.0.0
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

for entry in "${APIS[@]}"; do
  [[ -d "$DIST/${entry%%:*}" ]] || die "$DIST/${entry%%:*} not found - run scripts/build.sh first"
done
[[ -d "$DIST/product" ]] || die "$DIST/product not found - run scripts/build.sh first"

# Per-environment endpoint overrides: the gateway reaches MI at MI_BACKEND_FOR_APIM.
write_params() {  # <path-suffix> <file>
  cat > "$2" <<EOF
environments:
  - name: $APICTL_ENV
    configs:
      endpoints:
        production:
          url: ${MI_BACKEND_FOR_APIM}$1
        sandbox:
          url: ${MI_BACKEND_FOR_APIM}$1
EOF
}

if $DRY_RUN; then
  for entry in "${APIS[@]}"; do
    IFS=: read -r dir name path <<<"$entry"
    log "would import $name $API_VERSION -> $APICTL_ENV ($APIM_URL), backend ${MI_BACKEND_FOR_APIM}${path}"
  done
  log "would import API product $PRODUCT_NAME $PRODUCT_VERSION"
  exit 0
fi

require apictl
APIM_USER="${APIM_ADMIN_USER:-admin}"; APIM_PASS="${APIM_ADMIN_PASSWORD:-admin}"

wait_for "APIM ($APIM_URL)" 300 curl -skf "$APIM_URL/services/Version" || die "APIM not reachable at $APIM_URL"
apictl remove env "$APICTL_ENV" >/dev/null 2>&1 || true
apictl add env "$APICTL_ENV" --apim "$APIM_URL" >/dev/null
apictl login "$APICTL_ENV" -u "$APIM_USER" -p "$APIM_PASS" -k >/dev/null
log "apictl logged in to $APICTL_ENV"

api_exists() {
  apictl get apis -e "$APICTL_ENV" -k -q "name:$1" --format '{{.Name}} {{.Version}}' 2>/dev/null | grep -qx "$1 $API_VERSION"
}

# 1. back up what is there now (exported zips in $WORK/backup-<name>.zip)
backup_of() { [[ -f "$WORK/backup-$1.zip" ]] && echo "$WORK/backup-$1.zip"; }
for entry in "${APIS[@]}"; do
  IFS=: read -r dir name path <<<"$entry"
  if api_exists "$name"; then
    out="$(apictl export api -n "$name" -v "$API_VERSION" -e "$APICTL_ENV" -k --latest 2>&1)" || die "could not back up $name: $out"
    cp "$(sed -n 's/^Find the exported API at //p' <<<"$out")" "$WORK/backup-$name.zip"
    log "backed up $name"
  fi
done

IMPORTED=()
rollback() {
  warn "rolling back APIs imported in this run: ${IMPORTED[*]-none}"
  local name
  for name in ${IMPORTED[@]+"${IMPORTED[@]}"}; do
    if backup="$(backup_of "$name")"; then
      apictl import api -f "$backup" -e "$APICTL_ENV" -k --update --rotate-revision >/dev/null 2>&1 \
        && warn "restored $name from backup" || warn "could not restore $name - manual action needed"
    else
      apictl delete api -n "$name" -v "$API_VERSION" -e "$APICTL_ENV" -k >/dev/null 2>&1 \
        && warn "deleted newly created $name" || warn "could not delete $name - manual action needed"
    fi
  done
}

# 2. import every API; stop at the first failure
for entry in "${APIS[@]}"; do
  IFS=: read -r dir name path <<<"$entry"
  write_params "$path" "$WORK/$dir-params.yaml"
  log "importing $name ($dir), backend ${MI_BACKEND_FOR_APIM}${path}"
  IMPORTED+=("$name")
  if ! apictl import api -f "$DIST/$dir" -e "$APICTL_ENV" -k --update --rotate-revision --preserve-provider \
        --params "$WORK/$dir-params.yaml" 2>&1 | tee "$WORK/$dir.log" | grep -q "Successfully imported"; then
    cat "$WORK/$dir.log" >&2
    rollback
    die "import of $name failed; APIM restored to its previous state"
  fi
done

# 3. the product depends on all three APIs being in place
log "importing API product $PRODUCT_NAME"
if ! apictl import api-product -f "$DIST/product" -e "$APICTL_ENV" -k --update-api-product --rotate-revision \
      2>&1 | tee "$WORK/product.log" | grep -q "Successfully imported"; then
  cat "$WORK/product.log" >&2
  rollback
  die "import of $PRODUCT_NAME failed; APIM restored to its previous state"
fi

# 4. revisions reach the gateway asynchronously: wait until every operation answers
#    (401 without credentials) instead of 404.
gateway_serves() { [[ "$(curl -sk -o /dev/null -w '%{http_code}' "$1")" != 404 ]]; }
for entry in "${APIS[@]}"; do
  IFS=: read -r dir name path <<<"$entry"
  context="$(sed -n 's/^  context: //p' "$DIST/$dir/api.yaml")"
  for target in $(sed -n 's/^    - target: //p' "$DIST/$dir/api.yaml" | sed 's/{[^}]*}/probe/g'); do
    url="$APIM_GATEWAY_URL$context/$API_VERSION$target"
    wait_for "gateway route $context/$API_VERSION$target" 120 gateway_serves "$url" \
      || die "$name imported but $url is not served by the gateway"
  done
done

log "deployed to $APICTL_ENV:"
apictl get apis -e "$APICTL_ENV" -k -q "name:Jamii" >&2
apictl get api-products -e "$APICTL_ENV" -k >&2
