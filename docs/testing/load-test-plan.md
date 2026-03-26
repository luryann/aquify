# Load Test Plan — DARE Platform

**Version:** 1.0
**Last Updated:** 2026-03-23
**Owner:** Engineering (Performance)

---

## 1. Purpose

This document defines the load testing strategy for the DARE platform. It specifies the test environment, scenarios, pass/fail thresholds, how to interpret results, and known areas to monitor. Load tests must pass before any release candidate is promoted to production.

---

## 2. Test Environment

### 2.1 Environment

Load tests run against the **staging environment** only. Never run load tests against production.

| Component | Staging Specification |
|---|---|
| API instances | 2 replicas, 2 vCPU / 4 GB RAM each |
| PostgreSQL | 1 primary, db.t3.medium equivalent, 20-connection pool |
| Redis | Single node, cache.t3.micro equivalent |
| MinIO / S3 | Standard S3-compatible object storage |
| Keycloak | Single node, 1 vCPU / 2 GB RAM |

### 2.2 Data Seed

Before running load tests, seed the staging database with production-like volumes:

| Entity | Minimum Seed Count |
|---|---|
| Households | 500 |
| Swimmers | 1 200 |
| Groups | 20 |
| Meets | 50 (mixed lifecycle states) |
| Invoices | 2 000 |
| Audit events | 10 000 |
| Import jobs | 100 |

Seed script: `make load-seed` (uses `server/cmd/seed` with `--profile load-test`).

### 2.3 Stub vs Real Auth

For initial load tests, use stub bearer tokens (`AUTH_ENABLE_DEV_STUB_TOKENS=true`). For pre-launch certification runs, disable stubs and issue real tokens via Keycloak to test the full auth path.

---

## 3. Scenarios

### Scenario 1: Admin Browsing

Simulates admins and coaches navigating core list pages during a typical business day.

| Parameter | Value |
|---|---|
| Script | `tests/load/k6/admin-browse.js` |
| Concurrent users | 50 VUs |
| Duration | 5 minutes |
| Ramp-up | 0 (flat load) |
| Endpoints exercised | `/members/households`, `/members/swimmers`, `/meets`, `/billing/invoices`, `/audit` |

**Thresholds:**

| Metric | Pass Criterion |
|---|---|
| `http_req_duration` p95 | < 500 ms |
| Error rate | < 1% |
| Per-endpoint p95 | < 500 ms |

---

### Scenario 2: File Import

Simulates concurrent club staff uploading meet result files during results processing day.

| Parameter | Value |
|---|---|
| Script | `tests/load/k6/import-upload.js` |
| Concurrent users | 10 VUs |
| Duration | 3 minutes |
| Flow | Create job → presigned PUT → finalize |
| File size | ~1 KB synthetic SD3 |

**Thresholds:**

| Metric | Pass Criterion |
|---|---|
| End-to-end p95 (`import_e2e_latency`) | < 3 000 ms |
| `create_job_latency` p95 | < 500 ms |
| `upload_latency` p95 | < 2 000 ms |
| `finalize_latency` p95 | < 800 ms |
| Error rate | < 2% |

---

### Scenario 3: Webhook Burst

Simulates a burst of Stripe webhook events arriving in parallel, as occurs when multiple payments are processed simultaneously.

| Parameter | Value |
|---|---|
| Script | `tests/load/k6/webhook-burst.js` |
| Concurrent users | 20 VUs |
| Duration | 1 minute |
| Target throughput | ~100 webhooks/second |
| Payload type | `payment_intent.succeeded` |

**Thresholds:**

| Metric | Pass Criterion |
|---|---|
| `webhook_latency` p99 | < 200 ms |
| `http_req_duration` p99 | < 200 ms |
| Error rate | < 0.1% |

---

### Scenario 4: Portal Sessions

Simulates parents browsing the portal during commitment open periods — the highest-traffic window.

| Parameter | Value |
|---|---|
| Script | `tests/load/k6/portal-load.js` |
| Concurrent users | 20 VUs |
| Duration | 5 minutes |
| Endpoints exercised | Session, swimmers, meets, invoices, practices/calendar |

**Thresholds:**

| Metric | Pass Criterion |
|---|---|
| `http_req_duration` p95 | < 800 ms |
| Per-endpoint p95 | < 800 ms (calendar < 1 000 ms) |
| Error rate | < 1% |

