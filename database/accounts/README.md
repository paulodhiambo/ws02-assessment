# Accounts database

Backs `GET /accounts/{accountNumber}/balance` (Part 1a).

| File         | Purpose                                         |
|--------------|-------------------------------------------------|
| `schema.sql` | `customers` and `accounts` tables (MySQL 8)     |
| `seed.sql`   | Fictional customers and six accounts covering every status |

## How it is loaded

`docker-compose.yml` mounts both files into `/docker-entrypoint-initdb.d/`, so
MySQL runs them once, the first time the `accounts-db` volume is created. To
reload from scratch:

```bash
./scripts/setup-db.sh --reset
```

## Seed accounts

| Account      | Status  | Balance          | Use in tests                 |
|--------------|---------|------------------|------------------------------|
| `0100000001` | ACTIVE  | 152,340.75 KES   | Happy path                   |
| `0100000003` | DORMANT | 1,200.50 KES     | Non-active status            |
| `0100000004` | CLOSED  | 0.00 KES         | Closed account               |
| `0100000006` | ACTIVE  | 310.40 USD       | Non-KES currency             |
| `0100000099` | —       | —                | Not present: returns 404     |

## Design notes

- `account_number` is a fixed 10-digit string (`CHAR(10)`), not a number, so
  leading zeros survive. The API rejects anything else with `400` before
  touching the database.
- `status` and the number format are enforced with `CHECK` constraints so bad
  data cannot reach the API.
- MI only ever reads through the `AccountsDataService` data service, which uses
  a bound parameter (`WHERE account_number = ?`), never string concatenation.
- The MI database user only needs `SELECT` on `accounts`.
