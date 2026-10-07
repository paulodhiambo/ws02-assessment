# JamiiCustomersAPI (v1)

apictl project for the managed **Customers** API.

| Item            | Value |
|-----------------|-------|
| Gateway URLs    | `https://<gateway>:8243/jamii/customers/v1/{customerId}`, `.../{customerId}/dashboard` (bonus A) |
| Backend         | MI `CustomerAPI` at `${MI_BACKEND_FOR_APIM}/customers`, which proxies JSONPlaceholder `/users/{id}` |
| Definition      | `Definitions/swagger.yaml` (OpenAPI 3.0, hand-written) |
| Security        | OAuth2 (`Authorization: Bearer <token>`) |
| Subscription tiers | Gold, Silver |
| API-level limit | `50KPerMin` |
| Gateway policies | `jamiiCorrelation` (request), `jamiiSecurityHeaders` (response, fault) |

**Why OAuth2:** the response is customer PII (name, email, phone, address),
so it gets the same token-based protection as account data.

The gateway strips the caller's `Authorization` header before calling MI,
and MI strips the remaining credential and internal headers before calling
the backend. The backend never sees consumer credentials.
