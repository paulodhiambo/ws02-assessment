# JamiiLoanEligibilityAPI (v1)

apictl project for the managed **Loan Eligibility** API.

| Item            | Value |
|-----------------|-------|
| Gateway URL     | `POST https://<gateway>:8243/jamii/loans/v1/eligibility` |
| Backend         | MI `LoanEligibilityAPI` at `${MI_BACKEND_FOR_APIM}/loans` (REST to SOAP) |
| Definition      | `Definitions/swagger.yaml` (OpenAPI 3.0, hand-written, including the request schema) |
| Security        | API key (`apikey: <key>` header) |
| Subscription tiers | **Bronze only** |
| API-level limit | **`10KPerMin`** |
| Gateway policies | `jamiiCorrelation` (request), `jamiiSecurityHeaders` (response, fault) |

**Different throttling tier:** every call costs a round trip to an
external, rate-limited SOAP service. So consumers get the smaller Bronze
quota and the API as a whole is capped lower than the other two.

**Why an API key:** the response is an indicative calculation with no account
data or PII. The typical caller is a server-side partner or branch system,
where a revocable key per application is simpler than an OAuth client. The
key is still bound to a subscription, so throttling and analytics apply.
