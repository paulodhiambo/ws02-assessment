# Jamii Savings – WSO2 Integration Assignment

This project implements three integrations for the fictional **Jamii Savings** bank using **WSO2 Micro Integrator 4.6**. The APIs are exposed and managed through **WSO2 API Manager 4.6**, with the build, testing, and deployment process automated through Jenkins.

Everything can be run locally using Docker Compose, including the database, mock backend services, Micro Integrator, and API Manager.

| Part    | API / Component                                                            | Location                                  |
| ------- | -------------------------------------------------------------------------- | ----------------------------------------- |
| 1a      | `GET /accounts/{accountNumber}/balance` – MySQL-backed account lookup      | `mi/account-balance`, `database/accounts` |
| 1b      | `GET /customers/{customerId}` – managed REST proxy                         | `mi/customer-proxy`                       |
| 1c      | `POST /loans/eligibility` – REST/JSON to SOAP integration                  | `mi/loan-eligibility`                     |
| Bonus A | `GET /customers/{customerId}/dashboard` – parallel aggregation             | `mi/customer-proxy`                       |
| Bonus B | RabbitMQ loan-application events – consume, call 1c, publish, dead-letter  | `mi/loan-events`, `infrastructure/docker/rabbitmq` |
| Shared  | Correlation IDs, logging, sensitive-data masking and common error handling | `mi/common`                               |
| Part 2  | OpenAPI definitions, API Manager projects, policies and API Product        | `apim/`                                   |
| Part 3  | Jenkins pipeline and deployment scripts                                    | `Jenkinsfile`, `scripts/`                 |
| CI      | GitHub Actions workflow using the same build and test scripts              | `.github/`                                |

Additional documentation is available in:

* [Architecture](docs/architecture.md)
* [API design, mappings and error catalogue](docs/api-design.md)
* [Build and deployment](docs/deployment.md)
* [Demo script](docs/demo-script.md)

## Running locally

### Prerequisites

The following are required:

* Docker and Docker Compose
* JDK 17 or later
* Maven
* Go 1.24 or later

The Go installation is optional for most users because the build and test scripts can fall back to Docker where necessary.

WSO2 API Manager requires approximately 3 GB of memory in this setup.

### Build and start the environment

From the repository root:

```bash
scripts/build.sh
```

This builds the Micro Integrator CAR files, packages the API Manager projects and runs the MI artifact tests.

Start the complete environment:

```bash
docker compose --profile apim up -d --build --wait
```

If API Manager is not required, the profile can be omitted:

```bash
docker compose up -d --build --wait
```

Deploy the Micro Integrator applications:

```bash
scripts/deploy-mi.sh dev
```

The MI APIs are available locally through:

```text
http://localhost:8290
```

Install the API Controller CLI used by the deployment scripts:

```bash
scripts/install-apictl.sh
```

This installs `apictl` 4.6.4 under `.tools/`.

Deploy the APIs and API Product to API Manager:

```bash
scripts/deploy-apim.sh dev
```

The gateway is then available under:

```text
https://localhost:8243/jamii/
```

The repository also contains a helper for setting up a demo consumer application, subscriptions and credentials:

```bash
eval "$(scripts/apim-demo-consumer.sh | grep '^export ')"
```

A sample request can then be made with:

```bash
curl -sk \
  "$GW/accounts/v1/0100000001/balance" \
  -H "Authorization: Bearer $TOKEN"
```

### Running the tests

The complete test suite can be run with:

```bash
scripts/test.sh --all
```

This covers the MI tests, API-level tests, the failure scenarios for unavailable downstream services and the Bonus B event flows (`scripts/test.sh --events`).

Example requests are also provided under:

```text
tests/integration/
tests/negative/
tests/postman/
```

To remove the environment:

```bash
scripts/cleanup.sh --all
```

---

## Architecture and design decisions

### Micro Integrator handles integration; API Manager handles API access

The responsibilities are deliberately separated.

**WSO2 Micro Integrator** is responsible for integration logic such as request validation, database access, protocol transformation, downstream timeouts and error mapping.

**WSO2 API Manager** handles the API-facing concerns: authentication, throttling, subscriptions, the Developer Portal and gateway-level policies.

This also means the MI APIs can be tested directly without going through the API gateway. The CI pipeline tests both paths.

### Consistent error handling

