# Accounts database runtime

The compose file uses the official `mysql:8.4` image directly, with no
custom image. The schema and seed scripts in `database/accounts/` are
mounted into `/docker-entrypoint-initdb.d/` and run on first start.

- Host port: `3307` by default (`ACCOUNTS_DB_HOST_PORT`), to avoid clashing
  with a MySQL already running on the developer's machine.
- Data lives in the `accounts-db-data` volume; `./scripts/setup-db.sh --reset`
  recreates it from the scripts.
- MI connects as `ACCOUNTS_DB_USER` through `ACCOUNTS_DB_URL`, injected as
  environment variables into the MI container.
