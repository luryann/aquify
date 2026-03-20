# DARE Platform — On-Call Runbook

**Last updated:** 2026-03-18
**Owner:** Engineering Team
**Escalation:** See PagerDuty rotation schedule

---

## Overview

This runbook is the first-response guide for the on-call engineer. It covers
the most common production alerts and the steps to diagnose, mitigate, and
resolve them. All alerts are defined in
[`docs/observability/prometheus/alerts.yaml`](../observability/prometheus/alerts.yaml).

---

## Quick References

| Resource | URL / Command |
|---|---|
| Grafana Dashboards | `https://grafana.<your-domain>/dashboards` |
| Prometheus | `https://prometheus.<your-domain>` |
| Loki Logs | `https://grafana.<your-domain>/explore` |
| Jaeger / Tempo Traces | `https://tracing.<your-domain>` |
| Kubernetes Cluster | `kubectl --context <prod-context>` |
| Stripe Dashboard | `https://dashboard.stripe.com` |
| PagerDuty | `https://dare.pagerduty.com` |
| Slack Alert Channel | `#dare-alerts` |

---

## Severity Definitions

| Severity | Response Time | Description |
|---|---|---|
| **P0** | Immediate | Complete service outage or data loss |
| **P1** | < 15 min | Payments broken, imports blocked, auth failing |
| **P2** | < 1 h | Elevated error rate, degraded performance |
| **P3** | Business hours | Non-critical anomalies, warning thresholds |

---

## Alert Runbooks

### Import Failures

**Alerts:** `ImportFailureSpike`, `ImportFailureRateHigh`

**Symptoms:** Import jobs failing in bursts; households unable to upload results
files; parse workers returning errors.

**Diagnosis:**

1. Check the DARE import failure dashboard: `dare-imports` in Grafana.
2. Look for correlated worker errors in Loki:
   ```
   {app="dare-api"} |= "parse import: "
   ```
3. Check the `file_imports` table in PostgreSQL:
   ```sql
   SELECT id, status, error_message, created_at
   FROM file_imports
   WHERE status IN ('failed', 'parsing')
   ORDER BY created_at DESC
   LIMIT 20;
   ```
4. Inspect object storage (MinIO/S3) for upload availability:
   ```bash
   mc alias set dare https://s3.<domain> <access_key> <secret_key>
   mc admin info dare
   ```
5. Check if a recent deploy introduced a parser regression by reviewing the
   `parse_import` job handler and recent commits to `internal/meetfiles/`.

**Mitigation:**

- If a parser bug is confirmed: roll back the deployment.
- If object storage is unavailable: resolve the S3/MinIO incident; re-enqueue
  stuck imports by transitioning their status back to `uploaded` and re-dispatching
  the `parse_import` job.
- If a specific file format causes failures: mark the import as `archived` with
  an error note and notify the club admin.

**Escalate to:** Lead engineer if failure rate remains >30% after 30 minutes.

---

### Billing Webhook Failures

**Alerts:** `PaymentWebhookFailureSpike`, `WebhookProcessingLagHigh`

**Symptoms:** Households reporting payments not reflecting; Stripe events not
being processed; ledger mismatches reported by reconciliation.

**Diagnosis:**

1. Check Grafana webhook lag dashboard: `dare-webhook-lag`.
2. Inspect the `webhook_events` table for events stuck in `processing`:
   ```sql
   SELECT id, stripe_event_id, event_type, status, received_at, processed_at
   FROM webhook_events
   WHERE status IN ('processing', 'failed')
   ORDER BY received_at DESC
   LIMIT 20;
   ```
3. Check Stripe Dashboard → Webhooks → Recent deliveries for failed attempts.
4. Search Loki for billing handler errors:
   ```
   {app="dare-api"} |= "billing reconcile" |= "error"
   ```
5. Verify the `STRIPE_WEBHOOK_SECRET` environment variable is set correctly —
   signature verification failures will silently reject webhooks.

**Mitigation:**

- If webhook secret is wrong: update the environment variable and restart the
  API. Stripe will retry failed events up to 72 hours.
- If the billing service is crashing: check for memory/CPU saturation and
  restart the API pod.
- If reconciliation shows a mismatch:
  - Do **not** manually edit the ledger.
  - Trigger a manual reconciliation:
    ```bash
    curl -X POST /api/v1/stripe/reconcile \
      -H "Authorization: Bearer <admin-token>"
    ```
  - If mismatch persists: escalate to Finance team with a Stripe balance export.

