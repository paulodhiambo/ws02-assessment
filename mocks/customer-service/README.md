# Customer service mock

An offline stand-in for **JSONPlaceholder** `GET /users/{id}`, the backend
behind `GET /customers/{customerId}`. It returns the same JSON shape for
users 1–4, `404 {}` for unknown ids, and the same kind of leaky headers
(`X-Powered-By`, `Server`, ...) that the MI proxy must strip.

Failure modes for the gateway tests:

| Request        | Behaviour                                      | Expected via MI |
|----------------|------------------------------------------------|-----------------|
| `/users/500`   | 500 Internal Server Error                       | 502 `CUSTOMER_BACKEND_ERROR` |
| `/users/999`   | answers after `SLOW_DELAY_MS` (default 15s)     | 504 `CUSTOMER_BACKEND_TIMEOUT` after 5s |
| container stopped | connection refused                          | 503 `CUSTOMER_BACKEND_UNAVAILABLE` |

Each request is logged with the headers it received, so you can watch what
the gateway injected and stripped: `docker compose logs -f customer-mock`.

Switch MI to it with `CUSTOMER_BACKEND_URL=http://customer-mock:3000`.

## Run

A single static Go binary with no dependencies; the Docker image is built
`FROM scratch` (about 8 MB).

```bash
go run .              # http://localhost:3000/users/1   (PORT, SLOW_DELAY_MS)
go test ./...
```
