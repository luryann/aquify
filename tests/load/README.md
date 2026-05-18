# Load Testing — DARE Platform

## Overview

Load tests for the DARE platform are written using [k6](https://k6.io), a modern, developer-friendly load testing tool. Tests live under `tests/load/k6/` and are organized by scenario.

---

## Prerequisites

Install k6:

```bash
# macOS
brew install k6

# Linux
sudo apt-get install k6

# Docker
docker pull grafana/k6
```

---

## Running Tests

### Basic run

```bash
k6 run tests/load/k6/<script>.js
```

### With environment variables

```bash
k6 run \
  -e API_BASE_URL=https://api-staging.dare.club \
  -e AUTH_TOKEN=your-token-here \
  -e ORG_ID=org-demo \
  tests/load/k6/admin-browse.js
```

### Via Docker

```bash
docker run --rm -i grafana/k6 run - \
  -e API_BASE_URL=http://host.docker.internal:8080 \
  -e AUTH_TOKEN=stub-admin-token \
  -e ORG_ID=org-demo \
  < tests/load/k6/admin-browse.js
```

### All scenarios (sequential)

```bash
make load-test
```

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `API_BASE_URL` | `http://localhost:8080` | Base URL for the DARE API |
| `AUTH_TOKEN` | `stub-admin-token` | Bearer token for authentication |
| `ORG_ID` | `org-demo` | Organization ID sent as `X-Org-Id` header |
| `PARENT_TOKEN` | `stub-parent` | Bearer token for parent portal tests |
| `WEBHOOK_SECRET` | `whsec_test_secret` | Stripe webhook signing secret for webhook tests |

---

## Available Scripts

| Script | VUs | Duration | Description |
|---|---|---|---|
| `admin-browse.js` | 50 | 5 min | Admin browsing: households, swimmers, meets, invoices, audit |
| `import-upload.js` | 10 | 3 min | File import flow: create job → upload → finalize |
| `webhook-burst.js` | 20 | 1 min | Stripe webhook throughput at sustained rate |
| `portal-load.js` | 20 | 5 min | Parent portal: dashboard, meets, invoices, calendar |

---

## Thresholds

### API Endpoints (admin-browse, portal-load)
- **p95 response time:** < 500 ms
- **Error rate:** < 1%

### File Uploads (import-upload)
- **p95 end-to-end time:** < 3 000 ms
- **Error rate:** < 2%

### Webhook Ingestion (webhook-burst)
- **p99 response time:** < 200 ms
- **Error rate:** < 0.1%

---

## CI Integration

Load tests run in the **`perf` stage** of the CI pipeline. This stage is:

- **Manually triggered** — not run on every commit (too expensive)
- **Required before release candidates** targeting staging/production
- Triggered via: `gh workflow run perf-tests.yml --ref main`

To add a new script to CI, update `.github/workflows/perf-tests.yml` and add the script name to the matrix.

---

## Interpreting Results

k6 prints a summary at the end of each run. Key metrics to review:

```
http_req_duration.............: avg=83ms  min=12ms  med=71ms  max=1.2s  p(90)=145ms p(95)=210ms
http_req_failed...............: 0.12%
iterations....................: 4823
```

A run **passes** when all configured thresholds are met. k6 exits with code `0` on pass and non-zero on failure.

---

## Known Bottlenecks

Monitor these during load runs:

- **DB connection pool:** Watch `db_pool_size` and `db_wait_duration` metrics in Grafana. Increase `DB_POOL_MAX_CONNS` if pool exhaustion occurs.
- **Redis rate limiter:** If rate limit errors spike, check `SECURITY_RATE_LIMIT_GLOBAL_REQUESTS` configuration.
- **File upload / S3:** MinIO/S3 PUT throughput. Watch object storage response times in the import-upload scenario.
- **Background worker queue:** Asynq queue depth may back up during import-upload bursts. Check `/api/v1/admin/jobs` during runs.
