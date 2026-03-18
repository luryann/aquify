# STEP 12 Audit Failure Report

All four audit conditions were checked. **Three out of four conditions are not fully met.**

---

## Condition Verdicts

| # | Condition | Result |
|---|---|---|
| 1 | No bugs, potential issues, or edge cases | **FAILED** |
| 2 | No regressions in code quality | **FAILED** |
| 3 | No clear code security issues | **FAILED** |
| 4 | All directions followed and fully completed | **FAILED** |

---

## Condition 1: Bugs / Edge Cases — FAILED

### Bug 1 — Relay strokes trigger best-time recompute that would violate DB CHECK constraint in production

**File:** `server/internal/app/performance/service.go`, line 363–365
**File:** `db/migrations/20260315003200_create_best_times.sql`, line 25

`UpsertResult` calls `recomputeBestTimeLocked` for any official, non-DQ result with a finish time, including relay events. The `best_times` table's stroke CHECK constraint only allows individual strokes:

```sql
CONSTRAINT best_times_stroke_check CHECK (stroke IN ('free','back','breast','fly','im'))
```

But `results` allows `free_relay` and `medley_relay`. When a relay result is upserted with `stroke='free_relay'` and `is_official=true`, `recomputeBestTimeLocked` will create/update a `BestTime` record with `stroke='free_relay'`, which would fail the DB constraint in production. The in-memory stub silently masks this.

**How to fix:** Add a relay-stroke guard in `UpsertResult` before calling `recomputeBestTimeLocked`:

```go
// in service.go line 362, replace:
if result.IsOfficial && !result.DQ && result.FinTimeMs != nil {
    s.recomputeBestTimeLocked(orgID, actorID, input.SwimmerID, input.Distance, input.Stroke, input.Course)
}

// with:
isRelayStroke := input.Stroke == "free_relay" || input.Stroke == "medley_relay"
if result.IsOfficial && !result.DQ && result.FinTimeMs != nil && !isRelayStroke {
    s.recomputeBestTimeLocked(orgID, actorID, input.SwimmerID, input.Distance, input.Stroke, input.Course)
}
```

**What counts as passing:** Relay stroke results never trigger best-time recompute; a test case verifying this behavior.

---

### Bug 2 — `results.swimmer_id` is nullable, contradicting the plan spec and service assumptions

**File:** `db/migrations/20260315003000_create_results.sql`, line 5

```sql
swimmer_id  UUID REFERENCES swimmers(id) ON DELETE SET NULL,
```

`swimmer_id` has no `NOT NULL` constraint. The plan states `swimmer_id (FK swimmers)` without indicating nullability. The `best_times` table enforces `swimmer_id UUID NOT NULL`. A result with `swimmer_id = NULL` could be inserted directly to the DB (bypassing HTTP validation), creating orphaned records that are silently ignored by `recomputeBestTimeLocked` but never cleaned up.

**How to fix:** Change the column definition to:

```sql
swimmer_id  UUID NOT NULL REFERENCES swimmers(id) ON DELETE RESTRICT,
```

Using `ON DELETE RESTRICT` (not `SET NULL`) to preserve data integrity — a result should never silently lose its swimmer association. This requires a new migration since the column definition must change.

**What counts as passing:** `swimmer_id NOT NULL` enforced at the DB level.

---

### Bug 3 — `RecordedAt` is never populated on auto-computed best times

**File:** `server/internal/app/performance/service.go`, lines 424–439

When `recomputeBestTimeLocked` creates a new `BestTime` record from a result, `RecordedAt` is never set. The `best_times` schema documents this field as "date/time of the swim that set this BT." For auto-computed best times, this field should be populated from the result's `CreatedAt`. Similarly, when an existing BT is updated to a new best time (lines 415–421), `RecordedAt` is not updated. The field is only populated for manual overrides, making it useless for the automatic code path.

**How to fix:** In `recomputeBestTimeLocked`, when updating an existing BT:

```go
existing.BestTimeMs = best.FinTimeMs
existing.ResultID = best.ID
existing.RecordedAt = &best.CreatedAt  // add this
existing.UpdatedAt = now
existing.UpdatedBy = actorID
```

And when creating a new BT record (line 424+), add:

```go
RecordedAt: &best.CreatedAt,
```

**What counts as passing:** Auto-computed best times have `RecordedAt` set to the swim's timestamp.

---

## Condition 2: No Regressions in Code Quality — FAILED

### Regression 1 — HTTP status code inconsistency: `ErrOverrideRequiresReason` returns 400 instead of 422

**File:** `server/internal/app/handlers/performance.go`, line 254

```go
if errors.Is(err, performance.ErrOverrideRequiresReason) {
    httpx.WriteError(w, r, http.StatusBadRequest, "override_requires_reason", err.Error(), nil)
    return
}
```

The rest of the codebase (across all handler files) uses `http.StatusUnprocessableEntity` (422) for semantic/business validation failures. 400 is reserved for malformed requests (invalid JSON, missing required fields). A whitespace-only reason string (`"   "`) passes the handler's `validate:"required"` check (go-playground/validator `required` accepts non-empty strings) and reaches the service, which returns `ErrOverrideRequiresReason`. The handler returns 400, but 422 is the correct status code by the project's convention.

**How to fix:** Change `http.StatusBadRequest` to `http.StatusUnprocessableEntity` at line 254.

**What counts as passing:** `ErrOverrideRequiresReason` returns 422, consistent with all other semantic validation error paths in the codebase.

---

### Regression 2 — O(n) full map scan in `recomputeBestTimeLocked` while holding write lock

**File:** `server/internal/app/performance/service.go`, lines 392–398

```go
var existing *BestTime
for _, bt := range s.bestTimes {
    if btKey(bt.OrgID, bt.SwimmerID, bt.EventDistance, bt.Stroke, bt.Course) == key {
        existing = bt
        break
    }
}
```

