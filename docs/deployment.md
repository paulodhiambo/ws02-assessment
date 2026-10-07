# Build, deploy and pipeline

## Prerequisites

| Tool | Version used | Needed for |
|------|--------------|------------|
| Docker + compose plugin | 29.x / v2 | the whole environment |
| JDK | 17+ (tested on 21) | Maven |
| Maven | 3.9+ | MI build |
| Python 3 | 3.9+ | CAR packaging, deploy helpers (standard library only) |
| Go | 1.24+ | MI artifact tests and the mocks' unit tests (`scripts/test.sh` falls back to the `golang` image if Go is not installed) |
| apictl | 4.6.x | APIM import (`scripts/install-apictl.sh` installs it into `.tools/`, where the scripts look first) |
| curl, zip, rsync | n/a | scripts |

Images: `wso2/wso2mi:4.6.0` and `wso2/wso2am:4.6.0` (multi-arch, so they
run natively on Apple Silicon), plus `mysql:8.4`, `golang:1.27-alpine` (builds the mocks) and
`postman/newman:6-alpine`. APIM needs about 3 GB of RAM for Docker.

## Step by step

```bash
cp .env.example .env                              # optional; defaults work as-is
scripts/build.sh                                  # CARs + APIM packages (runs MI tests)
docker compose --profile apim up -d --build --wait   # DB, mocks, MI, APIM (APIM: ~2-3 min)
scripts/deploy-mi.sh dev                          # deploy the 4 CARs, all-or-nothing
scripts/install-apictl.sh                              # apictl 4.6.4 into .tools/ (scripts find it there)
scripts/deploy-apim.sh dev                        # import 3 APIs + product, all-or-nothing
python3 scripts/apim-demo-consumer.py             # Dev Portal app, subscriptions, keys; prints curl commands
scripts/test.sh --all                             # unit + integration (MI) + chaos
scripts/test.sh --integration --apim              # same suite through the gateway
```

MI only (no APIM): skip `--profile apim` and the APIM steps; the APIs are on
`http://localhost:8290`.

### Choosing backends

The defaults are the public services. For deterministic failure testing:

```bash
CUSTOMER_BACKEND_URL=http://customer-mock:3000 \
LOAN_SOAP_BACKEND_URL=http://soap-mock:8088/calculator.asmx \
docker compose up -d mi        # deployed CARs survive (volume), no redeploy needed
```

## Build (`scripts/build.sh`)

**MI.** `mvn clean install` in `mi/` (`pom.xml`):

| Phase | What runs |
|-------|-----------|
| validate | `build/package_car.py --validate-only`: XML well-formed, artifact names match files, no duplicates across modules |
| test | `go test ./...` in `tests/`: 34 artifact tests (policies and per-API contracts) |
| package | `build/package_car.py` → `target/cars/Jamii{Common,AccountBalance,CustomerProxy,LoanEligibility}_<version>.car` |
| install | CARs attached as Maven artifacts (type `car`) |

`-Dcar.version=1.0.<build>` stamps the CI build number into every CAR.

**APIM.** Each `apim/*-api` project is copied to `dist/apim/<name>/`, the
custom policies it references are copied from `apim/policies/` into
`Policies/`, and the result is zipped. The product is packaged the same way.
These directories and zips are the "API archives".

## Deploy

### MI (`scripts/deploy-mi.sh <env> [--dry-run]`)

1. Preflight: CARs present, management API login (`https://:9164/management`).
2. Back up the currently deployed `Jamii*.car` inside the container.
3. Copy the new CARs to a staging folder in the container, then
   `rm old && mv staged/* carbonapps/` (an atomic rename on the same file system).
4. Poll `GET /management/applications` until every expected `name:version`
   is active. **Any faulty app or a timeout → restore the backup → exit 1.**
5. On success, the replaced CARs are saved to `target/mi-previous/` for a
   cross-tier rollback.

### APIM (`scripts/deploy-apim.sh <env> [--dry-run]`)

1. `apictl add env` / `login` (credentials from `APIM_ADMIN_USER` / `APIM_ADMIN_PASSWORD`).
2. Export each API that already exists (backup).
3. Import the three APIs with `--update --rotate-revision --preserve-provider`
   and a generated `--params` file that points each API at
   `${MI_BACKEND_FOR_APIM}/<path>` for that environment.
4. Import the product (only after all three APIs succeeded).
5. **Any failure → re-import the backups (or delete newly created APIs) →
   exit 1.**

### Environments

`infrastructure/config/<env>.env` holds the per-environment settings (MI
management URL, apictl environment, the backend URL the gateway uses for MI,
the backends MI uses). Values already in the environment (e.g. Jenkins
credentials) take precedence.

- **dev**: the docker-compose stack. Fully wired.
- **prod**: `prod.env.example` documents the shape. Without a `prod.env`
  the scripts run in `--dry-run` mode and print what they would deploy
  where. The prod stage promotes the **same archived artifacts**; nothing
  is rebuilt.

## Jenkins pipeline (`Jenkinsfile`)

```
Build & package → Unit tests → Archive → Dev: environment → Dev: deploy → Dev: integration tests → [Prod: approval → Prod: deploy]
```

| Stage | Detail |
|-------|--------|
| Build & package | `scripts/build.sh --skip-tests --version 1.0.$BUILD_NUMBER` |
| Unit tests | mocks (`go vet` + `go test`) + MI artifact tests |
| Archive | `mi/target/cars/*.car`, `dist/apim/*.zip` (fingerprinted) |
| Dev: environment | `docker compose --profile apim up --wait`; `BACKENDS=mock\|public` picks the backends |
| Dev: deploy | `deploy-mi.sh dev`, then `deploy-apim.sh dev`; if APIM fails, MI is restored from `target/mi-previous` and the build fails |
| Dev: integration tests | newman → MI; Dev Portal onboarding; newman → gateway; chaos (DB, customer and SOAP backends stopped). JUnit reports published |
| Prod | only with `PROMOTE_TO_PROD`; manual `input`; credentials `jamii-prod-mi-admin` / `jamii-prod-apim-admin` |
| post | container logs archived; environment torn down unless `KEEP_DEV_ENV` |

One pipeline for all three APIs: nothing in the `Jenkinsfile` names an
individual API. The scripts loop over all of them, and every deploy step
either completes for all three or leaves the previous release in place, so
there is no silent partial deploy.

### GitHub Actions mirror

`.github/workflows/ci.yml` runs the same stages on every push and pull request
(GitHub-hosted `ubuntu-latest`, mock backends). Because both pipelines only
call `scripts/`, they stay in step. Mapping:

| Jenkins | GitHub Actions |
|---------|----------------|
| `archiveArtifacts` | `actions/upload-artifact` (`artifacts-<version>`, `logs-and-reports-<version>`) |
| `input` approval before prod | the `prod` environment with required reviewers (Settings → Environments) |
| `try/catch` MI restore | step with `if: failure() && steps.apim.outcome == 'failure'` |
| `post { always }` | steps with `if: always()` |

**Agent requirements:** Docker with compose, JDK, Maven, Python 3, curl,
zip, rsync. Credentials in Jenkins are only needed for prod; dev uses the
stock image defaults.
