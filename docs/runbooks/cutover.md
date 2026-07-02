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

For migration dry-run evidence capture, prefer the bundle command so the dry-run
report, source row-count comparison, billing-total comparison, remediation log,
and command output are archived together. This is a partial diagnostic bundle,
not enough to pass final launch evidence verification:

```bash
make launch-evidence-bundle \
  INPUT_DIR=/absolute/path/to/final-export \
  SOURCE_COUNTS=/absolute/path/to/source-counts.json \
  SOURCE_BILLING_TOTAL_CENTS=<source-outstanding-cents> \
  ORG_ID=<prod-org-id> \
  API_BASE_URL=https://api.example.com \
  AUTH_TOKEN=<admin-token> \
  RUN_PROVIDER_VALIDATION=false
```

Archive the generated `evidence/launch-<timestamp>/` directory with the staging
or rehearsal record. A clean partial migration bundle contains
`migration-report.json`, `remediation-log.md`, `cutover-certification.log`,
`cutover-smoke.log`, and `cutover-billing-check.log`.

Provider validation can be captured as part of the same bundle. For staging,
run with real Stripe test-mode credentials and the Postmark sandbox stream.
Provider validation is complete-evidence mode by default, so webhook, refund,
and Postmark inputs are required unless `REQUIRE_COMPLETE_PROVIDER_VALIDATION=false`
is set for a diagnostic-only run:

```bash
make launch-evidence-bundle \
  INPUT_DIR=/absolute/path/to/final-export \
  SOURCE_COUNTS=/absolute/path/to/source-counts.json \
  SOURCE_BILLING_TOTAL_CENTS=<source-outstanding-cents> \
  ORG_ID=<staging-org-id> \
  API_BASE_URL=https://staging-api.example.com \
  AUTH_TOKEN=<admin-token> \
  EXPECT_STRIPE_WEBHOOK_EVENT_ID=<evt_from_stripe_dashboard> \
  CONFIRM_STRIPE_REFUND=true \
  REFUND_INVOICE_ID=<invoice-id> \
  REFUND_PAYMENT_ID=<payment-id> \
  REFUND_AMOUNT_CENTS=<small-refund-cents> \
  POSTMARK_CAMPAIGN_ID=<sandbox-campaign-id> \
  CONFIRM_POSTMARK_SEND=true \
  REQUIRE_COMPLETE_PROVIDER_VALIDATION=true
```

Two full parallel meet cycles are captured by listing the completed meet IDs
and explicitly attesting that no manual data patching was used:

```bash
make launch-evidence-bundle \
  INPUT_DIR=/absolute/path/to/final-export \
  ORG_ID=<staging-org-id> \
  API_BASE_URL=https://staging-api.example.com \
  AUTH_TOKEN=<admin-token> \
  RUN_PARALLEL_MEET_EVIDENCE=true \
  MEET_IDS=<completed-meet-id-1>,<completed-meet-id-2> \
  NO_MANUAL_PATCHING_ATTESTATION=true
```

For final TODO closeout, generate one complete bundle that contains migration,
provider, two-meet-cycle, and stability evidence, then run the verifier:

On Windows workstations, run the native preflight first to catch missing
environment variables, input files, and source-count keys before moving the
bundle run to a Bash-capable environment:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/evidence-preflight.ps1 -Mode final
```

```bash
make launch-evidence-bundle \
  INPUT_DIR=/absolute/path/to/final-export \
  SOURCE_COUNTS=/absolute/path/to/source-counts.json \
  SOURCE_BILLING_TOTAL_CENTS=<source-outstanding-cents> \
  ORG_ID=<prod-org-id> \
  API_BASE_URL=https://api.example.com \
  AUTH_TOKEN=<admin-token> \
  EXPECT_STRIPE_WEBHOOK_EVENT_ID=<evt_from_stripe_dashboard> \
  CONFIRM_STRIPE_REFUND=true \
  REFUND_INVOICE_ID=<invoice-id> \
  REFUND_PAYMENT_ID=<payment-id> \
  REFUND_AMOUNT_CENTS=<small-refund-cents> \
  POSTMARK_CAMPAIGN_ID=<campaign-id> \
  CONFIRM_POSTMARK_SEND=true \
  RUN_PARALLEL_MEET_EVIDENCE=true \
  MEET_IDS=<completed-meet-id-1>,<completed-meet-id-2> \
  NO_MANUAL_PATCHING_ATTESTATION=true \
  RUN_STABILITY_REPORT=true \
  STABILITY_WINDOW_LABEL=+24h \
  NO_OPEN_P0_P1_ATTESTATION=true \
  SUPPORT_INCIDENTS_FILE=/absolute/path/to/support-incidents.csv \
  PROMETHEUS_URL=https://prometheus.example.com
```

```bash
make verify-launch-evidence EVIDENCE_DIR=/absolute/path/to/evidence/launch-<timestamp>
```

The verifier is intentionally strict by default. For a partial diagnostic bundle
only, run the Go verifier directly or pass explicit opt-out flags through
`VERIFY_LAUNCH_EVIDENCE_FLAGS`; do not use those opt-outs to close `TODO.md`
launch evidence items.

## Phase 2 — Pre-cutover backup

Post-launch stability reports can be generated as standalone evidence or as
part of the bundle after the first +30m/+2h/+8h/+24h checkpoints:

```bash
make stability-report-evidence \
  API_BASE_URL=https://api.example.com \
  AUTH_TOKEN=<admin-token> \
  ORG_ID=<prod-org-id> \
  STABILITY_WINDOW_LABEL=+24h \
  NO_OPEN_P0_P1_ATTESTATION=true \
  SUPPORT_INCIDENTS_FILE=/absolute/path/to/support-incidents.csv \
  PROMETHEUS_URL=https://prometheus.example.com \
  REQUIRE_SUPPORT_INCIDENTS_FILE=true
```

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

- launch evidence bundle directory from `make launch-evidence-bundle`
- rehearsal report
- pre-cutover backup path
- production validation report
- smoke test output
- billing reconciliation output
- certification suite output
- provider validation output from `make provider-validation-evidence` or
  `provider-validation/` in the launch evidence bundle
- two-meet-cycle output from `make parallel-meet-cycle-evidence` or
  `parallel-meet-cycles/` in the launch evidence bundle
- post-launch stability report output from `make stability-report-evidence` or
  `stability-report/` in the launch evidence bundle
- final signoff record
- cutover start/end timestamps
