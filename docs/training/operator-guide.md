# Operator Guide — DARE Platform

**Audience:** Technical operators, DevOps engineers, on-call engineers
**Version:** 1.0
**Last Updated:** 2026-03-23

---

## 1. System Overview

DARE is a full-stack swim club operations platform. The following diagram shows the major components and their relationships:

```
                            ┌─────────────────────────────────┐
                            │         Cloudflare (Edge/WAF)    │
                            └────────────┬────────────────────┘
                                         │
              ┌──────────────────────────┼───────────────────────────┐
              │                          │                           │
       ┌──────▼──────┐           ┌───────▼───────┐          ┌───────▼───────┐
       │  Admin UI   │           │  Parent Portal │          │ Swimmer Portal│
       │  (Next.js)  │           │   (Next.js)    │          │  (Next.js)    │
       └──────┬──────┘           └───────┬───────┘          └───────┬───────┘
              │                          │                           │
              └──────────────────────────┼───────────────────────────┘
                                         │ HTTP/REST (/api/v1/)
                                ┌────────▼────────┐
                                │   DARE API       │
                                │   (Go, Chi)      │
                                └────┬───────┬────┘
                                     │       │
                     ┌───────────────┘       └──────────────────┐
                     │                                          │
              ┌──────▼──────┐                          ┌────────▼───────┐
              │ PostgreSQL  │                          │  Redis          │
              │ (primary DB)│                          │  (sessions,     │
              └─────────────┘                          │   rate limit,   │
                                                       │   Asynq queues) │
                                                       └────────────────┘
                                     │
                              ┌──────▼──────┐
                              │  MinIO/S3   │
                              │  (artifacts,│
                              │   imports,  │
                              │   exports)  │
                              └─────────────┘
                                     │
                              ┌──────▼──────┐
                              │  Keycloak   │
                              │  (OIDC/SSO) │
                              └─────────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    │                │                │
             ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
             │  Postmark   │ │  Stripe     │ │  OpenTel.   │
             │  (Email)    │ │  (Billing)  │ │  Prometheus │
             └─────────────┘ └─────────────┘ │  Grafana    │
                                              └─────────────┘
```

---

## 2. Daily Operations

### 2.1 Checking System Health

**Liveness probe** (is the process running?):
```bash
curl https://api.dare.club/health/live
# Expected: 200 OK {"status":"ok"}
```

**Readiness probe** (is the system ready to handle traffic?):
```bash
curl https://api.dare.club/health/ready
# Expected: 200 OK {"status":"ready","db":"ok","redis":"ok","storage":"ok"}
```

**Grafana dashboards:** Navigate to `https://grafana.dare.club`. Key dashboards:
- **DARE API Overview** — request rate, error rate, p95 latency
- **DB Connections** — pool utilization, query duration
- **Background Jobs** — queue depths, DLQ count, job failure rate
- **Runtime Metrics** — Go GC, goroutine count, memory

**Prometheus alerts:** Check `https://grafana.dare.club/alerting` for active alerts. Critical alerts also page via PagerDuty (see `docs/runbooks/on-call.md`).

### 2.2 Monitoring Background Job Queues

The admin API exposes queue metrics:

```bash
# List all queues and their depths
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     https://api.dare.club/api/v1/admin/jobs

# Sample response
{
  "queues": [
    {"name": "imports", "pending": 2, "active": 1, "failed": 0},
    {"name": "billing", "pending": 0, "active": 0, "failed": 0},
    {"name": "email",   "pending": 5, "active": 2, "failed": 1}
  ]
}
```

Expected healthy state: all queues have `pending < 20` and `failed = 0`. If `failed > 0`, check the DLQ.

### 2.3 Reviewing DLQ Entries

```bash
# List dead-letter queue entries
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     https://api.dare.club/api/v1/admin/jobs/dlq

# Retry a specific DLQ entry
curl -X POST \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     https://api.dare.club/api/v1/admin/jobs/dlq/{taskId}/retry

# Delete a DLQ entry
curl -X DELETE \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     https://api.dare.club/api/v1/admin/jobs/dlq/{taskId}
```

### 2.4 Reviewing the Audit Log

```bash
# Fetch recent audit events (last 50)
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     "https://api.dare.club/api/v1/orgs/$ORG_ID/audit?limit=50&order=desc"
```

The admin UI also provides a filterable audit log at `/audit`.

---

## 3. Deployments

### 3.1 CI/CD Pipeline

1. **Commit merged to `main`** — GitHub Actions runs `make test` and `make lint`.
2. **On passing CI** — image is built and tagged `sha-<commit>`.
3. **Auto-deploy to staging** — `docker compose up -d` on the staging host with the new image tag.
4. **Manual production promote** — requires engineering approval in the GitHub Actions UI.

### 3.2 Promoting a Release to Production

```bash
# From the deploy host or CI runner
./deploy/scripts/promote.sh sha-<commit-hash>

# The script:
# 1. Pulls the tagged image
# 2. Runs pending DB migrations (make db-up)
# 3. Performs a rolling restart (zero-downtime via container orchestrator)
# 4. Runs smoke checks against /health/ready
# 5. Notifies #dare-ops Slack on success or failure
```