All three APIs use the same error structure:

```json
{
  "error": {
    "code": "ACCOUNT_DB_UNAVAILABLE",
    "message": "The accounts database is currently unavailable. Please retry later.",
    "status": 503
  },
  "requestId": "2fe466e6-009d-4539-ae67-228d4965fc2d",
  "correlationId": "demo-corr-0001",
  "timestamp": "2026-10-07T16:29:45.072Z"
}
```

The response is generated through a shared error sequence rather than being independently implemented by each API.

The error codes are:

| API | Codes |
| --- | ----- |
| Account balance | `INVALID_ACCOUNT_NUMBER` (400), `ACCOUNT_NOT_FOUND` (404), `ACCOUNT_DB_UNAVAILABLE` (503) |
| Customer | `INVALID_CUSTOMER_ID` (400), `CUSTOMER_NOT_FOUND` (404), `CUSTOMER_BACKEND_ERROR` (502), `CUSTOMER_BACKEND_UNAVAILABLE` (503), `CUSTOMER_BACKEND_TIMEOUT` (504) |
| Loan eligibility | `INVALID_REQUEST` (400), `UNSUPPORTED_MEDIA_TYPE` (415), `LOAN_REQUEST_REJECTED` (422), `LOAN_SERVICE_ERROR` (502), `LOAN_SERVICE_UNAVAILABLE` (503), `LOAN_SERVICE_TIMEOUT` (504) |
| Dashboard (Bonus A) | `DASHBOARD_UNAVAILABLE` (503), plus the customer codes above |
| All | `INTERNAL_ERROR` (500) |

