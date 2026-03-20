# DARE Platform — Cutover Runbook

**Last updated:** 2026-03-18
**Owner:** Engineering + Operations
**Related docs:** [release-gate-checklist.md](../planning/release-gate-checklist.md), [hypercare.md](./hypercare.md), [cutover-signoff-template.md](../migration/cutover-signoff-template.md)

## Purpose

This runbook turns Step 26 into an executable launch procedure for the DARE
Platform cutover from the legacy club operations system.

## Roles

| Role | Responsibility |
|---|---|
| Cutover commander | Owns timeline, checkpoints, and Go/No-Go calls |
| Migration operator | Runs rehearsal, backups, and migration commands |
| QA lead | Runs smoke and certification checks |
| Finance approver | Confirms reconciliation parity |
| Product approver | Confirms user-facing launch readiness |
| Operations approver | Confirms support desk, runbooks, and comms readiness |

## Required inputs before the window opens

- production API base URL
- production `DATABASE_URL`
- production backup destination
- production admin bearer token with owner/admin access
- target org ID for smoke and reconciliation checks
- Step 24 migration input directory
- completed [release-gate-checklist.md](../planning/release-gate-checklist.md)

## Phase 0 — Pre-window preparation

1. Confirm the freeze window start/end times with Product, Engineering,
   Operations, and Finance.
2. Announce the freeze window to staff and note that no legacy data changes are
   allowed after freeze start.
3. Confirm rollback owner and rollback decision threshold.
4. Verify production observability access:
   - Grafana dashboards from `docs/observability/grafana/`
   - Prometheus alerts from `docs/observability/prometheus/alerts.yaml`
   - on-call rotation from [on-call.md](./on-call.md)
5. Pre-stage the cutover inputs:
   - anonymized rehearsal report from Step 24
   - final export set from the legacy platform
   - signoff template from `docs/migration/cutover-signoff-template.md`

## Phase 1 — Final rehearsal

Run the migration CLI against the final export structure before touching
production:

```bash
make cutover-rehearsal INPUT_DIR=/absolute/path/to/final-export REPORT_PATH=/tmp/cutover-rehearsal.json
```

Acceptance criteria:

- the rehearsal exits successfully
- no missing required files are reported
- warnings are reviewed and assigned owners
- row-count and billing-total deltas are documented for Finance review

## Phase 2 — Pre-cutover backup

Take a database snapshot immediately before the production migration.

Example command:

```bash
pg_dump \
  --format=custom \
  --file=/secure/backups/aquify_pre_cutover_$(date +%Y%m%d_%H%M%S).pgdump \
  "$DATABASE_URL"
```

Record:

- backup file path
- operator
- timestamp
- restore validation owner

Do not continue until backup creation succeeds.

## Phase 3 — Production migration

1. Reconfirm legacy freeze is active.
2. Re-run database migration status:

```bash
make db-status
```

3. Apply pending application schema migrations if needed:

```bash
make db-up
```

4. Run the migration CLI against the final export set:

```bash
cd server && go run ./cmd/migrate dry-run \
  -input-dir /absolute/path/to/final-export \
  -report /tmp/cutover-production-validation.json \
  -org-id <prod-org-id> \
  -actor-id cutover-migration
```

5. If dry-run validation is clean, run the production migration process used by
   the designated operator path for the environment.
6. Archive the validation report with the cutover record.

Stop conditions:

- missing required export files
- validation errors without approved remediation
- billing totals outside the approved threshold
- unresolved P0 issue

## Phase 4 — Validation and smoke tests

Run the smoke checks against production:

```bash
make cutover-smoke \
  API_BASE_URL=https://api.example.com \
  AUTH_TOKEN=<admin-token> \
  ORG_ID=<prod-org-id>
```

Run the certification fixture suite from the repo:

```bash
make cutover-certify
```

Run billing reconciliation:

```bash
make cutover-billing-check \
  API_BASE_URL=https://api.example.com \
  AUTH_TOKEN=<admin-token> \
  ORG_ID=<prod-org-id>
```

Minimum validation checklist:

- `/api/v1/health/live` returns `200`
- `/api/v1/health/ready` returns `200`
- `/api/v1/openapi` returns `200`
- authenticated `/api/v1/authz/ping` returns `200`
- billing reconciliation is within approved threshold
- certification suite passes

## Phase 5 — Signoff

Collect approvals in `docs/migration/cutover-signoff-template.md`:

- Product
- Engineering
- Operations
- Finance

Do not switch traffic until all four signoffs are recorded.

## Phase 6 — Traffic switch

1. Lower DNS TTL ahead of the approved cutover window.
2. Switch DNS / load balancer / edge routing to the new platform.
3. Confirm that live traffic reaches the new platform.
4. Re-run `make cutover-smoke` against the public production hostname.
5. Announce cutover completion in the launch channel.

Rollback triggers:

- sustained 5xx spike above agreed P0 threshold
- broken billing reconciliation
- import/export certification failure on launch data
- production auth outage

## Phase 7 — Hypercare

Follow the first-24-hours monitoring plan in [hypercare.md](./hypercare.md).

## Evidence to archive

- rehearsal report
- pre-cutover backup path
- production validation report
- smoke test output
- billing reconciliation output
- certification suite output
- final signoff record
- cutover start/end timestamps
