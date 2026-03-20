# STEP 24 Remediation Follow-up Failure Report

Date: 2026-03-18
Audited scope: Step 24 migration prep and dry-runs after remediation pass
Result: STILL NOT FULLY PASSING

## 1) What went wrong

The code-level failures from the earlier Step 24 audit were addressed, but Step 24 still does not satisfy the required "all 4 conditions are well-met" bar because the staging-only completion work remains undone in the repository status documents.

### Fixed in this remediation pass

1. Legacy payment dates are now preserved as structured payment timestamps instead of being appended into free-form notes.
2. CSV ingestion now validates required headers up front and fails with a specific file/header error when the export shape is wrong.
3. Fully blank CSV rows are ignored so operator spacer lines do not create false row errors.
4. Dry-runs now fail overall when validation errors are present instead of silently succeeding with a report that contains errors.
5. Step 24 tests now cover:
   - payment date preservation
   - missing required headers
   - blank spacer rows
   - invalid date handling

### Why the step still does not pass overall

The repository still explicitly records four Step 24 checklist items as incomplete:

- `TODO.md:459` `Run dry-run migration in staging with anonymized data.`
- `TODO.md:460` `Compare row counts with source system.`
- `TODO.md:461` `Compare billing totals with source system.`
- `TODO.md:462` `Log all manual remediation items.`

`CLAUDE.md:201` also still states that the remaining Step 24 items are blocked on real legacy exports and a staging environment.

Because of that, Condition 4 from the earlier audit is still not met. I cannot truthfully certify that all earlier directions were completed fully, and I also cannot certify the stronger "all 4 conditions are well-met" requirement.

## 2) How to fix it

Complete the remaining staging-dependent migration validation work with real anonymized legacy exports:

1. Run the migration CLI dry-run in staging against the anonymized Step 24 packet.
2. Compare imported row counts against the source-system exports for each migrated entity set.
3. Compare billing totals between source exports and the dry-run/imported output.
4. Record every discrepancy and manual cleanup action in a persistent remediation log.
5. Update `TODO.md` to mark those four items complete only after the evidence exists.
6. Update `CLAUDE.md` so Step 24 is no longer described as blocked or partially complete once the above is done.

## 3) What counts as passing

Step 24 should count as fully passing only when all of the following are true:

1. Local verification remains green:
   - `GOCACHE=/Users/ryan/Documents/aquify/.gocache go test ./internal/migration/... ./cmd/migrate/...`
   - `GOCACHE=/Users/ryan/Documents/aquify/.gocache go test ./internal/app/billing/...`
   - `GOCACHE=/Users/ryan/Documents/aquify/.gocache go run ./cmd/migrate plan`
   - `GOCACHE=/Users/ryan/Documents/aquify/.gocache go run ./cmd/migrate dry-run -input-dir <dir> -report <file>`
2. The migration runner continues to preserve imported payment dates, reject malformed headers, ignore blank rows, and fail on validation errors.
3. The staging dry-run and parity checks are completed with real anonymized legacy exports.
4. `TODO.md` no longer shows the Step 24 staging checklist items as open.
5. `CLAUDE.md` no longer says Step 24 is blocked or partially complete.
