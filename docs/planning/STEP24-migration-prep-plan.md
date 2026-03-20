# STEP 24 Migration Prep Plan

Date: 2026-03-18
Owner: Codex
Status: In progress

## Goal

Finish the non-blocked implementation work in `TODO.md` section `24) Migration prep and dry-runs` by landing:

- migration packet artifacts for operators
- a source-to-target mapping template
- a legacy export inventory with explicit HY3 coverage and ownership
- a runnable Go migration CLI skeleton
- first-pass import steps for households, swimmers, groups, historical best times, open invoices, and payment history
- idempotency handling and validation report output

## Inputs reviewed

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`
- existing in-memory domain services under `server/internal/app`

## Constraints and assumptions

- The current backend still uses in-memory services for most domain logic, so Step 24 should reuse those services rather than inventing a parallel migration-only model.
- Real staging dry-runs, source-system parity checks, and remediation logging cannot be completed without actual legacy export files and a provisioned staging environment.
- The migration tool should therefore be able to run locally in a deterministic dry-run mode against CSV exports and emit a machine-readable validation report.
- Source files for Step 24 will live under a dedicated migration input directory instead of ad hoc filenames spread across the repo.

## Proposed delivery

### 1. Migration packet docs

- Add `docs/migration/source-to-target-field-mapping-template.csv`
- Add `docs/migration/legacy-export-inventory.md`
- Keep naming/operator guidance close to the migration CLI contract

### 2. Migration CLI

- Add `server/cmd/migrate`
- Add `server/internal/migration`
- Support commands:
  - `plan`
  - `dry-run`
- Accept a directory of CSV inputs and emit a JSON validation report

### 3. Initial implemented steps

- households
- swimmers
- groups
- historical best times
- open invoices
- payment history

### 4. Idempotency strategy

- Each imported row must provide a stable legacy key
- The runner will keep per-step idempotency indexes and skip duplicate keys deterministically
- The validation report will include duplicate/skip counts and per-step warnings

## Expected remaining blockers after this pass

- real anonymized legacy extracts
- staging dry-run execution
- source-system row-count parity
- source-system billing total parity
- manual remediation log from actual dry-run output

## Verification target

- `go test ./internal/migration/... ./cmd/migrate/...` in `server/`
- manual `go run ./cmd/migrate plan`
- manual `go run ./cmd/migrate dry-run -input-dir <dir> -report <file>`
