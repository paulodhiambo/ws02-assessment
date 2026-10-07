# Custom gateway mediation policies

Shared by all three APIs (attached as API-level policies in each `api.yaml`).
`scripts/build.sh` copies them into each packaged project's `Policies/`.

| Policy                    | Flow             | What it does |
|---------------------------|------------------|--------------|
| `jamiiCorrelation` v1     | request          | Keeps a valid caller `X-Correlation-ID`, otherwise generates `gw-<uuid>`; adds `X-Consumer-App` (subscribed application name) so MI logs show which consumer made the call |
| `jamiiSecurityHeaders` v1 | response, fault  | Adds `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`; removes `X-Consumer-App` |

Each policy is a spec (`.yaml`, APIM `operation_policy_specification`) plus a
Synapse template (`.j2`).
