# API design

| API | Gateway (APIM) | MI (direct) | Backend |
|-----|----------------|-------------|---------|
| Account balance | `GET /jamii/accounts/v1/{accountNumber}/balance` | `GET :8290/accounts/{accountNumber}/balance` | MySQL via `AccountsDataService` |
| Customer        | `GET /jamii/customers/v1/{customerId}`          | `GET :8290/customers/{customerId}`          | JSONPlaceholder `/users/{id}` |
| Dashboard (bonus A) | `GET /jamii/customers/v1/{customerId}/dashboard` | `GET :8290/customers/{customerId}/dashboard` | both of the above, in parallel |
| Loan eligibility| `POST /jamii/loans/v1/eligibility`              | `POST :8290/loans/eligibility`              | DNE Online SOAP calculator |
| API Product     | `/jamii/core/...` (all three operations)        | n/a                                         | n/a |

OpenAPI definitions: `apim/*-api/Definitions/swagger.yaml`.

## Common conventions

**Request headers.** `X-Correlation-ID` is optional. When it is present and
valid it is propagated to the backend and echoed back; otherwise one is
generated (`gw-<uuid>` at the gateway, `<uuid>` in MI).

**Response headers.** `X-Correlation-ID` and `X-Request-ID` are always set.
Backend headers (`Server`, `X-Powered-By`, CDN headers, cookies) are always
removed. The gateway adds `Strict-Transport-Security`,
`X-Content-Type-Options` and `Cache-Control: no-store`.

**Error envelope.** Identical for all three APIs and every failure type
(validation, not found, DB, REST backend, SOAP fault, timeout):

```json
{
  "error": { "code": "ACCOUNT_NOT_FOUND", "message": "No account exists with the given account number.", "status": 404 },
  "requestId": "82ae2640-6e70-4f25-b4c5-e062b0f649ed",
  "correlationId": "demo-corr-0001",
  "timestamp": "2026-10-07T16:29:32.389Z"
}
```

`error.code` is the stable contract for clients; `message` is for humans.
Gateway-generated errors (401, 403, 429) keep APIM's own format, because they
happen before MI is reached.

### Error catalogue

| HTTP | code | API | When |
|------|------|-----|------|
| 400 | `INVALID_ACCOUNT_NUMBER` | accounts | path is not exactly 10 digits |
| 400 | `INVALID_CUSTOMER_ID` | customers | path is not a positive integer |
| 400 | `INVALID_REQUEST` | loans | body is not JSON, or fails the JSON Schema |
| 404 | `ACCOUNT_NOT_FOUND` | accounts | no row for the account number |
| 404 | `CUSTOMER_NOT_FOUND` | customers | backend returned 404 |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | loans | Content-Type is not `application/json` |
| 422 | `LOAN_REQUEST_REJECTED` | loans | SOAP `Client` fault (the service rejected the input) |
| 500 | `INTERNAL_ERROR` | all | anything unexpected |
| 502 | `CUSTOMER_BACKEND_ERROR` | customers | backend returned another non-2xx status |
| 502 | `LOAN_SERVICE_ERROR` | loans | SOAP `Server` fault, or a response that is neither a result nor a fault |
| 503 | `ACCOUNT_DB_UNAVAILABLE` | accounts | any failure during the database call |
| 503 | `CUSTOMER_BACKEND_UNAVAILABLE` | customers | connection refused or reset, or endpoint suspended |
| 503 | `LOAN_SERVICE_UNAVAILABLE` | loans | connection refused or reset, or endpoint suspended |
| 503 | `DASHBOARD_UNAVAILABLE` | dashboard | both the customer and accounts sections failed |
| 504 | `CUSTOMER_BACKEND_TIMEOUT` | customers | no response in 5s |
| 504 | `LOAN_SERVICE_TIMEOUT` | loans | no response in 10s |

503 and 504 are safe to retry; 4xx are not.

## 1a. Account balance

Response, exactly as the specification requires:

```json
{
  "accountNumber": "0100000001",
  "status": "ACTIVE",
  "balance": { "amount": 152340.75, "currency": "KES" },
  "requestId": "10d00c1e-fa31-4e58-9802-49ab7e69bcbf",
  "timestamp": "2026-10-07T16:29:32.347Z"
}
```

