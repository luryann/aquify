# STEP 8 Post-Implementation Audit Failure

**Date:** 2026-03-15
**Step:** STEP 8 — Meet Setup, Commitments, and Entries
**Verdict:** FAIL — Conditions 1 (bugs/edge cases), 3 (security), and 4 (completeness) are not well-met.
**Condition 2 (regressions):** PASS — No regressions from STEP 1–7 detected.

---

## Condition 1: No Bugs, Potential Issues, or Edge Cases — FAIL

### CRITICAL-1: `PublishCommitmentRequest` bypasses the validated state machine

**File:** `server/internal/app/meets/service.go`, lines 627–636
**What went wrong:** `PublishCommitmentRequest` directly checks `if meet.Status != "published" && meet.Status != "draft"` and then sets `meet.Status = "commitment_open"` without calling `validateMeetTransition`. This allows a `draft → commitment_open` transition that the state machine table (`meetTransitions`) explicitly does not permit. Only `published → commitment_open` is a legal transition per spec.
**How to fix:** Remove the `"draft"` case from the status guard in `PublishCommitmentRequest`. Then route the status update through `validateMeetTransition(meet.Status, "commitment_open")` before applying it. The commitment creation logic should remain in this function; only the status guard needs to be corrected and unified with the validator.
**What counts as passing:** `PublishCommitmentRequest` must reject calls when meet is in `draft` status, and the transition to `commitment_open` must be validated through the same `validateMeetTransition` path used by `TransitionMeetStatus`.

---

### CRITICAL-2: `LockCommitments` bypasses the validated state machine

**File:** `server/internal/app/meets/service.go`, lines 933–942
**What went wrong:** `LockCommitments` directly sets `meet.Status = "commitment_closed"` after only checking `if meet.Status != "commitment_open"`, bypassing `validateMeetTransition`. This creates a second parallel transition path inconsistent with the state machine table.
**How to fix:** Route the status change through `validateMeetTransition(meet.Status, "commitment_closed")` before applying it.
**What counts as passing:** `LockCommitments` must use `validateMeetTransition` for the status change, consistent with `TransitionMeetStatus`.

---

### CRITICAL-3: Entries can be proposed while meet is `commitment_open`

**File:** `server/internal/app/meets/service.go`, line 1015
**What went wrong:** The guard `if meet.Status != "commitment_closed" && meet.Status != "entry_finalized" && meet.Status != "commitment_open"` allows `ProposeEntry` to run while the meet is still in `commitment_open`. Per the spec and state machine, entries are a post-commitment-lock action — parents are still responding during `commitment_open`.
**How to fix:** Remove `"commitment_open"` from the allowed statuses in `ProposeEntry`. Only `"commitment_closed"` (and `"entry_finalized"` if re-proposal is needed) should be permitted.
**What counts as passing:** `ProposeEntry` must return an error (e.g., 409 Conflict) when the meet status is `commitment_open` or any pre-commitment-closed state.

---

### CRITICAL-4: `OverrideEntry` does not check for duplicate entry after changing `MeetEventID`

**File:** `server/internal/app/meets/service.go`, lines 1113–1122
**What went wrong:** When a coach overrides an entry with a new `MeetEventID`, the service does not verify that another entry for the same `(meet, swimmer, newMeetEventID)` tuple does not already exist. The DB uniqueness constraint (`entries_swimmer_event_unique`) will produce an opaque constraint violation when the real database is used. At the in-memory layer, it silently creates an inconsistent state.
**How to fix:** Before applying the override, check whether an entry with `SwimmerID == entry.SwimmerID && MeetEventID == req.NewMeetEventID && MeetID == meetID` already exists (excluding the entry being modified). If found, return a conflict error.
**What counts as passing:** `OverrideEntry` must return a clear 409 Conflict error if the new event assignment would create a duplicate entry for that swimmer in that meet.

---

### HIGH-1: `ListEntries` exposes all entries regardless of parent household

**File:** `server/internal/app/meets/service.go`, lines 971–991
**What went wrong:** `ListEntries` applies no household filtering when the caller is a parent. Any authenticated parent can see every swimmer's entries for any meet in the org, including other families' entries, seed times, and selection data. `ListCommitments` (line 703) correctly applies household scoping for parents — `ListEntries` does not.
**How to fix:** Add the same household scoping pattern used in `ListCommitments`: when `hasAnyRole(principal, "parent")` and the principal does not have `owner`/`admin`/`coach`, filter `result` to entries where the swimmer's household matches the principal's household.
**What counts as passing:** A parent principal must only receive entries belonging to swimmers in their own household. Coaches/admins/owners see all entries unfiltered.

