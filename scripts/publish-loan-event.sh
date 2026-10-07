#!/usr/bin/env bash
# Publishes a loan-application event to RabbitMQ (exchange loan.events,
# routing key loan.applications) through the management HTTP API, so no AMQP
# client is needed. For demos and scripts/test.sh --events.
#
#   scripts/publish-loan-event.sh tests/events/valid-application.json [correlation-id]
#   echo '{"broken":' | scripts/publish-loan-event.sh - [correlation-id]
#
# Env: RABBITMQ_MGMT_URL (default http://localhost:15672), RABBITMQ_USER / RABBITMQ_PASSWORD.
set -euo pipefail
SCRIPT_NAME=publish-loan-event
source "$(dirname "$0")/lib/common.sh"

file="${1:?usage: publish-loan-event.sh <event.json|-> [correlation-id]}"
correlation="${2:-evt-$(date +%s)-$RANDOM}"
[[ "$file" == - ]] && file=/dev/stdin
jamii rabbit publish --routing-key loan.applications --header "x-correlation-id=$correlation" < "$file"
log "published to loan.applications (x-correlation-id: $correlation)"
