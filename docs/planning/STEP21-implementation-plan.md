# STEP 21 Implementation Plan

## Goal
Implement the core v1 reporting pack backend for membership status, meet fees, invoice aging, payment reconciliation, and registration status reports, including CSV export support and audit logging.

## Working Assumptions
- Existing admin `/reports` UI from Step 20B is a shell and does not yet call real report APIs.
- The backend uses in-memory domain services for application state, so Step 21 should compose those services rather than introduce a database-backed reporting layer.
- "Define SQL/view" in `TODO.md` will be satisfied by defining stable report row projections and view-like aggregation logic in the application layer, because the current runtime is service-backed and no report SQL/view infrastructure exists yet.

## Deliverables
- New `reports` application service with deterministic report builders.
- Endpoints for:
  - membership status report
  - meet fees report
  - invoice aging report
  - payment reconciliation report
  - registration status report
- CSV formatter utility shared by report endpoints.
- Audit event emission for report generation/export.
- Router wiring and tests covering role/scope behavior and CSV output.
- `CLAUDE.md` update with Step 21 status and report surface summary.

## Implementation Strategy
1. Inspect existing service APIs and data types in `members`, `groups`, `meets`, and `billing`.
2. Add a new `server/internal/app/reports` package that depends on those services.
3. Model each report with:
   - row struct
   - filter/query input
   - aggregate totals metadata where useful
4. Add a CSV encoder utility that accepts headers + row maps/struct projections.
5. Add report handlers under `/api/v1/reports/...`.
6. Return JSON by default and CSV when `format=csv`.
7. Append audit events with action names per report run.
8. Add server tests covering successful generation, CSV responses, and access control.

## Notes
- If existing services do not expose enough read methods, add small read-only accessors rather than reaching into internal fields from handlers.
- Keep report outputs stable and explicit so a later SQL-backed implementation can preserve the contract.
