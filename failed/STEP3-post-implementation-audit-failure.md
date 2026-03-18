# STEP 3 Post-Implementation Audit Failure

Date: 2026-03-15
Owner: Codex
Scope reviewed:
- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.txt`
- `docs/planning/Plan.txt`
- `CLAUDE.md`
- `TODO.md`
- All STEP 3 edits: `Makefile`, `README.md`, `docs/planning/STEP3-implementation-notes.md`, and `db/migrations/20260315000100` to `20260315000600`

## Result
The implementation does **not** currently satisfy all required acceptance conditions at a strong level.

## 1) What went wrong

### A. Correctness bug in `user_roles` uniqueness
File: `db/migrations/20260315000400_create_user_roles.sql`

The unique index:

`ON user_roles (org_id, user_id, role_id, scope_type, scope_id)`

does not prevent duplicates when `scope_id` is `NULL` (PostgreSQL treats `NULL` values as distinct in unique indexes).  
Impact: duplicate effective role assignments can be inserted for org-scoped roles, which can break authorization assumptions and create inconsistent RBAC behavior.

### B. Verification gap (runtime migration execution not validated)
`make db-status`/Goose runtime could not be executed in this environment because `go` is missing (`command not found: go`).  
Impact: cannot claim high confidence that migration chain applies/rolls back cleanly in practice.

## 2) How to fix it

### A. Fix `user_roles` uniqueness semantics
Choose one of the following robust approaches:

1. Preferred: use PostgreSQL 15+ `UNIQUE NULLS NOT DISTINCT` on `(org_id, user_id, role_id, scope_type, scope_id)`.
2. Portable alternative: replace with functional unique index:
`UNIQUE (org_id, user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid))`
3. Alternative: split into two partial unique indexes:
- unique for `scope_id IS NULL`
- unique for `scope_id IS NOT NULL`

Also add an insert test proving duplicate null-scoped assignments are rejected.

### B. Close runtime verification gap
In a Go-enabled environment:
1. `make infra-up`
2. `make db-up`
3. `make db-status`
4. `make db-down` (one step) and re-run `make db-status`
5. Optional: migrate to zero and back up to confirm full reversibility.

Record command outputs in a verification note or CI log.

## 3) What counts as passing

All of the following must be true:

1. Duplicate `user_roles` entries with `scope_id = NULL` are rejected by DB constraints.
2. Migration chain executes successfully end-to-end (`up`, `status`, `down`) with no SQL/runtime errors.
3. STEP 3 docs remain consistent (`TODO.md`, `README.md`, `CLAUDE.md`, `docs/planning/STEP3-implementation-notes.md`).
4. No new security concerns are introduced by the migration changes (at minimum: constraints and indexes enforce intended RBAC invariants).
