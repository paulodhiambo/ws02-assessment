#!/usr/bin/env bash
# Runs the test suites.
#
#   scripts/test.sh                         unit tests (default)
#   scripts/test.sh --integration           Postman collection via newman, direct to MI
#   scripts/test.sh --integration --apim    same collection through the APIM gateway
#   scripts/test.sh --chaos                 stops the DB/backends and checks the error mapping
#   scripts/test.sh --events                bonus B: RabbitMQ consumer, DLQ and retries
#   scripts/test.sh --all                   unit + integration (MI) + chaos + events
#
# Unit:        mock services, MI artifact tests and the helper CLI (go vet + go test).
# Integration: needs the compose stack with CARs deployed (and APIs imported for --apim).
#              newman runs in Docker on the compose network, so no local Node/newman is needed.
#              The "mock-backend" folder runs only when MI points at the local customer mock.
set -euo pipefail
SCRIPT_NAME=test
source "$(dirname "$0")/lib/common.sh"

UNIT=false; INTEGRATION=false; CHAOS=false; EVENTS=false; TARGET=mi
(( $# )) || UNIT=true
for arg in "$@"; do
  case "$arg" in
    --unit) UNIT=true ;;
    --integration) INTEGRATION=true ;;
    --chaos) CHAOS=true ;;
    --events) EVENTS=true ;;
    --apim) TARGET=apim ;;
    --all) UNIT=true; INTEGRATION=true; CHAOS=true; EVENTS=true ;;
    -h|--help) sed -n '2,15p' "$0"; exit 0 ;;
    *) die "unknown argument: $arg" ;;
  esac
done

NETWORK="${COMPOSE_NETWORK:-jamii_default}"
FAILED=0

# go_test <dir relative to the repo>: go vet + go test, with local Go if
# installed, otherwise in the golang image (the repo is mounted read-only).
go_test() {
  if command -v go >/dev/null; then
    (cd "$REPO_ROOT/$1" && go vet ./... && go test -count=1 ./...)
  else
    docker run --rm -v "$REPO_ROOT:/repo:ro" -w "/repo/$1" -e GOFLAGS=-buildvcs=false \
      golang:1.27-alpine sh -c 'go vet ./... && go test -count=1 ./...'
  fi
}

run_unit() {
  log "unit: mock services"
  go_test mocks/customer-service || FAILED=1
  go_test mocks/loan-eligibility-soap || FAILED=1
  log "unit: MI artifact tests"
  go_test mi/tests || FAILED=1
  log "unit: build/deploy helper CLI"
  go_test tools || FAILED=1
}

mi_customer_backend() { compose exec -T mi printenv CUSTOMER_BACKEND_URL 2>/dev/null || true; }

