# STEP 23 Test Strategy Execution Plan

## Goal

Close the testing gaps called out in `TODO.md` Step 23 by introducing reusable harnesses and then landing the missing test classes incrementally without duplicating setup across packages.

## Current Baseline

- `make test` and `make lint` are already executable and run real quality checks.
- The repo already has substantial HTTP/handler coverage across auth, members, meets, imports, exports, billing, reports, performance, communications, and security.
- The first unchecked Step 23 item is `Add unit test harness for Go services`.
- There is not yet a shared Go test helper package for service-level tests.
- There is not yet a shared integration harness with a real test DB.
- `internal/meetfiles/testdata` exists, but Step 23 still calls for additional fixture and golden-file organization.

## Step 23 Delivery Order

1. Add a shared Go unit test harness for service packages.
2. Use that harness in at least one service test file so the abstraction is exercised immediately.
3. Update tracking docs (`TODO.md`, `CLAUDE.md`) once the harness lands.
4. Reassess the next Step 23 item and continue if time remains.

## Unit Harness Requirements

- Keep setup lightweight and in-process.
- Support deterministic principals for common roles.
- Reduce repeated boilerplate for service construction and assertions.
- Avoid coupling the harness to a single domain package.
- Stay compatible with the current in-memory service architecture.

## Initial Implementation Scope

- Create `server/internal/testutil` for reusable testing helpers.
- Add principal builders for owner/admin/coach/parent/swimmer/billing/read-only roles.
- Add small assertion helpers for error and time checks used across service tests.
- Add a new direct service-level test file for `members.Service` using the harness.

## Follow-On Candidates

- Integration harness with ephemeral Postgres/`DATABASE_URL`-driven setup.
- OpenAPI contract test generation from examples.
- Golden-file coverage for SD3 exports and import reports.
- HY3 API-level finalization/report coverage.
- Broader invariant tests for billing ledger and RBAC matrix.
