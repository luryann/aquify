# DARE Platform — Rollback Procedure

## When to Rollback

Initiate a rollback when any of the following conditions are met:

| Trigger | Threshold | Source |
|---|---|---|
| HTTP 5xx error rate | > 5% of requests for 5 consecutive minutes | Grafana / Cloudflare analytics |
| Health check failing | `/healthz` returning non-200 for > 2 minutes | Uptime Kuma / Prometheus alert |
| P0 bug reported | Data loss, billing error, auth bypass, or PII exposure | Engineering / customer report |
| Background job queue depth | > 500 unprocessed jobs for > 10 minutes | Prometheus `dare_job_queue_depth` |
| Smoke tests failing post-deploy | Smoke test gate returns non-zero | CI pipeline |

**Do not rollback for:**
- Isolated user-reported UI bugs that do not affect core workflows (log, triage, hotfix instead)
- Cloudflare WAF false-positives (adjust WAF rule, no rollback needed)
- Staging-only failures (fix forward)

---

## Decision Authority

| Scenario | Who can initiate rollback |
|---|---|
| P0 bug / data loss | Any engineer on the team, no approval required |
| Error rate > 5% | On-call engineer |
| Cosmetic / non-critical bug | Team lead or engineering lead approval required |
| Database migration rollback | Engineering lead approval required; DBA review recommended |

When in doubt, **rollback first and investigate after**. A brief outage during rollback is preferable to an extended incident on a broken deploy.

---

## Automated Rollback

The fastest rollback path uses `deploy/scripts/rollback.sh`, which:

1. Reads the previous `IMAGE_TAG` from `/opt/dare/.rollback-state`.
2. Re-deploys production containers with the previous image tag.
3. Confirms health check passes.
4. Sends a Slack notification to `#dare-ops`.

```bash
# On the VPS (or triggered by CI):
/opt/dare/deploy/scripts/rollback.sh
```

The rollback script does NOT touch the database. If a database migration is involved, see the database rollback section below.

The previous image tag is written to `/opt/dare/.rollback-state` by the deploy script on every successful production deploy.

---

## Manual Rollback

If the automated script is unavailable or the rollback state file is missing:

### Step 1 — Identify the Previous Image Tag

```bash
# List recent images available in the registry
docker images dare-api --format "{{.Tag}}" | head -20
# Or check the deploy history in GitHub Actions → Deployments
```

### Step 2 — Update IMAGE_TAG and Re-deploy

```bash
ssh deploy@vps.dare.club
cd /opt/dare

# Set to previous known-good tag
export IMAGE_TAG=main-abc1234   # replace with actual tag

docker compose \
  -f docker-compose.production.yml \
  --project-name dare-production \
  --env-file .env.production \
  up -d --pull always dare-api dare-admin dare-worker
```

### Step 3 — Verify

```bash
curl -sf https://api.dare.club/healthz && echo "API healthy" || echo "API unhealthy"
curl -sf https://admin.dare.club && echo "Admin healthy" || echo "Admin unhealthy"
```

### Step 4 — Run Smoke Tests

```bash
make cutover-smoke \
  API_BASE_URL=https://api.dare.club \
  AUTH_TOKEN=$(cat /opt/dare/.smoke-token) \
  ORG_ID=org-production
```

If smoke tests pass, the rollback is complete. Proceed to post-rollback steps.

---

## Database Rollback

### When to Run Goose Down

Run `goose down` only when:
- The migration has been deployed for less than 30 minutes AND no data has been written through the new schema.
- The migration is purely additive (new table or column) and no application code has started writing to it.
- The migration includes a corresponding down migration that is verified safe.

**Do NOT run `goose down` when:**
- The migration has dropped or renamed columns.
- Existing data has been transformed or backfilled.
- Any production writes have occurred against the new schema.

These situations require a forward-fix migration rather than a down migration.

### Goose Down Procedure

```bash
ssh deploy@vps.dare.club
cd /opt/dare

# Stop API and worker to prevent writes during migration rollback
docker compose -f docker-compose.production.yml stop dare-api dare-worker

# Roll back one migration
DATABASE_URL=$(grep DATABASE_URL .env.production | cut -d= -f2-) \
  goose -dir db/migrations postgres "$DATABASE_URL" down

# Verify migration status
DATABASE_URL=$(grep DATABASE_URL .env.production | cut -d= -f2-) \
  goose -dir db/migrations postgres "$DATABASE_URL" status

# Restart with previous image
IMAGE_TAG=main-abc1234 docker compose \
  -f docker-compose.production.yml \
  up -d dare-api dare-worker
```

### When Migration Is Too Risky to Rollback

If the migration cannot be safely rolled back:
1. Keep the API on the previous image (which does not use the new schema).
2. Write a forward-fix migration to correct the schema issue.
3. Test the fix on staging.
4. Deploy the fix migration + new application image together.

Document the situation in the incident report.

---

## Communication Protocol

### Immediate (< 5 minutes after decision to rollback)

- Post to Slack `#dare-ops`:
  ```
  [ROLLBACK INITIATED] Deploying rollback for <brief description>.
  Trigger: <error rate / health check / bug report>
  Impact: <what users may experience>
  ETA: ~5-10 minutes
  On-call: @<your-name>
  ```

- Update the status page (Uptime Kuma at `status.dare.club`):
  - Create a new maintenance/incident window.
  - Set affected component: "DARE Platform API" or "Admin Portal" as appropriate.
  - Status: "Investigating" initially, then "Identified" once rollback is underway.

### During Rollback (every 10 minutes if not resolved)

Post updates to `#dare-ops` with current status and next steps.

### On Restore

- Post to `#dare-ops`: `[RESOLVED] Rollback complete. Platform healthy. Incident report to follow.`
- Update status page: mark incident resolved.
- If the outage exceeded 30 minutes: draft a brief email to affected organization contacts.

---

## Post-Rollback

### Immediate

1. Confirm smoke tests pass.
2. Check Grafana dashboards: error rate, latency, job queue depth, and active sessions.
3. Review logs for any residual errors: `docker logs dare-api --tail=200`.

### Within 24 Hours

1. Write an incident report. Minimum content:
   - Timeline (deploy time, detection time, rollback time, resolution time)
   - Root cause (preliminary if investigation ongoing)
   - Customer impact (number affected, duration)
   - What went wrong (code, process, test gap)
   - Immediate remediation (rollback)
   - Follow-up actions (fix, additional tests, process changes)

2. Open a GitHub issue tagged `incident` and `p0` (or `p1`) with the incident report.

3. Schedule a blameless post-mortem within 48 hours if the incident involved data loss, billing errors, or > 30 minutes of customer-facing impact.

### Longer Term

- Add or improve the test that would have caught the regression.
- Add the failed condition to the smoke test suite if not already covered.
- Review the rollback runbook for gaps this incident exposed.
