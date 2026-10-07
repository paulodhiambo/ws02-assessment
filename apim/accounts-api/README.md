# JamiiAccountsAPI (v1)

apictl project for the managed **Accounts** API.

| Item            | Value |
|-----------------|-------|
| Gateway URL     | `https://<gateway>:8243/jamii/accounts/v1/{accountNumber}/balance` |
| Backend         | MI `AccountBalanceAPI` at `${MI_BACKEND_FOR_APIM}/accounts` (set per environment by `scripts/deploy-apim.sh`) |
| Definition      | `Definitions/swagger.yaml` (OpenAPI 3.0, hand-written) |
| Security        | OAuth2 (`Authorization: Bearer <token>`) |
| Subscription tiers | Gold, Silver |
| API-level limit | `50KPerMin` |
| Gateway policies | `jamiiCorrelation` (request), `jamiiSecurityHeaders` (response, fault); source in `../policies` |

**Why OAuth2:** balances are sensitive financial data. Tokens are short-lived,
tied to a subscribed application and revocable, and the same scheme can carry
end-user context later (authorization code flow) without changing the API.

Files: `api.yaml` (APIM metadata), `api_meta.yaml` (import defaults),
`deployment_environments.yaml` (gateway environment). `scripts/build.sh`
copies the referenced policies into `Policies/` when it packages the project
into `dist/apim/accounts-api`.
