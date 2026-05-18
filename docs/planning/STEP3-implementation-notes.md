# STEP 3 Implementation Notes

Date: 2026-03-15
Owner: Codex
Purpose: execute TODO.md section `3) Database foundation and migration pipeline`.

## Execution Checklist
- [x] Install and configure migration tool
- [x] Create initial migration: extensions and base settings
- [x] Create `users` table migration
- [x] Create `roles` table migration
- [x] Create `user_roles` table migration
- [x] Create `sessions` table migration
- [x] Create `mfa_factors` table migration
- [x] Add `org_id` to identity tables where needed
- [x] Add `created_at`/`updated_at` audit columns to all created tables
- [x] Add `created_by`/`updated_by` columns to all business tables
- [x] Add migration rollback scripts for created migrations
- [x] Add `make db-up` target
- [x] Add `make db-down` target
- [x] Add `make db-status` target

## Notes
- Goose is configured through the root `Makefile` and executed with `go run github.com/pressly/goose/v3/cmd/goose@v3.24.1` to avoid requiring a pre-installed local binary.
- Migration files are versioned under `db/migrations/` and include explicit `-- +goose Up` and `-- +goose Down` sections for rollback support.
- Initial schema uses UUID primary keys (`gen_random_uuid()`), audit timestamps (`created_at`, `updated_at`), and audit actor fields (`created_by`, `updated_by`) across STEP 3 identity tables.
- `org_id` is included on identity tables where org scoping/context is needed (`users`, `roles`, `user_roles`, `sessions`, `mfa_factors`), matching the future-proofing requirement.
- `set_updated_at()` trigger function is added in the base migration and attached to each created table to enforce consistent `updated_at` mutation behavior.
- Post-audit hardening migration `20260315000700_harden_user_roles_uniqueness.sql` replaces the original `user_roles` uniqueness index with two partial unique indexes so duplicate assignments are rejected for both null-scope and non-null-scope records.
- Local runtime verification was completed against a running Postgres.app instance on `127.0.0.1:5432` with `make db-up` and `make db-status`.
- Migration `20260315000100_init_extensions.sql` uses Goose `StatementBegin/StatementEnd` for the PL/pgSQL function block to avoid dollar-quote parsing issues.

## Verification SQL (null-scope uniqueness)
Run this in a transaction after applying migrations:

```sql
BEGIN;
INSERT INTO user_roles (org_id, user_id, role_id, scope_type, scope_id)
VALUES
  ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333', 'org', NULL);

-- This insert must fail with duplicate key violation.
INSERT INTO user_roles (org_id, user_id, role_id, scope_type, scope_id)
VALUES
  ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333', 'org', NULL);
ROLLBACK;
```
