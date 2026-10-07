#!/usr/bin/env bash
# Builds every deployable artifact.
#
#   scripts/build.sh [--skip-tests] [--version 1.0.42]
#
#   MI    mvn clean install in mi/          -> mi/target/cars/*.car
#   APIM  one apictl project per API, with the shared custom policies copied in,
#         plus the API Product              -> dist/apim/<name>/ and dist/apim/<name>.zip
set -euo pipefail
SCRIPT_NAME=build
source "$(dirname "$0")/lib/common.sh"

SKIP_TESTS=false
VERSION="${CAR_VERSION:-1.0.0}"
while (( $# )); do
  case "$1" in
    --skip-tests) SKIP_TESTS=true ;;
    --version) VERSION="$2"; shift ;;
    -h|--help) sed -n '2,8p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

require mvn; require python3; require zip

# --- Micro Integrator ------------------------------------------------------
log "MI: mvn clean install (car.version=$VERSION)"
mvn -B -q -f "$REPO_ROOT/mi/pom.xml" clean install -Dcar.version="$VERSION" -DskipTests="$SKIP_TESTS"
ls -1 "$REPO_ROOT"/mi/target/cars/*.car | sed 's/^/  /' >&2

# --- API Manager -----------------------------------------------------------
DIST="$REPO_ROOT/dist/apim"
rm -rf "$DIST" && mkdir -p "$DIST"
for project in "$REPO_ROOT"/apim/*-api; do
  name="$(basename "$project")"
  [[ -f "$project/api.yaml" && -f "$project/Definitions/swagger.yaml" ]] || die "$name: missing api.yaml or Definitions/swagger.yaml"
  rsync -a --exclude README.md "$project/" "$DIST/$name/"
  # Copy in every shared policy the API references.
  for policy in $(grep -oE 'policyName: [A-Za-z0-9_]+' "$project/api.yaml" | awk '{print $2}' | sort -u); do
    files=("$REPO_ROOT/apim/policies/${policy}_"*.{yaml,j2})
    [[ -e "${files[0]}" ]] || die "$name references unknown policy '$policy' (expected apim/policies/${policy}_<version>.yaml)"
    mkdir -p "$DIST/$name/Policies"
    cp "${files[@]}" "$DIST/$name/Policies/"
  done
  (cd "$DIST" && zip -qr "$name.zip" "$name")
  log "APIM: packaged $name"
done
rsync -a --exclude README.md "$REPO_ROOT/apim/product/" "$DIST/product/"
(cd "$DIST" && zip -qr product.zip product)
log "APIM: packaged product"
log "done"
