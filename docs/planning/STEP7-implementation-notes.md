# STEP 7 Implementation Notes

Date: 2026-03-15
Owner: Codex session

Purpose: execute `TODO.md` section `7) Groups, eligibility, and registrations`.

## Checklist

- [x] Create `groups` table migration.
- [x] Create `eligibility_rules` table migration.
- [x] Create `registrations` table migration.
- [x] Implement endpoint: create group.
- [x] Implement endpoint: update group.
- [x] Implement endpoint: attach eligibility rule.
- [x] Implement endpoint: evaluate swimmer eligibility.
- [x] Implement endpoint: create registration.
- [x] Implement endpoint: registration status transition.
- [x] Add registration waiver acceptance record table.
- [x] Add endpoint: accept waiver.
- [x] Add endpoint test: group-scoped coach visibility.

## Implementation Strategy

1. **DB migrations (schema-first):** Four new migrations following the existing timestamp sequence and conventions (`org_id`, audit fields, `set_updated_at` triggers, soft deletes where appropriate).
   - `20260315001400_create_groups.sql` — canonical group definitions with `(org_id, code)` unique constraint.
   - `20260315001500_create_eligibility_rules.sql` — rules linked to groups via FK, `rule_type` + JSONB `parameters`.
   - `20260315001600_create_registrations.sql` — swimmer↔group↔season registration with composite unique constraint and FK to `swimmers`/`households`/`groups`.
   - `20260315001700_create_waiver_acceptances.sql` — append-only waiver records with `(registration_id, waiver_type)` unique constraint for idempotency.

2. **In-memory service (`server/internal/app/groups/service.go`):** Follows STEP 6 pattern exactly — maps with mutex, no DB dependency yet.
   - Groups: create (owner/admin/coach), update (owner/admin/coach), get/list (all authenticated; coaches see only their groups).
   - Eligibility rules: attach (owner/admin only), evaluate (owner/admin/coach/parent own household).
   - Registrations: create (all authenticated; parent scoped to own household), transition via state machine, waiver acceptance.

3. **Group-scoped coach visibility:** Coaches without owner/admin role can only list groups where `coach_user_id` matches their own `UserID`. Admins see all groups. Covered by `TestGroupScopedCoachVisibility`.

4. **Registration state machine:**
   ```
   pending  -> active, withdrawn
   active   -> inactive, withdrawn
   inactive -> active, withdrawn
   ```
   Parents may only transition to `withdrawn`. Any other transition by a parent returns 403.

5. **Eligibility rule evaluation:** Three rule types — `age_range` (computes age from DOB), `gender` (allowed list), `registration_status` (allowed list). Rules are evaluated in aggregate — all must pass for `eligible: true`.

6. **Waiver idempotency:** One acceptance per `(registration_id, waiver_type)` — duplicate returns 409 Conflict.

7. **Handler wiring:** `GroupsHandlers.RegisterRoutes()` mounts groups + registrations routes directly on the protected router (not under `/members`).

## Files Added

1. `db/migrations/20260315001400_create_groups.sql`
2. `db/migrations/20260315001500_create_eligibility_rules.sql`
3. `db/migrations/20260315001600_create_registrations.sql`
4. `db/migrations/20260315001700_create_waiver_acceptances.sql`
5. `server/internal/app/groups/service.go`
6. `server/internal/app/handlers/groups.go`
7. `server/internal/app/server_groups_test.go`
8. `docs/planning/STEP7-implementation-plan.md` (planning artifact)

## Files Updated

1. `server/internal/app/server.go` — add `groupsService` param + wire `groupsHandlers.RegisterRoutes`
2. `server/internal/app/server_members_test.go` — updated `NewRouterWithServices` call (added nil 5th arg)
3. `TODO.md` — mark STEP 7 items complete
4. `CLAUDE.md` — update project status and artifact list

## Verification

- `GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache go test ./...` (run from `server/`)
  - Result: all pass.
- `go vet ./...` — no issues.