---

### HIGH-2: `OverrideEntry` has no meet-status guard

**File:** `server/internal/app/meets/service.go`, lines 1072–1142
**What went wrong:** `OverrideEntry` only checks the entry's status (`submitted` or `draft`) but never checks the meet's current lifecycle state. A coach can call this endpoint when the meet is `completed` or `archived`, modifying historical records.
**How to fix:** Add a meet-status guard at the start of `OverrideEntry` that rejects the call (409 or 422) if `meet.Status` is `entry_finalized`, `completed`, or `archived`.
**What counts as passing:** `OverrideEntry` returns an error for any meet in a finalized or post-finalized lifecycle state.

---

### HIGH-3: `ApproveEntry` has no meet-status guard

**File:** `server/internal/app/meets/service.go`, lines 1145–1188
**What went wrong:** Same class of issue as HIGH-2. `ApproveEntry` only checks `entry.Status != "submitted"` but does not validate that the meet is in an appropriate state for approval.
**How to fix:** Add a meet-status guard rejecting approval if the meet is in `commitment_open`, `draft`, `completed`, or `archived` states.
**What counts as passing:** `ApproveEntry` only proceeds when the meet is in `commitment_closed` or `entry_finalized` status.

---

### HIGH-4: Parent can flip commitment response leaving orphaned event selections

**File:** `server/internal/app/meets/service.go`, lines 748–759
**What went wrong:** A parent who responded `committed_yes` (creating event selections) can call `RespondToCommitment` again with `committed_no`. The commitment status changes but the existing event selections are not deleted. This leaves orphaned selections in `commitment_event_selections` that may be included in coach reviews.
**How to fix:** When `RespondToCommitment` processes a `committed_no` response, delete any existing `commitment_event_selections` for that commitment before saving the status change.
**What counts as passing:** When a parent switches to `committed_no`, all associated event selections for that commitment are removed atomically.

---

### HIGH-5: No format validation on URL path parameters

**File:** `server/internal/app/handlers/meets.go`, multiple handler functions
**What went wrong:** URL parameters (`meetID`, `commitmentID`, `entryID`) are trimmed but not validated to be non-empty or in the expected ID format. While the current in-memory service uses safe map lookups, this is a defensive gap.
**How to fix:** Add a `validateID(v string) bool` helper that checks `len(v) > 0` and optionally validates UUID or expected token format. Call it at the top of each handler and return 400 if invalid.
**What counts as passing:** All handler functions reject empty or clearly invalid path parameter values with a 400 Bad Request before passing to the service layer.

---

## Condition 2: No Regressions — PASS

All pre-STEP 8 routes (`/auth/*`, `/members/*`, `/groups/*`, `/health/*`, `/openapi`) are unchanged. The STEP 8 change to `server.go` was purely additive (`meetsHandlers.RegisterRoutes(protected)`). The `authn/service.go`, `members/service.go`, and `groups/service.go` files were not modified. Middleware chain is intact.

---

## Condition 3: No Clear Security Issues — FAIL

### SEC-1: `ListEntries` exposes cross-household data to parents (see HIGH-1 above)

This is a privacy and authorization violation — a parent can enumerate entries for swimmer families they are not associated with.

### SEC-2: Coaches can respond to commitments on behalf of parents — RBAC violation

**File:** `server/internal/app/meets/service.go`, lines 715–769
**What went wrong:** The RBAC table in the spec marks "Commit yes/no" as `N` for the `coach` role. However, the service's `hasAnyRole` check at line 742 allows coaches to respond to any commitment without restriction (they skip the household filter):
```go
if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
```
A coach passes the outer role check (`owner`, `admin`, `coach` are all allowed to call the endpoint) and then bypasses the household guard.
**How to fix:** Add an explicit role check at the top of `RespondToCommitment` that returns 403 if the caller has only the `coach` role. Only `parent` (own household), `owner`, and `admin` should be permitted.
**What counts as passing:** A principal with only the `coach` role receives a 403 Forbidden when calling `RespondToCommitment`, consistent with the RBAC table.

---

## Condition 4: Directions Followed and Completed Fully — FAIL

### INCOMPLETENESS-1: No `GET /meets/{meetID}/sessions` endpoint