### 3.3 Rolling Back

If a deployment causes incidents, roll back immediately:

```bash
./deploy/scripts/rollback.sh

# The script:
# 1. Identifies the previously running image tag
# 2. Pulls the previous image
# 3. Restarts containers with the previous image
# 4. Does NOT run DB migrations (schema rollback is manual — see below)
```

**DB schema rollback:** If the new deployment included a migration that must be reversed:
```bash
make db-down  # runs one step of goose down
# Verify the expected schema version with: make db-status
```

### 3.4 Checking Logs

```bash
# API container logs
docker compose logs -f dare-api

# Filter for errors only
docker compose logs dare-api | grep '"level":"error"'

# Worker logs
docker compose logs -f dare-worker

# All services
docker compose logs -f
```

Structured logs are in JSON format. Key fields: `level`, `time`, `msg`, `user_id`, `org_id`, `trace_id`, `span_id`.

---

## 4. Database Operations

### 4.1 Running Migrations

```bash
# Apply all pending migrations
make db-up

# Check current migration version and pending count
make db-status

# Roll back one migration
make db-down
```

Migrations use [Goose](https://github.com/pressly/goose). Migration files live in `db/migrations/`.

### 4.2 Connecting to the Database

```bash
# Using DATABASE_URL from environment
psql $DATABASE_URL

# Explicit connection
psql -h <host> -p 5432 -U <user> -d aquify

# Example for local dev
psql -h 127.0.0.1 -p 5432 -d aquify -U postgres
```

### 4.3 Common DBA Tasks

**Check connection count:**
```sql
SELECT count(*), state FROM pg_stat_activity GROUP BY state;
```

**Check for slow queries:**
```sql
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND now() - query_start > interval '5 seconds'
ORDER BY duration DESC;
```

**Check table bloat:**
```sql
SELECT schemaname, tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS total_size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
LIMIT 20;
```

**Manual VACUUM (if autovacuum is lagging):**
```sql
VACUUM ANALYZE swimmers;
VACUUM ANALYZE audit_events;
```

---

## 5. Secrets Rotation

All secrets are stored in the deployment environment (`.env` on the host, or secrets manager in production). Rotate each secret type as follows:

### 5.1 Session Signing Key (`SESSION_SIGNING_KEY`)

1. Generate a new 64-byte random key: `openssl rand -hex 64`
2. Update the environment variable on all API instances.
3. Restart the API: `docker compose restart dare-api`
4. All existing sessions will be invalidated. Users must log in again.

### 5.2 Stripe API Key (`STRIPE_SECRET_KEY`)

1. In the Stripe dashboard, create a new restricted key.
2. Update `STRIPE_SECRET_KEY` in the deployment environment.
3. Restart the API.
4. Verify Stripe integration: `curl .../api/v1/billing/invoices` returns 200.

### 5.3 Stripe Webhook Secret (`STRIPE_WEBHOOK_SECRET`)

1. In the Stripe dashboard, regenerate the webhook endpoint signing secret.
2. Update `STRIPE_WEBHOOK_SECRET` in the deployment environment.
3. Restart the API.
4. Send a Stripe test event and verify it is processed correctly.

### 5.4 Postmark Server Token (`POSTMARK_SERVER_TOKEN`)

1. In the Postmark dashboard, create a new server token.
2. Update `POSTMARK_SERVER_TOKEN` in the deployment environment.
3. Restart the API.
4. Send a test announcement email and verify delivery.

### 5.5 Database Password

1. Update the PostgreSQL user password: `ALTER USER dare_api WITH PASSWORD 'new-password';`
2. Update `DATABASE_URL` in the deployment environment.
3. Restart the API.

### 5.6 Keycloak Client Secret (`OIDC_CLIENT_SECRET`)

1. In Keycloak admin, regenerate the client secret for the DARE client.
2. Update `OIDC_CLIENT_SECRET` in the deployment environment.
3. Restart the API.
4. Test login via browser SSO flow.

---

## 6. Scaling

### When to Scale

| Signal | Action |
|---|---|
| API p95 latency > 500 ms sustained | Add API replicas |
| DB connection pool consistently > 80% | Increase `DB_POOL_MAX_CONNS` or add read replica |
| Asynq queue depth consistently > 100 | Add worker replicas |
| Redis memory > 80% | Increase Redis instance size |

### How to Scale API Instances

In `docker-compose.yml` (or Kubernetes deployment), increase the replica count:
```yaml
services:
  dare-api:
    replicas: 4  # increase from 2
```

Then run: `docker compose up -d --scale dare-api=4`

### Increasing DB Connection Pool

Update `DB_POOL_MAX_CONNS` in the API environment. Note: this must not exceed the PostgreSQL `max_connections` setting. Check with:
```sql
SHOW max_connections;
SELECT count(*) FROM pg_stat_activity;
```

---

## 7. Incident Response

See the on-call runbook: `docs/runbooks/on-call.md`

Key contacts and escalation paths: `docs/training/launch-owner-matrix.md`

For support incidents (parent can't log in, import stuck, payment missing): `docs/training/support-runbook.md`