**Escalate to:** Finance team + Lead engineer if ledger mismatch > $100.

---

### Database Saturation

**Alerts:** `DBConnectionSaturation`, `DBConnectionSaturationWarning`

**Symptoms:** API requests timing out; connection pool exhausted errors in logs;
`FATAL: sorry, too many clients` in PostgreSQL logs.

**Diagnosis:**

1. Check Grafana — query `pg_stat_activity_count` vs `pg_settings_max_connections`.
2. Identify which application is holding the most connections:
   ```sql
   SELECT application_name, state, count(*)
   FROM pg_stat_activity
   GROUP BY application_name, state
   ORDER BY count(*) DESC;
   ```
3. Look for long-running idle-in-transaction sessions:
   ```sql
   SELECT pid, state, query_start, state_change, query
   FROM pg_stat_activity
   WHERE state = 'idle in transaction'
   ORDER BY query_start;
   ```
4. Check connection pool settings in `WORKER_CONCURRENCY_PER_QUEUE`.

**Mitigation:**

- Terminate idle-in-transaction connections older than 5 minutes:
  ```sql
  SELECT pg_terminate_backend(pid)
  FROM pg_stat_activity
  WHERE state = 'idle in transaction'
    AND state_change < NOW() - INTERVAL '5 minutes';
  ```
- Temporarily reduce `WORKER_CONCURRENCY_PER_QUEUE` to lower connection pressure.
- If saturation persists: add a PgBouncer instance in front of PostgreSQL with
  transaction-mode pooling.

**Escalate to:** Infrastructure team if DB saturation causes P1/P0 impact.

---

### API Errors

**Alerts:** `APIErrorRateHigh`, `APIHighLatency`

**Symptoms:** Users receiving 5xx responses; high error rate in Grafana; slow
page loads in the admin portal.

**Diagnosis:**

1. Check the `dare-api-errors` dashboard for which routes are failing.
2. Search Loki for 5xx log lines:
   ```
   {app="dare-api"} | json | status >= 500
   ```
3. Check for recent deploys that might have introduced a regression.
4. If latency is high: check for slow queries using `pg_stat_statements`:
   ```sql
   SELECT query, mean_exec_time, calls
   FROM pg_stat_statements
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```
5. Inspect OTel traces in Tempo/Jaeger for slow spans.

**Mitigation:**

- Roll back the most recent deployment if error rate spiked post-deploy.
- If a slow query is identified: add an index or query hint. Do **not** modify
  schema in production without the standard migration process.
- Scale the API horizontally if CPU or memory is the bottleneck.

---

## General Procedures

### Restarting the API

```bash
# Kubernetes
kubectl rollout restart deployment/dare-api -n dare-production

# Docker Compose (local / staging)
docker compose restart api
```

### Checking Application Logs

```bash
# Kubernetes
kubectl logs -n dare-production -l app=dare-api --tail=100 -f

# Loki (Grafana Explore)
{app="dare-api"} | json | level="error"
```

### Emergency Database Backup

```bash
pg_dump \
  --host=<db-host> \
  --port=5432 \
  --username=dare_api \
  --format=custom \
  --file=emergency_backup_$(date +%Y%m%d_%H%M%S).pgdump \
  dare_production
```

### Triggering Manual Reconciliation

```bash
curl -s -X POST https://api.<domain>/api/v1/stripe/reconcile \
  -H "Authorization: Bearer <owner-token>" \
  -H "Content-Type: application/json" | jq .
```

---

## Escalation Matrix

| Scenario | Escalate To | Channel |
|---|---|---|
| Payments not recording | Finance + Lead Eng | `#dare-billing` |
| Data loss suspected | CTO + Lead Eng | Phone/PagerDuty P0 |
| Auth service down | Lead Eng + Infra | `#dare-incidents` |
| Stripe outage | Monitor status.stripe.com | No internal escalation needed |
| Personal data exposure | Security + CTO | Out-of-band encrypted message |

---

## Post-Incident

After every P0 or P1 incident:

1. Write a **blameless post-mortem** within 48 hours.
2. File tickets for all identified action items.
3. Update this runbook if new failure modes were discovered.
4. Share learnings in the weekly engineering sync.

Template: `docs/runbooks/postmortem-template.md`