run_integration() {
  local vars=() folders=(--folder accounts --folder customers --folder loans --folder "dashboard (bonus A)" --folder negative)
  if [[ "$TARGET" == apim ]]; then
    local envfile="$REPO_ROOT/tests/postman/apim.env.json"
    [[ -f "$envfile" ]] || "$REPO_ROOT/scripts/apim-demo-consumer.sh" >/dev/null
    local token apikey
    token="$(json_get 'values[key=accessToken].value' < "$envfile")"
    apikey="$(json_get 'values[key=apiKey].value' < "$envfile")"
    vars=(--env-var accountsUrl=https://apim:8243/jamii/accounts/v1
          --env-var customersUrl=https://apim:8243/jamii/customers/v1
          --env-var loansUrl=https://apim:8243/jamii/loans/v1
          --env-var "accessToken=$token" --env-var "apiKey=$apikey")
  else
    vars=(--env-var accountsUrl=http://mi:8290/accounts
          --env-var customersUrl=http://mi:8290/customers
          --env-var loansUrl=http://mi:8290/loans)
  fi
  if [[ "$(mi_customer_backend)" == *customer-mock* ]]; then
    folders+=(--folder mock-backend)
  else
    warn "MI uses the public customer backend; skipping the mock-backend folder"
  fi
  # Warm-up: the first call from a fresh MI to a public backend pays for DNS,
  # TLS and CDN cold paths (seen at ~4s), which is not what these tests measure.
  curl -s -o /dev/null -m 15 http://localhost:8290/customers/1 || true
  curl -s -o /dev/null -m 15 -H 'Content-Type: application/json' \
    -d '{"customerId":"1","monthlyIncome":1000,"requestedAmount":1000,"tenureMonths":1}' http://localhost:8290/loans/eligibility || true
  log "integration: newman against $TARGET"
  mkdir -p "$REPO_ROOT/target/test-reports"
  docker run --rm --network "$NETWORK" \
    -v "$REPO_ROOT/tests/postman:/etc/newman:ro" -v "$REPO_ROOT/target/test-reports:/reports" \
    postman/newman:6-alpine run familybank-assignment.json --insecure \
    "${vars[@]}" "${folders[@]}" --reporters cli,junit --reporter-junit-export "/reports/newman-$TARGET.xml" \
    || FAILED=1
}

# expect <description> <expected-status> <expected-code> <curl args...>
expect() {
  local what="$1" status="$2" code="$3"; shift 3
  local out got_status got_code
  out="$(curl -s -m 30 -w '\n%{http_code}' "$@")"
  got_status="${out##*$'\n'}"
  got_code="$(sed '$d' <<<"$out" | json_get error.code 2>/dev/null || echo '?')"
  if [[ "$got_status" =~ ^($status)$ && "$got_code" =~ ^($code)$ ]]; then
    log "PASS $what -> $got_status $got_code"
  else
    warn "FAIL $what -> got $got_status $got_code, expected $status $code"; FAILED=1
  fi
}

run_chaos() {
  local mi="http://localhost:8290"
  log "chaos: accounts database down"
  compose stop accounts-db >/dev/null 2>&1
  expect "balance with DB down" 503 ACCOUNT_DB_UNAVAILABLE "$mi/accounts/0100000001/balance"
  # Bonus A: the dashboard degrades to partial data instead of failing.
  local dash body
  dash="$(curl -s -m 30 -w '\n%{http_code}' "$mi/customers/1/dashboard")"
  body="$(sed '$d' <<<"$dash")"
  if [[ "${dash##*$'\n'}" == 200 && \
        "$(json_get partial <<<"$body" 2>/dev/null):$(json_get 'errors[0].code' <<<"$body" 2>/dev/null)" == "true:ACCOUNT_DB_UNAVAILABLE" ]]; then
    log "PASS dashboard with DB down -> 200 partial, accounts flagged ACCOUNT_DB_UNAVAILABLE"
  else
    warn "FAIL dashboard with DB down -> ${dash//$'\n'/ }"; FAILED=1
  fi
  compose start accounts-db >/dev/null 2>&1
  wait_for "accounts-db" 120 curl -sf "$mi/accounts/0100000001/balance" || { warn "DB did not recover"; FAILED=1; }

  if [[ "$(mi_customer_backend)" == *customer-mock* ]]; then
    log "chaos: customer backend down"
    compose stop customer-mock >/dev/null 2>&1
    expect "customer with backend down" '50[34]' 'CUSTOMER_BACKEND_UNAVAILABLE|CUSTOMER_BACKEND_TIMEOUT' "$mi/customers/1"
    compose start customer-mock >/dev/null 2>&1
  else
    warn "skipping customer backend chaos test (MI is using the public backend)"
  fi

  if [[ "$(compose exec -T mi printenv LOAN_SOAP_BACKEND_URL 2>/dev/null)" == *soap-mock* ]]; then
    log "chaos: SOAP backend down"
    compose stop soap-mock >/dev/null 2>&1
    expect "eligibility with SOAP down" '50[34]' 'LOAN_SERVICE_UNAVAILABLE|LOAN_SERVICE_TIMEOUT' \
      -H 'Content-Type: application/json' \
      -d '{"customerId":"1","monthlyIncome":120000,"requestedAmount":300000,"tenureMonths":12}' "$mi/loans/eligibility"
    compose start soap-mock >/dev/null 2>&1
  else
    warn "skipping SOAP backend chaos test (MI is using the public backend)"
  fi

  # MI suspends endpoints after connection failures (circuit breaker, up to
  # 30s); wait for it to close so later suites see healthy backends.
  wait_for "customer endpoint" 90 curl -sf "$mi/customers/1" || warn "customer endpoint still suspended"
  wait_for "loan endpoint" 90 curl -sf -H 'Content-Type: application/json' \
    -d '{"customerId":"1","monthlyIncome":120000,"requestedAmount":300000,"tenureMonths":12}' "$mi/loans/eligibility" \
    || warn "loan endpoint still suspended"
}

# --- Bonus B: loan-application events ---------------------------------------

EVENTS_DIR="$REPO_ROOT/tests/events"

# wait_message <queue> <correlation-id> <timeout-seconds>: consumes the queue
# until a message with that x-correlation-id arrives; prints it (JSON) or fails.
wait_message() {
  local queue="$1" correlation="$2" deadline=$((SECONDS + $3)) msg
  while (( SECONDS < deadline )); do
    while IFS= read -r msg; do
      [[ "$(json_get 'headers.x-correlation-id' <<<"$msg" 2>/dev/null)" == "$correlation" ]] && { echo "$msg"; return 0; }
    done < <(jamii rabbit get --queue "$queue" --count 50)
    sleep 1
  done
  return 1
}

# event_check <description> <condition-result> <detail>
event_check() {
  if [[ "$2" == ok ]]; then log "PASS $1"; else warn "FAIL $1 ($3)"; FAILED=1; fi
}

run_events() {
  log "events: purging the loan queues"
  for q in loan.applications loan.applications.retry loan.applications.dlq loan.decisions; do
    jamii rabbit purge --queue "$q" || { warn "RabbitMQ not reachable"; FAILED=1; return; }
  done
  local run="t$(date +%s)" msg payload

  "$REPO_ROOT/scripts/publish-loan-event.sh" "$EVENTS_DIR/valid-application.json" "$run-valid" 2>/dev/null
  if msg="$(wait_message loan.decisions "$run-valid" 30)"; then
    payload="$(json_get payload <<<"$msg")"
    event_check "valid application -> LoanEligibilityDecided on loan.decisions" \
      "$([[ "$(json_get eventType <<<"$payload"):$(json_get applicationId <<<"$payload"):$(json_get decision <<<"$payload"):$(json_get loan.monthlyInstallment <<<"$payload")" == "LoanEligibilityDecided:APP-1001:NOT_ELIGIBLE:41667" ]] && echo ok)" "$payload"
  else
    event_check "valid application -> LoanEligibilityDecided on loan.decisions" fail "no decision within 30s"
  fi

  local expected
  for expected in "malformed.txt:MALFORMED_EVENT" "schema-invalid.json:INVALID_EVENT" "rejected-by-api.json:LOAN_REQUEST_REJECTED"; do
    local file="${expected%%:*}" code="${expected##*:}"
    "$REPO_ROOT/scripts/publish-loan-event.sh" "$EVENTS_DIR/$file" "$run-${file%%.*}" 2>/dev/null
    if msg="$(wait_message loan.applications.dlq "$run-${file%%.*}" 30)"; then
      event_check "$file -> DLQ with x-error-code $code, original body kept, no retries" \
        "$([[ "$(json_get 'headers.x-error-code' <<<"$msg")" == "$code" && "$(json_get payload <<<"$msg")" == "$(cat "$EVENTS_DIR/$file" | tr -d '\n')" ]] && ! json_get 'headers.x-death' <<<"$msg" >/dev/null 2>&1 && echo ok)" "$msg"
    else
      event_check "$file -> DLQ ($code)" fail "not dead-lettered within 30s"
    fi
  done

  if [[ "$(compose exec -T mi printenv LOAN_SOAP_BACKEND_URL 2>/dev/null)" != *soap-mock* ]]; then
    warn "skipping retry tests (MI is using the public SOAP backend)"
    return
  fi

  log "events: SOAP backend down for the whole retry window"
  compose stop soap-mock >/dev/null 2>&1
  "$REPO_ROOT/scripts/publish-loan-event.sh" "$EVENTS_DIR/eligible-application.json" "$run-exhausted" 2>/dev/null
  if msg="$(wait_message loan.applications.dlq "$run-exhausted" 60)"; then
    event_check "transient failure -> 3 retries, then parked in DLQ by MI" \
      "$([[ "$(json_get 'headers.x-death[0].count' <<<"$msg")" == 3 ]] && echo ok)" "x-death count $(json_get 'headers.x-death[0].count' <<<"$msg")"
  else
    event_check "transient failure -> parked in DLQ" fail "not parked within 60s"
  fi

  log "events: SOAP backend recovers during the retry window"
  "$REPO_ROOT/scripts/publish-loan-event.sh" "$EVENTS_DIR/eligible-application.json" "$run-recovered" 2>/dev/null
  sleep 3
  compose start soap-mock >/dev/null 2>&1
  if msg="$(wait_message loan.decisions "$run-recovered" 45)"; then
    event_check "transient failure then recovery -> decision published on a retry" \
      "$([[ "$(json_get payload <<<"$msg" | json_get decision)" == ELIGIBLE ]] && echo ok)" "$msg"
  else
    event_check "transient failure then recovery -> decision" fail "no decision within 45s"
  fi
}

$UNIT && run_unit
$INTEGRATION && run_integration
$CHAOS && run_chaos
$EVENTS && run_events
(( FAILED == 0 )) || die "tests failed"
log "all requested tests passed"
