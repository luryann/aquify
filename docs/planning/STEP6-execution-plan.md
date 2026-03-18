# STEP 6 Execution Plan

Date: 2026-03-15  
Owner: Codex session

Goal: complete `TODO.md` section `6) Membership and households`.

## Progress Tracker

- [x] Read required planning/design docs and identify unfinished step.
- [x] Add database migrations for membership domain tables and constraints.
- [x] Implement membership service layer for households, contacts, swimmers, and swimmer-group assignment.
- [x] Implement `/api/v1/members` endpoints for STEP 6 API checklist.
- [x] Enforce medical-flag visibility rules in service responses.
- [x] Emit audit events for swimmer profile edits.
- [x] Add/extend automated tests for authz and scope behavior in membership endpoints.
- [x] Run test suite and verify no regressions.
- [x] Update tracking docs (`TODO.md`, `CLAUDE.md`, `docs/planning/STEP6-implementation-notes.md`).

## Working Notes

- Existing backend uses an in-memory service pattern for early steps; STEP 6 follows that pattern for API behavior while still adding SQL migrations for schema groundwork.
- `groups` table is planned in STEP 7, so `swimmer_groups` stores a `group_code` token for now to avoid out-of-order FK dependencies.
- Medical visibility baseline:
  - `owner/admin/coach`: full medical visibility.
  - `parent`: own-household swimmers only.
  - others: redacted medical fields.