---

### Scenario 5: Meet Entry Day (Combined)

Simulates the peak event: commitment window is open and parents are committing / admins are reviewing simultaneously.

| Parameter | Value |
|---|---|
| Scripts | `portal-load.js` + `admin-browse.js` run in parallel |
| Total VUs | 70 (20 portal + 50 admin) |
| Duration | 10 minutes |

**Thresholds:**

| Metric | Pass Criterion |
|---|---|
| Combined `http_req_duration` p95 | < 800 ms |
| Combined error rate | < 1% |
| DB connection wait p95 | < 50 ms (from Grafana) |

To run the combined scenario:

```bash
k6 run tests/load/k6/portal-load.js &
k6 run tests/load/k6/admin-browse.js &
wait
```

---

## 4. Pass/Fail Thresholds Summary

| Scenario | Metric | Threshold | Blocking? |
|---|---|---|---|
| Admin browsing | p95 response time | < 500 ms | Yes |
| Admin browsing | Error rate | < 1% | Yes |
| File import | End-to-end p95 | < 3 000 ms | Yes |
| File import | Error rate | < 2% | Yes |
| Webhook burst | p99 response time | < 200 ms | Yes |
| Webhook burst | Error rate | < 0.1% | Yes |
| Portal sessions | p95 response time | < 800 ms | Yes |
| Portal sessions | Error rate | < 1% | Yes |
| Meet entry day | Combined p95 | < 800 ms | Yes |
| Meet entry day | DB connection wait p95 | < 50 ms | Warning |

A release is **blocked** if any threshold marked "Yes" fails.

---

## 5. Baseline Metrics

Before each release, record the following baseline metrics from a clean run and store them in the release notes:

- `http_req_duration` p50, p95, p99
- `http_req_failed` rate
- `iterations` total count
- `data_received` and `data_sent` totals
- Per-endpoint latency trends (from k6 tags)
- DB connection pool stats at peak (from Grafana)
- Redis memory at peak

Baseline file location: `docs/testing/baselines/<release-tag>.json`
Generate with: `k6 run --out json=baseline.json tests/load/k6/admin-browse.js`

---

## 6. Interpreting Results

### k6 Summary Output

```
http_req_duration.............: avg=83ms  min=12ms  med=71ms  max=1.2s  p(90)=145ms p(95)=210ms
http_req_failed...............: 0.12%   ✓ 4778 ✗ 6
iterations....................: 4823    96.46/s
```

- **avg/p50:** Typical user experience
- **p95/p99:** Worst-case experience for most users — these are the threshold values
- **max:** Outliers; investigate if dramatically higher than p99
- **http_req_failed:** Percentage of requests that returned a non-2xx status

### Threshold Failures

k6 exits with a non-zero code when a threshold fails. The failing metric is marked with `✗`:

```
✗ http_req_duration............: p(95)=712ms threshold=p(95)<500
```

This means the scenario failed and the release is blocked.

### Escalation

1. If a threshold fails on the first run, investigate the Grafana dashboards for DB query slowness, memory pressure, or connection pool exhaustion.
2. Re-run with `--http-debug=full` to capture raw request/response logs.
3. If DB wait times spike, increase `DB_POOL_MAX_CONNS` and re-run.
4. If the failure persists after investigation, escalate to the on-call engineer before releasing.

---

## 7. Known Bottlenecks

| Component | What to Watch | Grafana Panel |
|---|---|---|
| DB connection pool | `db_pool_acquire_duration_ms` p95; pool size near max | DB Connections |
| Redis rate limiter | `rate_limit_rejected_total` counter | Rate Limiting |
| File upload / S3 | MinIO PUT duration during import tests | Object Storage |
| Asynq job queue | Queue depth rising during import burst | Background Jobs |
| Go GC | GC pause duration spikes during webhook burst | Runtime Metrics |

---

## 8. CI Integration

Load tests are in the `perf` GitHub Actions stage:

```yaml
# .github/workflows/perf-tests.yml
on:
  workflow_dispatch:
    inputs:
      scenario:
        description: 'Scenario to run (admin-browse, import-upload, webhook-burst, portal-load)'
        default: 'admin-browse'
```

Trigger manually: `gh workflow run perf-tests.yml --field scenario=admin-browse`

Required before: release candidate promotion to production.
