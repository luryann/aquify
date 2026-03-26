# Step 31: Schema Drift Fixes Plan

## Goal
Close the remaining schema-drift cleanup work by ensuring repo-expected columns are present, invoice date handling is resilient, and startup logs a clear schema error instead of surfacing opaque repository failures.

## Findings
- `db/migrations/20260323000200_fix_missing_columns.sql` already covers the core Step 31 column drift for:
  - `eligibility_rules.rule_value`
  - `meets.venue`
  - `file_exports.filename`
  - `announcements.deleted_at`
  - `data_export_requests.deleted_at`
- `db/migrations/20260323000300_fix_missing_columns_2.sql` adds follow-up drift fixes for additional repo-referenced columns.
- The remaining missing Step 31 guardrail is startup-time schema validation with an actionable error message.
- Invoice persistence already converts an empty due date string to `NULL` in the repository layer, but a migration is still useful to normalize legacy drifted DB state.

## Implementation Plan
1. Add a DB schema validation helper that checks repo-required columns using `information_schema.columns`.
2. Call the validator during router startup after the DB pool is established and log a clear error when drift is detected.
3. Add an invoice due-date normalization migration that coerces empty-string drift to `NULL` before the app reads the column as a date.
4. Add targeted tests for the schema validation helper.
5. Run focused Go tests and attempt `make db-up` if the environment supports it.

## Verification Target
- Focused Go test pass for the new schema validation logic.
- If a database is available: run migrations and confirm startup no longer reports Step 31 schema mismatches.