Every result upsert performs a full linear scan of `s.bestTimes` (and `s.results`) while holding an exclusive write lock, blocking all concurrent reads. Every other service in this codebase (meets, groups, imports, exports) uses direct map ID lookups. A secondary index map `map[string]*BestTime` keyed by the composite key (as `btKey` already computes) would make this O(1).

This is an in-memory stub, so it won't cause production failures, but it is a regression from the patterns established in all other service implementations and will cause noticeable slowdowns at modest data sizes.

**How to fix:** Add a secondary index to the `Service` struct:

```go
type Service struct {
    mu         sync.RWMutex
    results    map[string]*Result
    splits     map[string][]*Split
    bestTimes  map[string]*BestTime       // id → BestTime
    btByKey    map[string]*BestTime       // composite key → BestTime (secondary index)
    standards  map[string]*TimeStandard
    auditEvents []*BestTimeAuditEvent
}
```

Maintain `btByKey` in sync with `bestTimes` in `recomputeBestTimeLocked` and `OverrideBestTime`.

**What counts as passing:** `recomputeBestTimeLocked` uses an O(1) lookup instead of a full map scan.

---

## Condition 3: No Clear Code Security Issues — FAILED

### Security Issue — Read endpoints have no role or household scoping for `parent` role

**File:** `server/internal/app/handlers/performance.go`, lines 200–224, 267–299, 303–326

The STEP 12 plan explicitly states:
```
## Role Checks
- Read endpoints: owner, admin, coach, parent (own household)
```

The three read endpoints (`listBestTimes`, `listProgression`, `evaluateCutQualification`) currently have **zero role enforcement**. Any authenticated user in the org (including `billing` role, or a `parent` viewing another household's swimmer) can read any swimmer's best times and progression data.

The `parent (own household)` clause is a data isolation requirement: a parent may only read data for swimmers in their own household. This is not enforced at all.

**How to fix:**
1. For write endpoints: the existing `requireAdminOrCoach` check is correct.
2. For read endpoints: add a role check that allows `owner`, `admin`, `coach`, and `parent` roles. For `parent` role, additionally verify that the requested `swimmerID` belongs to a swimmer in the principal's household. Since this is an in-memory stub without a household membership lookup, the household check can be documented as a TODO but the role allowlist check must be present.

At minimum, add a `requireReadAccess` helper to the service:

```go
func (s *Service) requireReadAccess(principal *appmiddleware.Principal) error {
    for _, r := range principal.Roles {
        switch strings.ToLower(r) {
        case "admin", "owner", "coach", "parent":
            return nil
        }
    }
    return ErrForbidden
}
```

And call it at the start of `ListBestTimes`, `ListProgression`, and `EvaluateCutQualification`.

**What counts as passing:** Read endpoints return 403 for roles not in {owner, admin, coach, parent}; the household scoping requirement is documented as a TODO with a clear note of which roles are restricted.

---

## Condition 4: All Directions Followed and Fully Completed — FAILED

### Gap 1 — `UpdateFutureEntrySeedTimes` is a no-op and is never called

**File:** `server/internal/app/performance/service.go`, lines 462–468

The plan marks `[x] Service: update future entry seed times from results import` as complete. The implementation:

```go
func (s *Service) UpdateFutureEntrySeedTimes(_ string, _ *appmiddleware.Principal, _ string) error {
    // TODO(production): publish Asynq task "update_seed_times" with swimmerID.
    return nil
}
```

This function is never called from `UpsertResult`. There is no code path that invokes it. The plan checkbox is incorrect; the function exists in name only and has no behavior.

**How to fix:** Either:
1. Call `UpdateFutureEntrySeedTimes` from `UpsertResult` after a successful best-time recompute (the stub still does nothing, but the call chain is wired), and update the plan checkbox to note "stub wired, Asynq enqueue deferred to production," OR
2. Mark the checkbox as `[ ]` and document it as deferred.

**What counts as passing:** `UpsertResult` calls `UpdateFutureEntrySeedTimes` as part of the results upsert flow, and the function is documented as a stub pending Asynq integration.

---

### Gap 2 — `result.upserted` audit event is not emitted

**File:** `server/internal/app/performance/service.go`, `UpsertResult` function

The plan specifies:
```
## Audit Events
- `result.upserted` — actor, swimmer_id, result details
- `best_time.manual_override` — actor, swimmer_id, before/after values, reason
```

Only `best_time.manual_override` is emitted (in `OverrideBestTime`). No audit event is emitted for result upserts. The plan checkbox `[x] Audit event for manual best-time override` is only partially satisfied; the `result.upserted` event is entirely absent.

**How to fix:** At the end of `UpsertResult`, append a `BestTimeAuditEvent` with `Action: "result.upserted"` and relevant meta fields (swimmer_id, distance, stroke, course, fin_time_ms, is_official).

**What counts as passing:** `UpsertResult` emits a `result.upserted` audit event that is appended to `s.auditEvents`.

---

## Master List of Files Edited in STEP 12

| File | Change Type |
|---|---|
| `docs/planning/STEP12-implementation-plan.md` | Created |
| `db/migrations/20260315003000_create_results.sql` | Created |
| `db/migrations/20260315003100_create_splits.sql` | Created |
| `db/migrations/20260315003200_create_best_times.sql` | Created |
| `db/migrations/20260315003300_create_time_standards.sql` | Created |
| `server/internal/app/performance/service.go` | Created |
| `server/internal/app/handlers/performance.go` | Created |
| `server/internal/app/server.go` | Modified (wiring) |
| `server/internal/app/server_performance_test.go` | Created |
| `CLAUDE.md` | Modified (STEP 12 entries added) |