`amount` is a JSON number rendered straight from `DECIMAL(18,2)`, so
`0.00` and `152340.75` keep their scale, with no float round-trip in MI.

## 1b. Customer proxy

**Backend: JSONPlaceholder** (`https://jsonplaceholder.typicode.com/users/{id}`).
Why it was chosen:

- it is free, needs no key and is widely available;
- `/users/{id}` is a realistic customer profile **with PII** (name, email,
  phone, address), which exercises masking and `Cache-Control: no-store`;
- it returns a real `404 {}` for unknown ids, so not-found mapping is
  tested against real behaviour;
- it sits behind Cloudflare and returns plenty of infrastructure headers,
  which makes header stripping visible.

The local mock (`mocks/customer-service`) serves the same contract and adds
failure modes: `/users/500` returns 500 and `/users/999` hangs for 15s.

Gateway value added on top of a pass-through: correlation ID injection;
stripping `Authorization`, `Cookie`, `apikey`, `X-JWT-Assertion`,
`X-Internal-*` and `X-Forwarded-For` on the way out; dropping every backend
header on the way back; a 5s timeout; circuit breaking; and fault mapping.
The body itself is passed through unchanged, which is the point of a
pass-through proxy.

## 1c. Loan eligibility (SOAP to REST)

**Backend: DNE Online calculator** (`http://www.dneonline.com/calculator.asmx`).
There is no public loan-eligibility SOAP service, and the assignment calls
the check an "eligibility check" in quotes. So I chose the most stable public
SOAP 1.1 test service available and used it for the arithmetic at the heart
of an affordability check:

- it is a real .NET ASMX service with a WSDL, a SOAPAction and document/literal;
- it produces **real SOAP faults with stack traces** (Int32 overflow,
  divide by zero), which is exactly what must not leak to clients.

Request:

```json
{ "customerId": "1", "monthlyIncome": 120000, "existingMonthlyDebt": 15000,
  "requestedAmount": 500000, "tenureMonths": 12 }
```

Mapping:

| Step | Where | Rule |
|------|-------|------|
| 1 | MI | Validate against `LoanEligibilityRequestSchema` (JSON Schema draft-04; `additionalProperties: false`) |
| 2 | SOAP | `Divide(intA = round(requestedAmount), intB = tenureMonths)` → `monthlyInstallment` |
| 3 | MI | `maxAffordableInstallment = 0.40 × monthlyIncome − existingMonthlyDebt` (rounded to cents) |
| 4 | MI | `eligible = monthlyInstallment ≤ maxAffordableInstallment` |

Response:

```json
{
  "customerId": "1",
  "eligible": false,
  "decision": "NOT_ELIGIBLE",
  "reason": "Monthly installment exceeds 40% of monthly income after existing debt.",
  "loan": { "requestedAmount": 500000, "tenureMonths": 12, "monthlyInstallment": 41667,
            "maxAffordableInstallment": 33000, "currency": "KES" },
  "requestId": "1d1152c0-fe1b-4dc4-9b98-7cd7ca449da4",
  "timestamp": "2026-10-07T16:40:38.973Z"
}
```

Why this mapping:

- The SOAP call does the part a remote "loan engine" would own (the
  installment). The policy threshold (40% debt-to-income, a common
  lending rule of thumb) stays in the integration layer, where it is
  visible and testable.
- The SOAP result (`DivideResult`, an Int32) maps to a JSON number. The
  service rounds half-to-even (41,666.67 → 41,667), so `monthlyInstallment`
  is in whole shillings. The original `requestedAmount` is echoed back
  unrounded, so the client can see both.
- The output is a business response (`eligible`, `decision`, `reason`) rather
  than a mirror of the SOAP structure.

SOAP faults: `faultcode`/`faultstring` are logged (first line only, 300
characters at most) and mapped by fault code. `soap:Client` (the service
rejected the input, e.g. `requestedAmount` > 2,147,483,647) becomes 422
`LOAN_REQUEST_REJECTED`; `soap:Server` becomes 502 `LOAN_SERVICE_ERROR`. No
XML, fault string or stack trace reaches the client; the Postman tests
assert this.

Simplifications, stated openly: principal only, no interest; no credit
history. A real implementation would call the bank's loan engine; that is a
change of endpoint and template, not of the API contract.

## Bonus A – customer dashboard

`GET /customers/{customerId}/dashboard` (a second resource on `CustomerAPI`;
exposed as an extra operation on `JamiiCustomersAPI`).

