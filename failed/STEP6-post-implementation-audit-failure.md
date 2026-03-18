# STEP 6 Post-Implementation Audit Failure

Date: 2026-03-15
Auditor: Codex session
Scope: STEP 6 (Membership and households) plus required planning/design docs

## Outcome
STEP 6 does **not** pass the required post-implementation gate. At least one of the required conditions is not well-met.

## Condition Check
1. No bugs/potential issues/edge cases: **FAIL**
2. No regressions in code quality: **PARTIAL / NOT WELL-MET**
3. No clear security issues: **FAIL**
4. Followed earlier directions and completed fully: **PARTIAL / NOT WELL-MET**

## 1) What went wrong

### A. Cross-tenant integrity is not enforced at the DB level (security + data integrity)
- `contacts.org_id`, `swimmers.org_id`, and `swimmer_groups.org_id` are independent columns with FKs that do not enforce same-org parent linkage.
- Evidence:
  - `db/migrations/20260315001000_create_contacts.sql:4-5`
  - `db/migrations/20260315001100_create_swimmers.sql:4-5`
  - `db/migrations/20260315001200_create_swimmer_groups.sql:4-5`
- Why this matters: direct DB writes, backfills, or future repository bugs can create cross-org links (`contact.org_id != household.org_id`, etc.), violating tenant isolation assumptions.

### B. Parent household access model has an edge-case lockout
- Parent access is derived only from `parentHouseholdIndex` seeded when a parent creates a household.
- Evidence:
  - Index seeded only on create: `server/internal/app/members/service.go:223-228`
  - Parent enforcement depends on this index: `server/internal/app/members/service.go:602-613`
- Why this matters: if a parent is associated to an existing household via import/admin assignment (common real workflow), they can be incorrectly denied access.

### C. Uniqueness for swimmer external IDs is case-sensitive
- Unique index is on raw `TEXT` without normalization.
- Evidence:
  - `db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql:2-4`
- Why this matters: IDs like `ABC123` and `abc123` can coexist if app-level normalization is bypassed, causing interoperability ambiguity.

### D. Test coverage does not exercise key failure edges from A/B
- Membership tests cover happy path, unauthorized access, and medical redaction, but not cross-tenant DB constraints or parent-association edge cases.
- Evidence: `server/internal/app/server_members_test.go` (only 3 tests; no DB constraint tests, no parent assignment import/admin scenarios).

## 2) How to fix

### A. Enforce org-consistent foreign keys at schema level
- Add composite unique keys and composite FKs:
  - `households (org_id, id)` unique
  - `swimmers (org_id, id)` unique
  - `contacts (org_id, household_id)` FK -> `households(org_id, id)`
  - `swimmers (org_id, household_id)` FK -> `households(org_id, id)`
  - `swimmer_groups (org_id, swimmer_id)` FK -> `swimmers(org_id, id)`

### B. Replace/augment in-memory parent ownership mapping
- Add explicit membership linkage (e.g., `parent_households` mapping in service for now, DB table later).
- Add service methods to associate parent->household without requiring household creation by parent.
- Update access checks to use explicit membership source-of-truth.

### C. Make swimmer external ID uniqueness case-insensitive
- Either convert column to `CITEXT` or add functional unique index on `lower(swimmer_external_id)` with same partial predicate.

### D. Expand tests
- Add tests for:
  - Parent associated to pre-existing household can view/update own swimmers.
  - Parent cannot access unrelated household even with same org.
  - Duplicate external ID differing only by case is rejected.
  - DB-level org mismatch inserts are blocked (integration migration test).

## 3) What counts as passing
A passing re-audit requires all items below:

1. Schema prevents cross-org linkage for contacts/swimmers/swimmer_groups by constraint, not only handler/service logic.
2. Parent household access works for both newly created and pre-associated households.
3. External ID uniqueness is case-insensitive and covered by tests.
4. Tests include negative and edge scenarios for tenant boundaries and parent scope mapping.
5. Full `go test ./... -count=1` remains green after fixes.

## 4) Master list of files edited by Codex in STEP 6

1. `db/migrations/20260315000900_create_households.sql`
2. `db/migrations/20260315001000_create_contacts.sql`
3. `db/migrations/20260315001100_create_swimmers.sql`
4. `db/migrations/20260315001200_create_swimmer_groups.sql`
5. `db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql`
6. `server/internal/app/members/service.go`
7. `server/internal/app/handlers/members.go`
8. `server/internal/app/server_members_test.go`
9. `server/internal/app/server.go`
10. `TODO.md`
11. `CLAUDE.md`
12. `docs/planning/STEP6-execution-plan.md`
13. `docs/planning/STEP6-implementation-notes.md`
