#!/usr/bin/env bash
# Starts the accounts database and checks the seed data is there.
#
#   scripts/setup-db.sh            start (first start runs schema.sql + seed.sql)
#   scripts/setup-db.sh --reset    drop the data volume and reload from scratch
set -euo pipefail
SCRIPT_NAME=setup-db
source "$(dirname "$0")/lib/common.sh"
require docker

if [[ "${1:-}" == "--reset" ]]; then
  warn "dropping the accounts database volume"
  compose rm -sfv accounts-db >/dev/null
  docker volume rm -f jamii_accounts-db-data >/dev/null
fi

compose up -d accounts-db >/dev/null
wait_for "accounts-db" 120 compose exec -T accounts-db sh -c 'mysqladmin ping -h 127.0.0.1 -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent' \
  || die "accounts-db did not become healthy"

# The init scripts run asynchronously on first start; wait for the seed rows.
count() {
  compose exec -T accounts-db sh -c 'mysql -N -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -e "SELECT COUNT(*) FROM accounts" 2>/dev/null'
}
for _ in $(seq 1 20); do
  rows="$(count || true)"
  [[ "${rows:-0}" -gt 0 ]] && break
  sleep 3
done
[[ "${rows:-0}" -gt 0 ]] || die "accounts table is empty or missing"
log "accounts rows: $rows"
compose exec -T accounts-db sh -c 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -e "SELECT account_number, status, balance, currency FROM accounts" 2>/dev/null'
