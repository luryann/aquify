# Step 24 Audit Failure Report

Date: 2026-03-18
Audited scope: Step 24 migration prep and dry-runs
Result: FAILED

## 1) What went wrong

Step 24 does not meet the required "all 4 conditions are well-met" bar.

### Condition 4 failed: Step 24 is not fully complete

The repository itself still marks four Step 24 items as incomplete in `TODO.md`:

- staging dry-run with anonymized data
- source-system row-count comparison
- source-system billing total comparison
- manual remediation log

References:

- `TODO.md:459`
- `TODO.md:460`
- `TODO.md:461`
- `TODO.md:462`
- `CLAUDE.md:201`

Because those are still open, I cannot truthfully certify that all earlier directions were followed and completed fully.

### Condition 1 failed: there are still migration-quality gaps / edge-case risks

The local Step 24 implementation works for the covered happy path, but there are still clear gaps that keep it from being "no bugs, no potential issues, no edge cases":

1. Legacy payment dates are not preserved as first-class payment timestamps.
   The importer reads `paid_at`, but it only appends it into `notes` and then records the payment with the billing service's current timestamp. That means imported payment chronology is not faithfully represented in the in-memory billing record.

   Reference:

   - `server/internal/migration/runner.go:707`
   - `server/internal/migration/runner.go:715`

2. CSV ingestion does not validate required headers up front.
   `loadRows` reads whatever headers are present and silently maps missing columns to empty strings. That means malformed or partially renamed exports degrade into row-level missing-key errors instead of a precise "this export is structurally invalid" failure.

   Reference:

   - `server/internal/migration/runner.go:752`

3. There is no coverage yet for malformed-header, blank-row, or payment-date preservation cases.
   The existing tests cover the primary happy path, duplicate idempotency keys, and an unknown household reference, but not the migration-specific edge cases above.

   Reference:

   - `server/internal/migration/runner_test.go`

## 2) How to fix it

1. Complete the remaining Step 24 checklist items against real anonymized legacy exports in staging:
   - run the dry-run in staging
   - compare source and imported row counts
   - compare source and imported billing totals
   - capture every manual remediation item in a persistent remediation log

2. Preserve legacy payment dates explicitly:
   - extend the billing dry-run path so a migrated payment can carry the legacy `paid_at` as structured data, not only as appended notes
   - if the current billing service cannot accept an explicit payment timestamp, add a migration-only import path or a safe billing-service option for imported historical payments

3. Add structural CSV validation before row processing:
   - validate each required file against a required-header set
   - fail the step with a specific error message when headers are missing or renamed unexpectedly
   - ignore fully blank rows so operator exports with spacer lines do not create noisy false errors

4. Expand Step 24 tests:
   - missing required headers
   - blank rows / whitespace-only rows
   - invalid numeric/date fields
   - payment-date preservation behavior
   - reconciliation-oriented invoice/payment combinations

## 3) What counts as passing

Step 24 should count as passing only when all of the following are true:

1. The local migration CLI and tests pass.
   Verified locally today:
   - `go test ./internal/migration/... ./cmd/migrate/...`
   - `go run ./cmd/migrate plan`
   - `go run ./cmd/migrate dry-run -input-dir <sample-dir> -report <file>`

2. The migration importer has no known unaddressed edge-case gaps in the audited scope:
   - payment dates are preserved correctly
   - required CSV headers are validated explicitly
   - malformed/blank-row behavior is covered by tests

3. The remaining Step 24 repo checklist items are completed with real staging inputs:
   - anonymized staging dry-run executed
   - row-count parity checked
   - billing-total parity checked
   - remediation log produced

4. The repository status documents no longer describe Step 24 as partially complete or blocked.

## 4) Master list of files edited in Step 24

- `CLAUDE.md`
- `docs/migration/legacy-export-inventory.md`
- `docs/migration/source-to-target-field-mapping-template.csv`
- `docs/planning/STEP24-migration-prep-plan.md`
- `server/cmd/migrate/main.go`
- `server/internal/migration/runner.go`
- `server/internal/migration/runner_test.go`

