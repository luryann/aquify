# STEP 16 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** STEP 16 — Audit/Compliance Primitives
**Result:** FAIL — Multiple conditions not met

---

## Conditions Evaluated

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **FAIL** — Multiple issues (see below) |
| 2 | No regressions in code quality | **FAIL** — Missing tests violate established DoD pattern |
| 3 | No clear code security issues | **FAIL** — Parent household scope bypass; audit event immutability gap |
| 4 | All directions followed and completed fully | **FAIL** — TTL config unused, deletion review days unused, no test file |

---

## Issues Found

### CRITICAL-1: Parent Can Access Any Household's Compliance Data (Security Vulnerability)

**File:** `server/internal/app/handlers/compliance.go` lines ~79–82 and ~172–175

**What went wrong:**
`createDataExportRequest` and `createDataDeletionRequest` check only `hasAnyRole(principal, "owner", "admin", "parent")` but never verify that a `parent`-role caller is requesting their own household. Any parent can supply any `household_id` in the JSON body and successfully create a compliance request for a household they don't belong to — leaking that household's data or triggering deletion of another family's account.

**How to fix:**
Add a household ownership check in both handlers: if `principal.Role == "parent"`, confirm `req.HouseholdID == principal.HouseholdID` (or however the session-bound household is stored). If the `Principal` struct does not carry `HouseholdID`, add it during session issuance. Return `403 Forbidden` if the values do not match.

**What counts as passing:**
A parent with household ID X cannot create or trigger a compliance request for household ID Y. The check must be enforced in the handler before the service call.

---

### CRITICAL-2: `audit_events` Table Has No DB-Level Immutability

**File:** `db/migrations/20260316000100_create_audit_events.sql`

**What went wrong:**
The migration comment says "No updates or deletes are permitted" and CLAUDE.md says the table is "immutable append-only by design," but there is no PostgreSQL `RULE`, row-level security policy, or trigger that prevents `UPDATE` or `DELETE` on this table. Any DB user with table-level write access can silently alter or delete audit records, defeating the entire compliance use case.

**How to fix:**
Add the following to the migration (after the `CREATE TABLE`):

```sql
-- Prevent any updates to audit records
CREATE RULE audit_events_no_update AS ON UPDATE TO audit_events DO INSTEAD NOTHING;

-- Prevent any deletes from audit records
CREATE RULE audit_events_no_delete AS ON DELETE TO audit_events DO INSTEAD NOTHING;
```

Or, alternatively, use a trigger that raises an exception on any attempted UPDATE/DELETE:

```sql
CREATE OR REPLACE FUNCTION audit_events_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_events records are immutable';
END;
$$;

CREATE TRIGGER audit_events_no_update
  BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

CREATE TRIGGER audit_events_no_delete
  BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();
```

**What counts as passing:**
An `UPDATE` or `DELETE` statement against `audit_events` must fail at the database level, not just by application convention.

---

### CRITICAL-3: No Test File for STEP 16 (DoD Not Met)

**File:** `server/internal/app/` — no `server_audit_test.go` or `server_compliance_test.go` exists

**What went wrong:**
Every prior step has a corresponding test file (`server_billing_test.go`, `server_comms_test.go`, `server_meets_test.go`, etc.). STEP 16 has none. The Definition of Done in CLAUDE.md explicitly states "Code merged with tests." Key behaviors that are untested include:
- `GET /api/v1/audit-events` role gate (owner/admin only)
- Parent household scope enforcement on compliance POST endpoints
- Cursor pagination edge cases
- All 6 compliance CRUD endpoints