```
                      ┌─▶ dashboard-customer-section ─▶ CustomerBackendEP (JSONPlaceholder)
scatter-gather ───────┼─▶ dashboard-accounts-section ─▶ AccountsDataService.getAccountsByCustomer
(parallel, 7s)        └─▶ heartbeat (answers immediately)
        │
        └─▶ merge (script mediator): look sections up by name ─▶ 200 / 404 / 503
```

```json
{
  "customerId": "1",
  "partial": true,
  "customer": { "id": 1, "name": "Leanne Graham", "email": "Sincere@april.biz", "phone": "1-770-736-8031 x56442" },
  "accounts": null,
  "errors": [ { "section": "accounts", "code": "ACCOUNT_DB_UNAVAILABLE", "message": "Accounts are temporarily unavailable." } ],
  "requestId": "6fbeb56c-6604-4515-80bc-71bc89d319b6",
  "timestamp": "2026-10-07T17:22:24.483Z"
}
```

**Partial data or fail the whole response? Partial.** The dashboard is a
read-only view; a customer can still use their balances when the profile
service is slow, and vice versa. Clients must check `partial` and treat
`null` sections as unavailable rather than empty. (`accounts: []` means the
customer genuinely has no accounts.)

| Outcome | Response |
|---------|----------|
| both sections OK | 200, `partial: false` |
| one section failed or missed the 7s deadline | 200, `partial: true`, failed section `null`, reason in `errors` |
| customer backend says the customer does not exist | 404 `CUSTOMER_NOT_FOUND` (no point showing accounts for an unknown id) |
| both sections failed | 503 `DASHBOARD_UNAVAILABLE` |

How it works, and what testing it showed:

- Each branch ends with a `{"section": ..., "ok": ...}` object. Branch
  failures (timeouts, connection errors, DB errors) are caught by an
  `onError` handler. The aggregator ignores messages produced by error
  handlers, so the handler logs and drops the message; a missing section
  counts as failed.
- The aggregation timer only starts when the first message arrives. A
  third "heartbeat" branch answers immediately, so a request where both
  real branches fail still completes at the deadline (without it, that
  request hung).
- Branches complete in any order, so the merge looks sections up by name.
- The accounts array is built by MySQL (`JSON_ARRAYAGG`), which avoids
  XML-to-JSON pitfalls (single-element arrays, account numbers with
  leading zeros being turned into numbers).

## Exposure in API Manager

| API | Security | Subscription tiers | API-level limit |
|-----|----------|--------------------|-----------------|
| JamiiAccountsAPI | OAuth2 | Gold, Silver | 50KPerMin |
| JamiiCustomersAPI | OAuth2 | Gold, Silver | 50KPerMin |
| JamiiLoanEligibilityAPI | API key (`apikey` header) | **Bronze** | **10KPerMin** |
| JamiiCoreBankingProduct | OAuth2 or API key | Gold | n/a |

Loan eligibility gets a lower tier because every call is a round trip to an
external, rate-limited SOAP service.

Custom gateway policies (`apim/policies`), attached to all three APIs:
`jamiiCorrelation` (request: correlation ID plus `X-Consumer-App`) and
`jamiiSecurityHeaders` (response and fault).

## Consuming the APIs (Developer Portal)

1. **Discover**: open `https://<apim>:9443/devportal`. The three APIs and the
   *Jamii Core Banking* product are listed (tags `jamii`, `banking`). Each
   has its OpenAPI definition, a try-it console and documentation.
2. **Create an application** (Applications → Add), e.g. "Mobile Banking".
3. **Subscribe** the application, either to the **product** (one
   subscription, Gold) or to individual APIs on the tiers they offer.
4. **Get credentials** for the application:
   - *Production Keys → OAuth2 Tokens → Generate Keys*: consumer key and
     secret. Call `POST /oauth2/token` with `grant_type=client_credentials`
     to get a bearer token for Accounts and Customers.
   - *API Key → Generate*: a key to send as the `apikey` header to Loan
     Eligibility.
5. **Call** the gateway URLs shown on each API's overview page. Usage and
   throttling are tracked per application and subscription.

`scripts/apim-demo-consumer.py` does steps 2–4 through the Developer Portal
REST API. CI uses it before running the test suite through the gateway.
