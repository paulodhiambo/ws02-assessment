# Jamii Savings – WSO2 integration assignment

Three integrations for the fictional **Jamii Savings** bank, built on **WSO2
Micro Integrator 4.6**, exposed through **WSO2 API Manager 4.6**, and built,
tested and deployed by one **Jenkins** pipeline against a docker-compose
environment.

| Part | API | Where |
|------|-----|-------|
| 1a | `GET /accounts/{accountNumber}/balance`: MySQL through an MI data service | `mi/account-balance`, `database/accounts` |
| 1b | `GET /customers/{customerId}`: managed proxy for JSONPlaceholder | `mi/customer-proxy` |
| 1c | `POST /loans/eligibility`: REST/JSON over the DNE Online SOAP calculator | `mi/loan-eligibility` |
| shared | correlation IDs, logging, masking, the common error envelope | `mi/common` |
| 2 | apictl projects, OpenAPI definitions, custom policies, API Product | `apim/` |
| 3 | `Jenkinsfile` + `scripts/` (build, test, all-or-nothing deploy) | root |

More detail: [architecture](docs/architecture.md) · [API design, mappings and error catalogue](docs/api-design.md) · [build and deploy](docs/deployment.md) · [demo script](docs/demo-script.md)

## Run it locally

Needs Docker (about 3 GB of RAM for APIM), JDK 17+, Maven, Python 3.

```bash
scripts/build.sh                                       # 4 CARs + APIM packages; runs 28 MI artifact tests
docker compose --profile apim up -d --build --wait     # MySQL, mocks, MI, APIM (drop --profile apim for MI only)
scripts/deploy-mi.sh dev                               # MI APIs on http://localhost:8290
scripts/install-apictl.sh                              # apictl 4.6.4 into .tools/ (scripts find it there)
scripts/deploy-apim.sh dev                             # 3 APIs + product on https://localhost:8243/jamii/...
eval "$(python3 scripts/apim-demo-consumer.py | grep '^export ')"   # Dev Portal app, subscriptions, OAuth2 token, API key
curl -sk "$GW/accounts/v1/0100000001/balance" -H "Authorization: Bearer $TOKEN"
scripts/test.sh --all                                  # unit + Postman suite against MI + outage tests
scripts/cleanup.sh --all
```

Ready-made requests: `tests/integration/*.http`, `tests/negative/*.http`, `tests/postman/`.

## Architecture decisions and trade-offs

- **MI owns integration behaviour; APIM owns access.** MI does validation,
  protocol mediation, timeouts and error mapping, so the APIs behave the
  same with or without the gateway (CI tests both). APIM does OAuth2 and API
  keys, throttling, the Developer Portal and two custom gateway policies
  (correlation and consumer headers; security response headers).
- **One error envelope** (`error.code`, `message`, `status`, `requestId`,
  `correlationId`, `timestamp`), built in exactly one sequence. Build-time
  tests fail if a fault sequence builds its own payload or any log mediator
  could log a payload.
- **Database:** an in-process `dataServiceCall` with a bound parameter. A
  DB failure gives 503 `ACCOUNT_DB_UNAVAILABLE`, distinct from 404. The pool
  recovers by itself when the DB returns (tested by stopping MySQL).
- **Customer backend: JSONPlaceholder.** It is free and stable, returns a
  realistic PII-bearing profile and a real 404, and sends plenty of CDN
  headers that the proxy strips. Value added on top of pass-through:
  injected `X-Correlation-ID`, stripped credential and internal headers in
  both directions, `no-store`, a 5s timeout with circuit breaking, and
  502/503/504 mapping.
- **SOAP backend: DNE Online calculator.** No public loan SOAP service
  exists, so the SOAP call computes the installment
  (`Divide(requestedAmount, tenureMonths)`) and MI applies a 40%
  debt-to-income rule. The service raises real .NET SOAP faults with stack
  traces; they are logged and mapped (`soap:Client` → 422,
  `soap:Server` → 502) and never returned.
- **Contract-compatible local mocks** of both public backends make CI
  deterministic and let tests trigger 500s, timeouts and faults. Switching
  between mock and public backends is an environment variable.
- **Security and throttling:** OAuth2 for accounts and customers (financial
  data and PII); an API key for loan eligibility (no PII, server-to-server
  callers). **Loan eligibility uses Bronze and 10KPerMin instead of
  Gold/Silver and 50KPerMin, because each call costs a round trip to an
  external, rate-limited SOAP service.**
- **API Product** `JamiiCoreBankingProduct` bundles all three operations,
  so first-party channels get one subscription. The APIs stay individually
  subscribable.
- **Packaging:** a small Python packager, run by `mvn clean install`, writes
  standard CARs from a simple folder layout instead of depending on the
  WSO2 Maven plugins and Integration Studio metadata. The trade-off is one
  piece of custom tooling.
- **Deployments are all-or-nothing.** MI CARs are swapped in atomically,
  verified through the management API, and rolled back on any faulty app.
  APIM APIs are backed up, imported, and restored if any import fails. If
  APIM fails after MI succeeded, the pipeline puts MI back too. Both
  rollbacks were tested with deliberately broken artifacts.

## Assumptions

- Fictional bank, fictional data. KES unless the account says otherwise;
  amounts are `DECIMAL(18,2)`; account numbers are exactly 10 digits.
- Eligibility is indicative: principal only, no interest or credit history.
  The SOAP service is Int32, so the installment is in whole shillings
  (half-to-even rounding by the service).
- MI is reachable only from the gateway network in a real deployment, so MI
  itself does no client authentication.
- Only dev is wired up. Prod is modelled (`infrastructure/config/prod.env.example`,
  an approval gate, promotion of the same archived artifacts) and runs as a
  dry run.
- Bonus tasks (aggregation API, message queue) were not attempted.

## With more time

- Synapse unit tests on MI's unit-test server, plus contract tests that
  check the MI responses against the OpenAPI definitions.
- Real interest in the eligibility rule (a second SOAP call to `Multiply`
  through the same template), and caching the decision per request hash.
- Secrets from a vault; mutual TLS from the gateway to MI; JSON logs shipped
  to a central platform; Prometheus metrics and alerts on the 5xx codes.
- A real prod target (Kubernetes with Helm, MI image per release), and
  per-API selective deploys driven by changed paths.
- The aggregation bonus: parallel clone/aggregate over accounts and
  customers, returning partial data with the failed section flagged.
