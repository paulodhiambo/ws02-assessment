#!/usr/bin/env bash
# Installs apictl into .tools/apictl (used by CI; locally any apictl 4.6.x on PATH works).
#
#   scripts/install-apictl.sh [version]     default 4.6.4
set -euo pipefail
SCRIPT_NAME=install-apictl
source "$(dirname "$0")/lib/common.sh"

VERSION="${1:-${APICTL_VERSION:-4.6.4}}"
DEST="$REPO_ROOT/.tools"
if [[ -x "$DEST/apictl/apictl" ]] && "$DEST/apictl/apictl" version 2>/dev/null | grep -q "Version: $VERSION"; then
  log "apictl $VERSION already installed"; exit 0
fi

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) die "unsupported OS $(uname -s)" ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) die "unsupported arch $(uname -m)" ;; esac
url="https://github.com/wso2/product-apim-tooling/releases/download/v$VERSION/apictl-$VERSION-$os-$arch.tar.gz"

log "downloading $url"
rm -rf "$DEST/apictl" && mkdir -p "$DEST"
curl -sfL "$url" | tar -xz -C "$DEST"
"$DEST/apictl/apictl" version | head -1
