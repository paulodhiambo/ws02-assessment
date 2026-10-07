#!/usr/bin/env bash
# Builds the jamii helper CLI (tools/) into .tools/bin/jamii; a no-op when the
# binary is newer than every source file. Uses local Go if installed,
# otherwise cross-compiles for this machine in the golang image.
#
#   scripts/build-tools.sh [--force]
set -euo pipefail
SCRIPT_NAME=build-tools
source "$(dirname "$0")/lib/common.sh"

if [[ "${1:-}" != "--force" && -x "$JAMII_BIN" ]] &&
   [[ -z "$(find "$REPO_ROOT/tools" \( -name '*.go' -o -name go.mod \) -newer "$JAMII_BIN" | head -1)" ]]; then
  exit 0
fi

mkdir -p "$(dirname "$JAMII_BIN")"
if command -v go >/dev/null; then
  (cd "$REPO_ROOT/tools" && CGO_ENABLED=0 go build -trimpath -o "$JAMII_BIN" .)
else
  require docker
  case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) die "unsupported OS $(uname -s)" ;; esac
  case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) die "unsupported arch $(uname -m)" ;; esac
  docker run --rm -v "$REPO_ROOT:/repo" -w /repo/tools \
    -e CGO_ENABLED=0 -e GOOS="$os" -e GOARCH="$arch" -e GOFLAGS=-buildvcs=false \
    golang:1.27-alpine go build -trimpath -o "/repo/${JAMII_BIN#"$REPO_ROOT"/}" .
fi
log "built $(basename "$JAMII_BIN") ($("$JAMII_BIN" help >/dev/null && echo ok))"