The full catalogue, with when each code is returned, is in [API design](docs/api-design.md#error-catalogue).

The implementation also deliberately avoids logging complete account numbers, customer information, credentials or tokens.

### Account balance API

The account balance API uses MySQL through an MI data service.

The account number is passed as a bound parameter rather than being concatenated into SQL. An unknown account produces a `404`, while a database connectivity or availability problem produces a separate `503` response.

The database connection pool is allowed to recover automatically when the database becomes available again. This behaviour is covered by the failure tests.

### Customer API

The customer endpoint is implemented as a managed proxy over **JSONPlaceholder**.

JSONPlaceholder was chosen because it is publicly available, simple to work with and provides a realistic REST response suitable for demonstrating mediation.

The proxy adds value beyond simply forwarding the request. It:

* Adds an `X-Correlation-ID` when one is not supplied.
* Removes internal or credential-related headers.
* Applies a five-second downstream timeout.
* Prevents caching of the response.
* Maps downstream failures to the common error format.

For testing, the repository also includes a local implementation with the same contract. This makes CI predictable and allows the tests to deliberately simulate `500`, timeout and unavailable-backend scenarios.

The backend can be selected through configuration, so the local mock can be used during development while the public service can be used for demonstration.

### Loan eligibility API

The loan eligibility endpoint accepts JSON and communicates with the **DNE Online calculator** through SOAP.

The flow is:

```text
REST/JSON
    ↓
WSO2 Micro Integrator
    ↓
SOAP request
    ↓
DNE Online service
    ↓
SOAP response
    ↓
JSON response
```

There is no suitable public loan-eligibility SOAP service that directly provides the required behaviour, so the SOAP service is used to perform the underlying installment calculation. MI then applies a 40% debt-to-income rule to determine eligibility. The 40% threshold is an assumption of this implementation (a common lending rule of thumb), not something the assignment specifies.

SOAP faults are handled inside MI and converted to the common REST error format. The original SOAP fault XML, including any server-side details or stack traces, is never returned to the API consumer.

A client-side SOAP fault is mapped to `422`, while a server-side SOAP fault is mapped to `502`.

### Local mocks

Both external services have contract-compatible local mocks.

This is intentional rather than simply being a convenience for development. Depending on a public service during CI introduces an unnecessary external dependency and makes it difficult to reliably test failure scenarios.

The local services can simulate:

* Successful responses
* `404` responses
* `500` responses
* Timeouts
* SOAP faults

The same MI configuration can switch between the local mocks and the public services using environment configuration.

---

## Security and throttling

OAuth2 is used for the account and customer APIs because they expose financial information and customer data.

The loan eligibility API uses an API key because the intended use case is a controlled server-to-server integration and the API does not expose customer-identifying information in its response.

The APIs also use different throttling tiers.

The loan eligibility API uses the lower `Bronze` / `10KPerMin` tier, while the other APIs use higher tiers. The reason is that every loan eligibility request results in an external SOAP call, making the backend more expensive and potentially subject to the limits of a third-party service.

API Manager also applies custom gateway mediation policies for correlation and consumer headers, as well as security-related response headers.

---

## API Product

The three APIs are grouped into:

```text
JamiiCoreBankingProduct
```

The product gives consuming applications a single subscription path while keeping the underlying APIs independently manageable.

A typical consumer workflow is:

```text
Developer
    ↓
Developer Portal
    ↓
Create application
    ↓
Subscribe to JamiiCoreBankingProduct
    ↓
Obtain credentials
    ↓
Invoke the individual APIs
```

---

## Deployment

The repository uses a single Jenkins pipeline for all three integrations.

The pipeline performs the following stages:

```text
Build & package          MI CARs (mvn clean install) and API Manager project archives
   ↓
Unit tests               MI artifact tests, mock and tooling tests (Go)
   ↓
Archive artifacts        the exact CARs and archives that are deployed and promoted
   ↓
Dev: environment         docker compose: MySQL, mocks, RabbitMQ, MI, API Manager
   ↓
Dev: deploy              MI, then API Manager, all-or-nothing
   ↓
Dev: integration tests   Postman suite against MI and through the gateway,
                         outage tests and Bonus B event tests
   ↓
Prod: approval           manual gate (only when PROMOTE_TO_PROD is selected)
   ↓
Prod: deploy             promotes the same archived artifacts (dry run here)
```

The same stages run on GitHub Actions (`.github/workflows/ci.yml`), using the same scripts.

Only the development environment is wired to a running deployment target. Production is modelled through the configuration and pipeline stages but is not connected to a live production environment.

The deployment process is designed to fail rather than silently leave the environment partially updated.

For MI, the CAR files are deployed and verified through the management API. If a deployment fails, the previous version is restored.

For API Manager, existing API definitions are backed up before deployment. If an import fails, the previous state is restored.

If API Manager deployment fails after the MI deployment has already succeeded, the pipeline also rolls the MI deployment back.

These rollback paths have been tested using deliberately invalid artifacts.

---

## Bonus A – Customer dashboard

The optional aggregation API has also been implemented:

```http
GET /customers/{customerId}/dashboard
```

The API retrieves the customer profile and account information in parallel using MI's Scatter-Gather pattern.

A successful response contains:

```text
customerId
customer
accounts
partial
errors
requestId
timestamp
```

### Partial-response decision

The dashboard is intended to support a read-only customer-facing view, so partial results are preferable to failing the entire request when one backend is temporarily unavailable.

For example, if the customer service is unavailable but the account database is healthy:

```json
{
  "customerId": "1",
  "partial": true,
  "customer": null,
  "accounts": [...],
  "errors": [
    {
      "section": "customer",
      "code": "CUSTOMER_BACKEND_UNAVAILABLE",
      "message": "The customer service did not respond in time."
    }
  ],
  "requestId": "752f7f5b-a482-4247-a93a-e7f5903590e4",
  "timestamp": "2026-10-07T17:22:13.873Z"
}
```

The parallel calls share a seven-second deadline, so one slow backend cannot hold the entire response.

The API returns `404` when the customer does not exist. If both backend calls fail and there is no useful information to return, the API responds with:

```text
503 DASHBOARD_UNAVAILABLE
```

The complete response mapping is documented in [API Design](docs/api-design.md#bonus-a--customer-dashboard).

---

## Bonus B – Loan-application events (RabbitMQ)

MI consumes `LoanApplicationSubmitted` events from RabbitMQ, transforms each one into a request to the Part 1c API (`POST /loans/eligibility`) and publishes the result as a `LoanEligibilityDecided` event.

```text
loan.applications ──▶ MI inbound endpoint ──▶ POST /loans/eligibility ──▶ loan.decisions
        ▲   │                                    (Part 1c, unchanged)
        │   └── transient failure: reject ──▶ loan.applications.retry (5 s TTL) ──┐
        └──────────────────────────────────────────────────────────────────────┘
                     permanent failure, or 3 retries used up ──▶ loan.applications.dlq
```

The broker topology (exchange, retry queue, dead-letter queue) is defined in `infrastructure/docker/rabbitmq/definitions.json`, and RabbitMQ starts with the rest of the environment.

```bash
scripts/publish-loan-event.sh tests/events/valid-application.json demo-001
.tools/bin/jamii rabbit get --queue loan.decisions
```

The management UI is at `http://localhost:15672` (`jamii` / `jamii-dev-only`).

### Malformed and unprocessable messages

Nothing is silently dropped. Failures are split into two kinds:

| Failure | Example | Handling |
| ------- | ------- | -------- |
| Permanent | body is not JSON, fails the event schema, or the API answers `4xx` (e.g. `422 LOAN_REQUEST_REJECTED`) | published straight to `loan.applications.dlq` with the **original body unchanged** and `x-error-code`, `x-error-reason` and `x-correlation-id` headers. Retrying cannot help, so it is not retried. |
| Transient | the API answers `5xx`, times out, or the SOAP backend is down | the message is rejected and the broker routes it through the 5-second retry queue. After **3 retries** MI parks it in the same DLQ, and the broker's `x-death` headers record every attempt. |

The consumer acknowledges a message only after it has been fully handled: decided, parked, or handed back to the broker for retry. If MI stops mid-message, RabbitMQ redelivers it.

### How this differs from the request/response APIs

| | Request/response APIs (Part 1) | Event consumer (Bonus B) |
| --- | --- | --- |
| Who sees an error | The caller, immediately, as an HTTP status in the common error format | Nobody is waiting. The outcome is a message on `loan.decisions` or `loan.applications.dlq`. |
| Retries | The client's decision. MI never retries, which avoids duplicate side effects and keeps latency predictable. | MI and the broker own retries: a fixed back-off (5 s), bounded at 3, then parked. |
| Bad input | `400`/`415`/`422`; the client corrects it and resends | The message is moved to the DLQ with the reason, for an operator or the producer to fix and republish. |
| Backend outage | Fail fast (`503`/`504`, with circuit breaking) | Absorbed: the message waits in the queue and retry loop, and succeeds if the backend recovers within the retry window. |
| Tracing | `X-Correlation-ID` header | the `x-correlation-id` AMQP header, propagated to the API call and to the decision or DLQ message |

Reusing the Part 1c API keeps one set of validation and SOAP-fault rules for both entry points.

---

## What I would do differently with more time

* **Testing:** add Synapse unit tests on MI's unit-test server, and contract tests that check the live MI responses against the OpenAPI definitions.
* **Eligibility:** include interest in the eligibility calculation (a second SOAP operation through the existing call template), and cache decisions for identical requests.
* **Security:** load secrets from a vault rather than environment variables, and use mutual TLS between the API gateway and MI.
* **Observability:** ship JSON logs to a central platform and export MI and API Manager metrics to Prometheus, with alerts on the 5xx error codes and on DLQ depth.
* **Production:** wire up a real production target (for example Kubernetes with Helm, with an MI image per release), and deploy only the APIs whose files changed.
* **Events:** enable publisher confirms, add tooling to replay DLQ messages, and publish an AsyncAPI definition of the event contract.

---

## Assumptions

The following assumptions were made for the assignment:

* Jamii Savings and all customer/account data are fictional.
* Amounts are represented in KES unless the account specifies another currency.
* Account numbers contain exactly ten digits.
* Monetary values use `DECIMAL(18,2)`.
* Loan eligibility is indicative and does not perform a real credit-history check.
* The eligibility calculation considers principal and tenure only.
* The SOAP calculator uses integer values, so the installment is returned in whole shillings.
* In a production architecture, MI would normally only be reachable from the API gateway network. Client authentication is therefore handled at the gateway rather than duplicated in MI.
* Only the development environment is connected to an actual deployment target.
* Production deployment is represented as a promotion/dry-run stage using the same archived artifacts.
* Both bonus tasks (A and B) have been implemented.
* Loan-application events are produced by another system; publishers only need the `loan.events` exchange and the `loan.applications` routing key.

---

## Tooling

A small Go-based helper under `tools/` supports the build and deployment scripts.

The helper is built automatically into:

```text
.tools/bin/jamii
```

The Micro Integrator artifacts remain standard WSO2 CAR files. The custom tooling mainly removes unnecessary dependence on Integration Studio metadata and simplifies packaging and API Manager automation.