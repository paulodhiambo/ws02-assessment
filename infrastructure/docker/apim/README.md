# API Manager runtime

The compose file runs the stock `wso2/wso2am:4.6.0` image (all-in-one:
Publisher, Developer Portal, Key Manager, Gateway). It is behind the `apim`
profile because it needs roughly 2-4 GB of RAM and 2-3 minutes to start:

```bash
docker compose --profile apim up -d
# wait until https://localhost:9443/publisher responds (admin / admin)
```

| Port | Purpose                                         |
|------|-------------------------------------------------|
| 9443 | Publisher, Developer Portal, Admin, REST APIs   |
| 8243 | Gateway (HTTPS)                                 |
| 8280 | Gateway (HTTP)                                  |

No custom image is needed: everything API-specific (definitions, policies,
throttling tiers, the API Product) is imported with `apictl` by
`scripts/deploy-apim.sh`, so the runtime stays disposable.

The gateway reaches MI over the compose network at `http://mi:8290`.

## Production notes (not done here)

- Change the default `admin` credentials and keystores.
- Use an external database instead of the embedded H2.
- Run the Gateway separately from the control plane and scale it horizontally.
