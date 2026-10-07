#!/usr/bin/env bash
# Runs the test suites.
#
#   scripts/test.sh                         unit tests (default)
#   scripts/test.sh --integration           Postman collection via newman, direct to MI
#   scripts/test.sh --integration --apim    same collection through the APIM gateway
#   scripts/test.sh --chaos                 stops the DB/backends and checks the error mapping
#   scripts/test.sh --all                   unit + integration (MI) + chaos
#
# Unit:        mock services (node --test) and MI artifact tests (python).
# Integration: needs the compose stack with CARs deployed (and APIs imported for --apim).
#              newman runs in Docker on the compose network, so no local Node/newman is needed.
#              The "mock-backend" folder runs only when MI points at the local customer mock.
set -euo pipefail
SCRIPT_NAME=test
source "$(dirname "$0")/lib/common.sh"

UNIT=false; INTEGRATION=false; CHAOS=false; TARGET=mi
(( $# )) || UNIT=true
for arg in "$@"; do
  case "$arg" in
    --unit) UNIT=true ;;
    --integration) INTEGRATION=true ;;
    --chaos) CHAOS=true ;;
    --apim) TARGET=apim ;;
    --all) UNIT=true; INTEGRATION=true; CHAOS=true ;;
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    *) die "unknown argument: $arg" ;;
  esac
done

NETWORK="${COMPOSE_NETWORK:-jamii_default}"
FAILED=0

run_unit() {
  log "unit: mock services"
  for mock in customer-service loan-eligibility-soap; do
    if command -v node >/dev/null; then
      (cd "$REPO_ROOT/mocks/$mock" && node --test src/) || FAILED=1
    else
      docker run --rm -v "$REPO_ROOT/mocks/$mock:/app:ro" -w /app node:22-alpine node --test src/ || FAILED=1
    fi
  done
  log "unit: MI artifact tests"
  python3 "$REPO_ROOT/mi/tests/run_tests.py" || FAILED=1
}

mi_customer_backend() { compose exec -T mi printenv CUSTOMER_BACKEND_URL 2>/dev/null || true; }

run_integration() {
  local vars=() folders=(--folder accounts --folder customers --folder loans --folder negative)
  if [[ "$TARGET" == apim ]]; then
    local envfile="$REPO_ROOT/tests/postman/apim.env.json"
    [[ -f "$envfile" ]] || python3 "$REPO_ROOT/scripts/apim-demo-consumer.py" >/dev/null
    local token apikey
    token="$(json_get "[v['value'] for v in d['values'] if v['key']=='accessToken'][0]" < "$envfile")"
    apikey="$(json_get "[v['value'] for v in d['values'] if v['key']=='apiKey'][0]" < "$envfile")"
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
  got_code="$(sed '$d' <<<"$out" | json_get "d['error']['code']" 2>/dev/null || echo '?')"
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

$UNIT && run_unit
$INTEGRATION && run_integration
$CHAOS && run_chaos
(( FAILED == 0 )) || die "tests failed"
log "all requested tests passed"