**How to fix:**
Create `server/internal/app/server_audit_test.go` and `server/internal/app/server_compliance_test.go` (or a combined `server_step16_test.go`) covering at minimum:
- Audit query with `owner` token → 200
- Audit query with `parent` token → 403
- Audit query with valid `from`/`to` filters → correct results
- Audit query with invalid `from` timestamp → 400 (once Issue #6 is fixed)
- Compliance export request by owner → 201
- Compliance export request by parent for own household → 201
- Compliance export request by parent for other household → 403
- Compliance deletion request by owner → 201
- Compliance GET by requester → 200
- Compliance GET by non-owner/non-requester → 403

**What counts as passing:**
A test file exists, runs via `make test`, covers the role-gated endpoints, and includes at least one test for the parent household scope enforcement.

---

### IMPORTANT-1: Malformed Time Filter Silently Ignored in Audit Query

**File:** `server/internal/app/handlers/audit.go` lines ~85–93

**What went wrong:**
When the caller passes a malformed `from` or `to` timestamp, `time.Parse` returns an error that is silently swallowed (`if err == nil` means the filter is just skipped). The response returns a larger result set than the caller intended with no error indication.

**How to fix:**
Change the pattern from:
```go
if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
    filter.From = t
}
```
to:
```go
if fromStr != "" {
    t, err := time.Parse(time.RFC3339, fromStr)
    if err != nil {
        http.Error(w, `{"error":"invalid 'from' timestamp, use RFC3339"}`, http.StatusBadRequest)
        return
    }
    filter.From = t
}
```
Apply the same fix to the `to` parameter.

**What counts as passing:**
A request with `?from=not-a-date` returns HTTP 400 with a clear error message.

---

### IMPORTANT-2: `DeletionReviewDays` Config Is Loaded but Never Passed to Compliance Service

**File:** `server/internal/app/config.go` and `server/internal/app/compliance/service.go`

**What went wrong:**
`ComplianceConfig.DeletionReviewDays` is declared and loaded from `COMPLIANCE_DELETION_REVIEW_DAYS` env var, but `compliance.NewServiceWithConfig` only accepts `exportTTLDays` (one argument). The deletion review window is never enforced.

**How to fix:**
Update `NewServiceWithConfig` signature to accept both values:
```go
func NewServiceWithConfig(exportTTLDays, deletionReviewDays int) *Service
```
Store `deletionReviewDays` in the struct and use it when setting `ExpiresAt` on newly created deletion requests (i.e., `ExpiresAt = now + deletionReviewDays`). Update the wiring in `server.go` to pass both config values.

**What counts as passing:**
A newly created deletion request has `ExpiresAt` set to `now + COMPLIANCE_DELETION_REVIEW_DAYS`. The config value is not silently discarded.

---

### IMPORTANT-3: `exportTTLDays` Is Stored but Never Used — `ExpiresAt` Always Nil

**File:** `server/internal/app/compliance/service.go` — `CreateDataExportRequest`

**What went wrong:**
`exportTTLDays` is stored in the service struct but never read when creating export requests. `DataExportRequest.ExpiresAt` is always `nil`, meaning all export requests are permanent. The spec, CLAUDE.md, and the retention config all describe an expiry mechanism.

**How to fix:**
In `CreateDataExportRequest`, set:
```go
expiresAt := time.Now().UTC().AddDate(0, 0, s.exportTTLDays)
req.ExpiresAt = &expiresAt
```

**What counts as passing:**
A newly created export request has `ExpiresAt` set to approximately `now + COMPLIANCE_EXPORT_TTL_DAYS`.

---

### IMPORTANT-4: Compliance GET Endpoints Leak Resource Existence to Unauthorized Callers

**File:** `server/internal/app/handlers/compliance.go` lines ~144–156 and ~237–246

**What went wrong:**
Both `getDataExportRequest` and `getDataDeletionRequest` fetch the resource first, then perform the authorization check. This means:
- If the ID does not exist → `404`
- If the ID exists but belongs to another household → `403`

An unauthorized caller can enumerate compliance request IDs and determine which ones exist (just by seeing `403` vs `404`), which leaks the existence of other households' compliance filings.

**How to fix:**
Return `404` in both the "not found" and "not authorized" cases for non-owner/non-admin callers. A parent who guesses another household's request ID should receive `404`, not `403`.

**What counts as passing:**
An unauthorized caller receives identical `404` responses regardless of whether the requested ID belongs to a real record or not.

---

### IMPORTANT-5: `swimmer.update` Audit Event Missing `Before` State

**File:** `server/internal/app/handlers/members.go` lines ~318–328

**What went wrong:**
The `updateSwimmer` handler appends an audit event with only `After: swimmer` and no `Before` state. All other modify-action audit events (billing, meets overrides) capture the before-state. Swimmer data includes medical flags, which are high-sensitivity fields. The audit log cannot answer "what was changed" for swimmer edits.

**How to fix:**
Before calling the service to update, fetch the current swimmer record:
```go
beforeSwimmer, err := h.service.GetSwimmer(orgID, principal, swimmerID)
// ... then proceed with update ...
h.auditService.Append(audit.Event{
    ...
    Before: beforeSwimmer,
    After:  updatedSwimmer,
})
```

**What counts as passing:**
The audit event for `members.swimmer.update` includes a non-nil `Before` field containing the swimmer's state prior to the update.

---

### IMPORTANT-6: Possible Duplicate Audit Events for `meet.entry.override`, `approveEntry`, and `lockCommitments`

**File:** `server/internal/app/handlers/meets.go` — `overrideEntry` (~line 838), `approveEntry` (~line 889), `lockCommitments` (~line 678)

**What went wrong:**
These handlers both return an `auditEvent` from the service call (which itself may internally record an event) and also call `h.auditService.Append(...)` in the handler. If the service constructs and stores its own internal event *and* the handler appends a second one, two audit records are written per action. This needs verification against `meets/service.go`.

**How to fix:**
Audit events must be written exactly once per action. Choose either: (a) the service writes the audit event and the handler does not, or (b) the handler is responsible and the service never writes audit events directly. The current pattern of returning `auditEvent` from the service AND writing in the handler creates ambiguity.

**What counts as passing:**
Exactly one audit event is written per `overrideEntry`, `approveEntry`, and `lockCommitments` call. Verify by checking that `h.auditService.Append` is only called in handlers (not inside service methods) or vice versa — not both.

---

### MINOR: `Diff` Helper Undocumented Flat-Struct Constraint

**File:** `server/internal/app/audit/service.go` lines ~219–248

**What went wrong:**
The `Diff` function only works correctly on flat JSON objects. Passing a Go struct with nested slices or objects will either produce misleading whole-key diffs or return a JSON unmarshal error. This constraint is not documented.

**How to fix:**
Add a comment to `Diff` explaining the flat-struct limitation, OR implement recursive diffing for nested objects. At minimum, add a guard that returns a clear error if `toStringMap` fails so callers know the before/after values were incompatible.

---

## Master List of All Files Created or Modified in STEP 16

### New files (created in STEP 16):
1. `db/migrations/20260316000100_create_audit_events.sql`
2. `db/migrations/20260316000200_create_data_export_requests.sql`
3. `db/migrations/20260316000300_create_data_deletion_requests.sql`
4. `server/internal/app/audit/service.go`
5. `server/internal/app/compliance/service.go`
6. `server/internal/app/handlers/audit.go`
7. `server/internal/app/handlers/compliance.go`
8. `docs/compliance/breach-response-runbook.md`
9. `docs/planning/STEP16-implementation-plan.md`

### Modified files (wired/extended in STEP 16):
10. `server/internal/app/server.go` — added 13th (`*audit.Service`) and 14th (`*compliance.Service`) parameters to `NewRouterWithServices`; audit/compliance service init; env-based config wiring
11. `server/internal/app/config.go` — added `AuditConfig` and `ComplianceConfig` structs with `RetentionDays`, `ExportTTLDays`, `DeletionReviewDays`
12. `server/internal/app/handlers/auth.go` — added `auditService` field; `auth.role.assign` audit event
13. `server/internal/app/handlers/members.go` — added `auditService` field; `members.swimmer.update` audit event
14. `server/internal/app/handlers/meets.go` — added `auditService` field; `meet.entry.override`, `meet.entry.approve`, `meet.commitments.lock` audit events
15. `server/internal/app/handlers/imports.go` — added `auditService` field; `import.conflict.resolve` audit event
16. `server/internal/app/handlers/performance.go` — added `auditService` field; `performance.best_time.override` audit event
17. `server/internal/app/handlers/billing.go` — added `auditService` field; `billing.invoice.issue`, `billing.invoice.void`, `billing.invoice.write_off`, `billing.refund.create` audit events

---

## What Counts as a Full Pass for STEP 16

All of the following must be true:

1. **Security**: Parent role cannot create compliance requests for other households (`CRITICAL-1`).
2. **DB immutability**: `audit_events` table has a trigger or rule preventing `UPDATE`/`DELETE` (`CRITICAL-2`).
3. **Tests exist**: `server_audit_test.go` and/or `server_compliance_test.go` present and passing via `make test` (`CRITICAL-3`).
4. **Input validation**: Malformed `from`/`to` audit query params return HTTP 400 (`IMPORTANT-1`).
5. **Config wired**: `DeletionReviewDays` passed to compliance service and used to set `ExpiresAt` on deletion requests (`IMPORTANT-2`).
6. **TTL enforced**: `exportTTLDays` used to set `ExpiresAt` on export requests (`IMPORTANT-3`).
7. **Existence leakage**: Compliance GET returns `404` uniformly for both "not found" and "unauthorized" cases for non-owner/non-admin callers (`IMPORTANT-4`).
8. **Audit before-state**: `swimmer.update` audit event includes `Before` state (`IMPORTANT-5`).
9. **No duplicate audits**: Each privileged action writes exactly one audit event (`IMPORTANT-6`).
