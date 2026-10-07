#!/usr/bin/env bash
# Onboards the demo consuming application through the APIM Developer Portal
# REST API (create app, subscribe to the three APIs and the API Product,
# generate an OAuth2 token and an API key), then prints shell exports and
# ready-to-run curl commands. Implementation: tools/apimconsumer.
#
#   scripts/apim-demo-consumer.sh [--apim https://localhost:9443] [--gateway https://localhost:8243]
#   eval "$(scripts/apim-demo-consumer.sh | grep '^export ')"     # sets GW, TOKEN, APIKEY
#
# Credentials: APIM_ADMIN_USER / APIM_ADMIN_PASSWORD (default admin/admin).
# Writes tests/postman/apim.env.json for scripts/test.sh --integration --apim.
set -euo pipefail
SCRIPT_NAME=apim-demo-consumer
source "$(dirname "$0")/lib/common.sh"
jamii apim-consumer --env-file "$REPO_ROOT/tests/postman/apim.env.json" "$@"