**What went wrong:** Sessions can be created (`POST /meets/{meetID}/sessions`) but cannot be retrieved. There is no list or get endpoint for meet sessions. Any client building a meet detail view would be unable to retrieve session data through the API.
**How to fix:** Add a `ListMeetSessions` method to the service and a corresponding `GET /meets/{meetID}/sessions` handler and route.
**What counts as passing:** `GET /meets/{meetID}/sessions` returns a paginated list of sessions for a given meet, accessible to `owner`, `admin`, `coach`, and `parent` roles.

### INCOMPLETENESS-2: STEP 8 tables missing `deleted_at`/`deleted_by` soft-delete columns

**What went wrong:** CLAUDE.md architectural convention states: "Soft deletes use `deleted_at`/`deleted_by`." None of the six STEP 8 migrations (`meets`, `meet_sessions`, `meet_events`, `commitments`, `entries`, `entry_legs`) include these columns.
**How to fix:** Create a new migration (`20260315002400_add_soft_delete_to_meets_tables.sql`) that adds `deleted_at TIMESTAMPTZ` and `deleted_by UUID REFERENCES users(id)` to all six tables, with `NULL` defaults (records are not deleted by default).
**What counts as passing:** All STEP 8 tables have `deleted_at` and `deleted_by` columns matching the convention used in STEP 3–7 tables.

### INCOMPLETENESS-3: `commitment_event_selections` missing `updated_at`/`updated_by`

**What went wrong:** CLAUDE.md states: "Every table has `id`, `created_at`, `updated_at`, `created_by`, `updated_by`." The `commitment_event_selections` table only has `created_at`/`created_by`.
**How to fix:** Add `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()` and `updated_by UUID REFERENCES users(id)` to the `commitment_event_selections` table in a new migration.
**What counts as passing:** `commitment_event_selections` has all five standard columns.

### INCOMPLETENESS-4: Missing index on `commitment_event_selections.meet_event_id`

**What went wrong:** Querying selections by event (e.g., for coach review) requires a full table scan. Only a `commitment_id` index is present.
**How to fix:** Add `CREATE INDEX commitment_event_selections_meet_event_id_idx ON commitment_event_selections (meet_event_id);` to the migration.
**What counts as passing:** A `meet_event_id` index exists on `commitment_event_selections`.

### MEDIUM: Two parallel transition paths for `commitment_open` create inconsistent state

**File:** `server/internal/app/meets/service.go`, lines 1207–1225 and 627–636
**What went wrong:** Both `POST /meets/{meetID}/transition` (with `{"status": "commitment_open"}`) and `POST /meets/{meetID}/commitment-request` can move a meet to `commitment_open`. The `/transition` path does not create commitment records; `/commitment-request` does. A coach using `/transition` leaves the meet in `commitment_open` with no commitments created.
**How to fix:** Remove `"commitment_open"` from the `meetTransitions` map so it cannot be reached via the generic `/transition` endpoint. `commitment_open` must only be reachable via `/commitment-request`. Similarly, `"commitment_closed"` should be removed from `meetTransitions` and only reachable via `/lock-commitments`.
**What counts as passing:** The `/transition` endpoint cannot move a meet to `commitment_open` or `commitment_closed`. Those transitions are exclusively controlled by their dedicated endpoints.

---

## Master File List — All Files Edited/Created in STEP 8

### Database Migrations
- `db/migrations/20260315001800_create_meets.sql`
- `db/migrations/20260315001900_create_meet_sessions.sql`
- `db/migrations/20260315002000_create_meet_events.sql`
- `db/migrations/20260315002100_create_commitments.sql`
- `db/migrations/20260315002200_create_entries.sql`
- `db/migrations/20260315002300_create_entry_legs.sql`

### Backend Service + Handlers
- `server/internal/app/meets/service.go`
- `server/internal/app/handlers/meets.go`

### Route Registration
- `server/internal/app/server.go`

### Tests
- `server/internal/app/server_meets_test.go`

### Planning Trace
- `docs/planning/STEP8-implementation-notes.md`

---

## What Passes (for reference when re-implementing)

- ✅ All 6 migrations present with correct foreign keys and indexes (excluding the missing soft-delete and `updated_at` issues)
- ✅ `TransitionMeetStatus` correctly uses `validateMeetTransition`
- ✅ Audit logging present for coach override with before/after diff and reason field
- ✅ Audit logging present for entry approval
- ✅ Commitment lock prevents further event selections
- ✅ `ListCommitments` correctly applies household scoping for parents
- ✅ No regressions in STEP 1–7 code
- ✅ State machine transitions for `draft → published → ...` are correctly enforced via the validator
- ✅ `DisallowUnknownFields` is applied to all request bodies via `decodeJSONBody`
