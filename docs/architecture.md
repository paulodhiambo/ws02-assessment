# Architecture

## Overview

```
                        ┌────────────────────────── WSO2 API Manager 4.6 ─────────────────────────┐
  Consumer app ──HTTPS──▶  Gateway :8243                                                           │
  (OAuth2 token          │   • authN: OAuth2 (accounts, customers) / API key (loans)               │
   or API key)           │   • throttling: subscription tier + API-level limit                     │
                         │   • policies: jamiiCorrelation (req), jamiiSecurityHeaders (resp/fault)  │
                         │  Publisher / Developer Portal :9443   (apictl imports from apim/)       │
                         └───────────────────────────────┬─────────────────────────────────────────┘
                                                         │ http://mi:8290
                        ┌────────────────────── WSO2 Micro Integrator 4.6 ────────────────────────┐
                        │  common: correlation · request/response logging · masking · common-error│
                        │                                                                         │
                        │  AccountBalanceAPI ──dataServiceCall──▶ AccountsDataService ──JDBC──────┼──▶ MySQL 8.4
                        │  CustomerAPI ──────── CustomerBackendEP (5s, circuit breaker) ──HTTPS───┼──▶ JSONPlaceholder /users/{id}
                        │  LoanEligibilityAPI ─ CalculatorDivideTemplate ─ LoanSoapEP (10s) ─SOAP─┼──▶ DNE Online calculator.asmx
                        └─────────────────────────────────────────────────────────────────────────┘
                                   (local mocks with the same contracts replace both public backends in CI)
```

Everything runs from `docker-compose.yml`. MI starts with no integrations
deployed; the CARs are deployed into the running container by
`scripts/deploy-mi.sh`, the same way they would be deployed to a long-lived
server.

## Responsibilities by layer

| Concern                       | API Manager gateway                     | Micro Integrator                                   |
|-------------------------------|-----------------------------------------|----------------------------------------------------|
| Authentication / authorisation| OAuth2, API key, subscriptions          | none (trusts the gateway; network-restricted in prod) |
| Rate limiting                 | Subscription tiers and API-level limits | none                                               |
| Correlation ID                | Generates one if the caller sent none   | Validates or generates, propagates, logs, echoes   |
| Input validation              | none (passthrough)                      | Path regexes, JSON Schema, Content-Type            |
| Backend protocol / mapping    | none                                    | JDBC data service, REST proxy, JSON to SOAP to JSON |
| Timeouts / circuit breaking   | none                                    | Per-endpoint timeout and suspension                |
| Error shape                   | Gateway errors (401/403/429)            | One envelope for every integration error           |
| Response headers              | HSTS, nosniff, no-store                 | Strips every backend header                        |

Keeping validation and error mapping in MI means the integrations behave the
same whether they are called through the gateway or directly (CI calls MI
directly first, then repeats the same tests through the gateway).

## Shared building blocks (mi/common)

- **correlation**: `REQUEST_ID` (new per request) and `CORRELATION_ID`
  (the caller's `X-Correlation-ID` if it matches `[A-Za-z0-9._-]{8,64}`,
  otherwise generated). The format check stops log or header injection
  through a caller-controlled value.
- **request-logging / response-logging**: one structured log line per
  request in and per response out, with API, masked subject, status, error
  code, duration, request and correlation IDs. Each API also logs
  `BACKEND_CALL` and `BACKEND_RESPONSE` with latency.
- **mask-sensitive-data**: keeps the last 4 characters (`******0001`).
  Payloads are never logged; a unit test fails the build if any `log`
  mediator uses anything other than `level="custom"`.
- **common-error**: the only place an error body is built. Every fault
  sequence must delegate to it (also enforced by a unit test), so the three
  APIs cannot drift apart.

## Key decisions and trade-offs

**Data service called in-process, not over HTTP.** `AccountBalanceAPI` uses
the `dataServiceCall` mediator, so there is no second HTTP hop or second
public endpoint to secure. A failure inside the data service call is
unambiguously a database failure (`STAGE=DB_CALL`), which gives a clean 503
`ACCOUNT_DB_UNAVAILABLE` that is distinct from 404. The trade-off is that
the data service is also reachable as a SOAP service on MI's own port; in
prod MI is not exposed outside the gateway network.

**Pool settings for a database that may be down.** `initialSize=0`,
`testOnBorrow` and a 3s `connectTimeout` mean MI deploys even if MySQL is
down, fails fast while it is down, and recovers without a redeploy once it
is back. This was verified by stopping the container mid-run.

**Configuration from the environment.** `$SYSTEM:` placeholders (data
service URL and credentials, SOAP endpoint) and `get-property('env', ...)`
(customer base URL) mean the same CAR runs in dev and prod. A missing
variable fails deployment, and the deploy script then rolls back.

**Endpoint resilience.** Customer endpoint: 5s timeout. Loan endpoint: 10s
timeout, because the public service is slower. Connection failures suspend
the endpoint for 2s, doubling up to 30s, so a dead backend costs
milliseconds instead of a socket timeout per request. MI's timeout sweeper
interval was lowered from 15s to 1s, otherwise a 5s timeout fires after
about 15s; that was measured before and after the change.

**SOAP via a call template.** The SOAP envelope, `SOAPAction` and header
scrubbing live in `CalculatorDivideTemplate`, so adding another operation
(e.g. `Multiply` for interest) reuses the endpoint and the hygiene rules.

**Custom CAR packaging.** `mi/build/package_car.py`, run from Maven, writes
the standard CAR layout (`artifacts.xml` plus one folder per artifact). It
avoids depending on the WSO2 Maven repository and Integration Studio project
metadata, and keeps the folder layout simple. The trade-off is that it is a
small piece of custom tooling to maintain. The generated CARs are
hot-deployed by the stock MI runtime with no warnings.

**All-or-nothing deployments.** MI: CARs are staged inside the container,
swapped in with `mv` (an atomic rename, so the hot deployer never reads a
half-copied file), and verified through the management API; any faulty or
missing app restores the previous set. APIM: existing APIs are exported
first, then each import is checked, and a failure re-imports the backups (or
deletes APIs that were new). Across the two tiers, the pipeline restores the
previous MI CARs if the APIM step fails. Both rollback paths were tested
with deliberately broken artifacts.

## What changes in production

- MI and APIM clustered behind a load balancer, with an external APIM
  database and changed default credentials and keystores.
- MI reachable only from the gateway network; mutual TLS between gateway
  and MI.
- Secrets from a vault (MI secure vault or the platform's secret store)
  instead of environment variables.
- Logs shipped as JSON to the bank's log platform; MI and APIM metrics to
  Prometheus.
