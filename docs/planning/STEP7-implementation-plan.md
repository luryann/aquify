# STEP 7 Implementation Plan — Groups, Eligibility, and Registrations

Date: 2026-03-15
Status: In Progress

## Scope (from TODO.md section 7)

- [ ] Create `groups` table migration
- [ ] Create `eligibility_rules` table migration
- [ ] Create `registrations` table migration
- [ ] Implement endpoint: create group
- [ ] Implement endpoint: update group
- [ ] Implement endpoint: attach eligibility rule
- [ ] Implement endpoint: evaluate swimmer eligibility
- [ ] Implement endpoint: create registration
- [ ] Implement endpoint: registration status transition
- [ ] Add registration waiver acceptance record table
- [ ] Add endpoint: accept waiver
- [ ] Add endpoint test: group-scoped coach visibility

## Design Decisions

### In-Memory Service Pattern
Following STEP 6's pattern: in-memory service with maps, mutex protection, no DB yet.

### Groups Table
Formalizes group definitions. `swimmer_groups` table (from STEP 6) becomes the swimmer↔group assignment link using `group_id` reference instead of only free-text `group_code`.

> Note: `swimmer_groups` used `group_code` (text) as a denormalized ref. New `groups` table is the canonical definition. Backward-compatible — existing swimmer_groups rows reference group codes, new registrations reference group IDs. The groups service validates `group_id` references internally.

### Eligibility Rules
- Rule types: `age_range`, `gender`, `registration_status`
- Parameters: JSON object specific to rule type:
  - `age_range`: `{"min_age": 6, "max_age": 18}`
  - `gender`: `{"allowed": ["female","male","non_binary","undisclosed"]}`
  - `registration_status`: `{"allowed": ["active","pending"]}`

### Registration Status State Machine
```
pending -> active     (admin/coach)
pending -> withdrawn  (parent/admin/coach)
active  -> inactive   (admin/coach)
active  -> withdrawn  (admin/coach/parent)
inactive -> active   (admin/coach)
inactive -> withdrawn (admin/coach)
```

### Group-Scoped Coach Visibility
- Coaches can view any group (operations need)
- Coaches can only list/see swimmers associated with groups where `coach_user_id` matches their user ID
- Test asserts a coach only sees groups where they are the assigned coach when filtered

### RBAC
- Create/Update group: owner, admin, coach
- Attach eligibility rule: owner, admin
- Evaluate eligibility: owner, admin, coach, parent (own household)
- Create registration: owner, admin, coach, parent
- Transition registration status: owner, admin, coach (parent can only withdraw)
- Accept waiver: parent (own household), owner, admin

## Files to Create

1. `db/migrations/20260315001400_create_groups.sql`
2. `db/migrations/20260315001500_create_eligibility_rules.sql`
3. `db/migrations/20260315001600_create_registrations.sql`
4. `db/migrations/20260315001700_create_waiver_acceptances.sql`
5. `server/internal/app/groups/service.go`
6. `server/internal/app/handlers/groups.go`
7. `server/internal/app/server_groups_test.go`

## Files to Update

1. `server/internal/app/server.go` — register `/groups` and `/registrations` routes
2. `TODO.md` — mark STEP 7 items complete
3. `CLAUDE.md` — update status
4. This file (progress tracking)
