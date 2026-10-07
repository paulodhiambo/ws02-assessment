# Demo script (≈5 minutes)

Before recording: stack up, CARs and APIs deployed, keys generated:

```bash
scripts/build.sh && docker compose --profile apim up -d --build --wait
scripts/deploy-mi.sh dev && scripts/deploy-apim.sh dev
eval "$(scripts/apim-demo-consumer.sh | grep '^export ')"   # sets GW, TOKEN (1h), APIKEY
```

Have three terminals: commands, `docker compose logs -f mi | grep event`, and
`docker compose logs -f customer-mock`.

## 0:00 – Setup (30s)

- Show the repo tree: `mi/`, `apim/`, `Jenkinsfile`, `docker-compose.yml`.
- Developer Portal (`https://localhost:9443/devportal`): three APIs plus the
  *Jamii Core Banking* product; JamiiDemoApp subscriptions (Gold / Bronze).

## 0:30 – Account balance, DB-backed (1m15)

```bash
curl -sk $GW/accounts/v1/0100000001/balance -H "Authorization: Bearer $TOKEN" -H 'X-Correlation-ID: demo-video-001' -i
curl -sk $GW/accounts/v1/0100000099/balance -H "Authorization: Bearer $TOKEN"    # 404 ACCOUNT_NOT_FOUND
curl -sk $GW/accounts/v1/12ab/balance       -H "Authorization: Bearer $TOKEN"    # 400 INVALID_ACCOUNT_NUMBER
docker compose stop accounts-db
curl -sk $GW/accounts/v1/0100000001/balance -H "Authorization: Bearer $TOKEN"    # 503 ACCOUNT_DB_UNAVAILABLE
docker compose start accounts-db
```

Point out: the exact spec envelope; the security headers added by the
gateway policy; in the log terminal the account is masked (`******0001`) and
the same correlation ID appears on REQUEST_IN, BACKEND_CALL and RESPONSE_OUT.

## 1:45 – Customer proxy (1m15)

```bash
curl -sk $GW/customers/v1/1 -H "Authorization: Bearer $TOKEN" -H 'X-Internal-Debug: true' -i
curl -sk $GW/customers/v1/4242 -H "Authorization: Bearer $TOKEN"     # 404 CUSTOMER_NOT_FOUND
curl -sk $GW/customers/v1/1                                          # 401 from the gateway (no token)
```

Point out: the body is JSONPlaceholder's, unchanged; no Cloudflare or
`Server` headers came back; `Cache-Control: no-store`. If MI points at the
mock, the customer-mock terminal shows `x-correlation-id` arriving and
`authorization` / `x-internal-debug` missing. Then:

```bash
curl -sk $GW/customers/v1/999 -H "Authorization: Bearer $TOKEN"      # 504 after 5s (mock backend)
```

## 3:00 – Loan eligibility, SOAP to REST (1m30)

```bash
curl -sk $GW/loans/v1/eligibility -H "apikey: $APIKEY" -H 'Content-Type: application/json' \
  -d '{"customerId":"1","monthlyIncome":120000,"existingMonthlyDebt":15000,"requestedAmount":500000,"tenureMonths":12}'
curl -sk $GW/loans/v1/eligibility -H "apikey: $APIKEY" -H 'Content-Type: application/json' \
  -d '{"customerId":"1","monthlyIncome":120000,"requestedAmount":300000,"tenureMonths":12}'
curl -sk $GW/loans/v1/eligibility -H "apikey: $APIKEY" -H 'Content-Type: application/json' \
  -d '{"customerId":"1","monthlyIncome":120000,"requestedAmount":3000000000,"tenureMonths":12}'   # 422
curl -sk $GW/loans/v1/eligibility -H "apikey: $APIKEY" -H 'Content-Type: application/json' -d '{"customerId": "1",'   # 400
```

Point out: the mapping (41,667 vs 33,000 → NOT_ELIGIBLE); for the 422,
show the log line with the real .NET stack trace from the SOAP fault, and
note the client only saw `LOAN_REQUEST_REJECTED`. API key auth, Bronze tier.

## 4:30 – Pipeline (30s)

Show the Jenkins stage view (or `Jenkinsfile`) and run
`scripts/test.sh --integration --apim`: 104 assertions passing (with the mock backends) through the
gateway. Mention the rollback tests (`docs/deployment.md`).
