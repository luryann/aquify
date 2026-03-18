# STEP 6 Implementation Notes

Date: 2026-03-15  
Owner: Codex session

Purpose: execute `TODO.md` section `6) Membership and households`.

## Checklist

- [x] Create `households` table migration.
- [x] Create `contacts` table migration.
- [x] Create `swimmers` table migration.
- [x] Create `swimmer_groups` table migration.
- [x] Add DB constraints for swimmer uniqueness per org.
- [x] Add API endpoint: create household.
- [x] Add API endpoint: update household.
- [x] Add API endpoint: add contact.
- [x] Add API endpoint: create swimmer profile.
- [x] Add API endpoint: update swimmer profile.
- [x] Add API endpoint: assign swimmer to group.
- [x] Add API endpoint: list swimmers with filters.
- [x] Add medical flag visibility enforcement in service layer.
- [x] Add audit events for swimmer profile edits.

## Implementation Strategy

1. Add schema-first migrations for membership domain tables while preserving existing table conventions (`org_id`, audit fields, and `set_updated_at` triggers).
2. Introduce an in-memory membership service (`server/internal/app/members`) to keep STEP 6 endpoint behavior unblocked by pending sqlc/repository work in later steps.
3. Enforce tenant scope invariants in handlers:
   - `principal.org_id == X-Org-Id`
   - body `org_id == X-Org-Id` for create flows.
4. Enforce parent household boundaries in service methods by tracking parent-created households and denying cross-household access.
5. Enforce medical flag visibility in the service list operation with role-aware redaction:
   - owner/admin/coach: visible
   - parent: own household only
   - billing/read-only: redacted
6. Emit explicit swimmer audit events for profile create/update responses (`swimmer.profile_created`, `swimmer.profile_updated`).
7. Add endpoint tests for deny-by-default coverage, medical redaction behavior, and swimmer profile audit event emission.

## Files Added

1. `db/migrations/20260315000900_create_households.sql`
2. `db/migrations/20260315001000_create_contacts.sql`
3. `db/migrations/20260315001100_create_swimmers.sql`
4. `db/migrations/20260315001200_create_swimmer_groups.sql`
5. `db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql`
6. `server/internal/app/members/service.go`
7. `server/internal/app/handlers/members.go`
8. `server/internal/app/server_members_test.go`
9. `docs/planning/STEP6-execution-plan.md`

## Files Updated

1. `server/internal/app/server.go`
2. `TODO.md`
3. `CLAUDE.md`

## Verification

- `GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache go test ./...` (run from `server/`)
  - Result: pass.
