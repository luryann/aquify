# STEP 7 Post-Implementation Audit Failure

Date: 2026-03-15
Scope reviewed: `design-language-guide.html`, `Developer Handoff Master Document.txt`, `Plan.txt`, `CLAUDE.md`, `TODO.md`, and all files created or modified in STEP 7.

## Audit Result

This step **fails** the required quality bar. Three of the four mandatory conditions are not well-met.

| Condition | Result |
|---|---|
| 1. No bugs, potential issues, or edge cases | **FAIL** |
| 2. No regressions in code quality | **FAIL** |
| 3. No clear code security issues | **PASS** |
| 4. All directions followed and fully completed | **FAIL** |

---

## 1) What Went Wrong

### Issue A — Dead code: `ErrRuleNotFound` declared but never returned (Condition 1 & 2)

**File:** `server/internal/app/groups/service.go`, line 21

```go
ErrRuleNotFound = errors.New("eligibility rule not found")
```

This sentinel error is declared in the package-level `var` block but no service method ever returns it. There is no "get rule" or "delete rule" endpoint, and no code path reaches it. It is unreachable dead code that pollutes the public API surface of the package.

---

### Issue B — Dead code: `ErrHouseholdNotFound` declared but never returned; handler case is unreachable (Condition 1 & 2)

**Files:**
- `server/internal/app/groups/service.go`, line 26
- `server/internal/app/handlers/groups.go`, lines 306–308

```go
// service.go – declared:
ErrHouseholdNotFound = errors.New("household not found")

// handlers/groups.go – handler case that can never trigger:
case errors.Is(err, groups.ErrHouseholdNotFound):
    httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
```

`CreateRegistration` verifies that the swimmer exists and that `swimmer.HouseholdID == householdID`, but it never independently looks up the household from the `households` map and never returns `ErrHouseholdNotFound`. The handler case at line 306 is unreachable dead code that creates a false impression of a validation that does not exist.

---

### Issue C — Dead code: `householdRef` type and `households` map allocated but never read (Condition 1 & 2)

**File:** `server/internal/app/groups/service.go`, lines 162–166, 176, 188, 206

```go
// Type defined:
type householdRef struct {
    ID    string
    OrgID string
}

// Map allocated and populated:
households: make(map[string]householdRef),
// ...in SeedSwimmer:
s.households[householdID] = householdRef{ID: householdID, OrgID: orgID}
```

The `households` map is written to in `SeedSwimmer` but is never read anywhere in the service. `parentOwnsHouseholdLocked` uses `s.parentHousehold`, not `s.households`. This is wasted allocation per service instance and creates confusion about the data model.

---

### Issue D — `computeAge` leap year edge case (Condition 1)

**File:** `server/internal/app/groups/service.go`, lines 716–727

```go
func computeAge(dob string) int {
    t, err := time.Parse("2006-01-02", dob)
    if err != nil {
        return -1
    }
    now := time.Now().UTC()
    age := now.Year() - t.Year()
    if now.YearDay() < t.YearDay() {
        age--
    }
    return age
}
```

The use of `YearDay()` for birthday comparison is incorrect when the DOB falls on February 29 (a leap day). In a non-leap year, dates from March 1 onward have a `YearDay()` value that is 1 less than the equivalent date in a leap year. For example:

- DOB: `2008-02-29` → `t.YearDay() = 60` (leap year)
- Today: `2026-03-01` → `now.YearDay() = 60` (non-leap year, day 60 = March 1)
- `60 < 60 = false` → age NOT decremented → computed age: 18
- Correct: the birthday hasn't occurred in 2026 (Feb 29 doesn't exist), so age should be 17

This produces an off-by-one age for the specific edge case of swimmers born on February 29, which can incorrectly pass age-range eligibility rules.

---

### Issue E — Non-standard import grouping in service.go (Condition 2)

**File:** `server/internal/app/groups/service.go`, lines 3–15

```go
import (
    "errors"
    "fmt"
    "slices"
    "strings"
    "sync"
    "time"

    "crypto/rand"   // ← should be in the block above with stdlib imports
    "encoding/hex"  // ← same issue

    appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)
```

The established convention in this codebase (as set by `members/service.go` in STEP 6) is to place **all** standard library imports in a single group, then third-party/internal imports in a second group. Splitting `crypto/rand` and `encoding/hex` into a separate group is a regression from the STEP 6 convention. `goimports` and standard Go tooling would flag and auto-fix this.

---

### Issue F — `ruleEnvelope` type defined but never used in tests (Condition 2)

**File:** `server/internal/app/server_groups_test.go`, lines 34–39

```go
type ruleEnvelope struct {
    Rule struct {
        ID       string `json:"id"`
        RuleType string `json:"rule_type"`
    } `json:"rule"`
}
```

`TestEligibilityRuleAttachAndEvaluate` fires `POST /groups/{id}/eligibility-rules` and checks the HTTP status code, but never decodes or validates the response body using `ruleEnvelope`. The type declaration is dead code. The test is weaker than it should be because it does not verify the returned rule's fields.

---

### Issue G — No audit events for privileged actions (Condition 4)

**File:** `server/internal/app/groups/service.go`
**Reference:** `CLAUDE.md` → Definition of Done → "Audit events included for privileged actions"
**Precedent:** `server/internal/app/members/service.go` → `SwimmerAuditEvent` emitted for every profile create/update

STEP 7 introduces several privileged operations but emits **zero** audit events for any of them:

| Operation | Role required | Audit event emitted? |
|---|---|---|
| `CreateGroup` | owner, admin, coach | ✗ No |
| `UpdateGroup` | owner, admin, coach | ✗ No |
| `AttachEligibilityRule` | owner, admin only | ✗ No |
| `TransitionRegistrationStatus` | admin, coach, parent (withdraw only) | ✗ No |
| `AcceptWaiver` | owner, admin, parent | partial (the waiver record itself stores actor, timestamp, IP — but no separate audit event structure matching the established pattern) |

This directly violates the stated Definition of Done. STEP 6 set the precedent: privileged mutations emit `SwimmerAuditEvent` structs that are returned in the HTTP response body and stored in the audit log. STEP 7 skips this entirely.

The most critical omission is `AttachEligibilityRule` (owner/admin-only, directly controls who can register) and `TransitionRegistrationStatus` (changes membership state).

---

## 2) How To Fix Each Issue

### Fix A — Remove `ErrRuleNotFound`
Delete the line `ErrRuleNotFound = errors.New("eligibility rule not found")` from the `var` block in `service.go`. If a rule-level endpoint is added later, reintroduce it at that time.

### Fix B — Either implement household lookup or remove the dead case
**Option 1 (correct implementation):** Add a lookup to `s.households` in `CreateRegistration` and return `ErrHouseholdNotFound` if the household isn't found or doesn't match the org. Also call `s.households[householdID]` to verify org ownership before accepting the registration.

**Option 2 (remove dead code):** Remove the `ErrHouseholdNotFound` var and the handler case in `groups.go`. Document that household existence is implicitly verified via the swimmer's `HouseholdID`.

Option 1 is preferred for correctness.

### Fix C — Remove `householdRef` type and `households` map
Delete the `householdRef` struct (lines 162–166), the `households` field in `Service` (line 176), its initialization (line 188), and its assignment in `SeedSwimmer` (line 206). If Issue B is fixed with Option 1 above, keep the map but actually read from it.

### Fix D — Fix `computeAge` to use month/day comparison
Replace `YearDay()` comparison with an explicit month+day comparison:

```go
func computeAge(dob string) int {
    t, err := time.Parse("2006-01-02", dob)
    if err != nil {
        return -1
    }
    now := time.Now().UTC()
    age := now.Year() - t.Year()
    // Subtract 1 if birthday hasn't occurred yet this year.
    if now.Month() < t.Month() || (now.Month() == t.Month() && now.Day() < t.Day()) {
        age--
    }
    return age
}
```

This correctly handles all edge cases including Feb 29 birthdays in non-leap years (treated as March 1 in the comparison, so age is decremented until March 1 of the current year).

### Fix E — Consolidate stdlib imports into one group
Move `"crypto/rand"` and `"encoding/hex"` to the first import group with the other stdlib imports:

```go
import (
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "slices"
    "strings"
    "sync"
    "time"

    appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)
```

### Fix F — Use `ruleEnvelope` in the test or remove it
In `TestEligibilityRuleAttachAndEvaluate`, after the `POST .../eligibility-rules` call, decode the response body and assert on `rule.ID` and `rule.RuleType`:

```go
var ruleEnv ruleEnvelope
if err := json.Unmarshal(ruleRes.Body.Bytes(), &ruleEnv); err != nil {
    t.Fatalf("failed to decode rule response: %v", err)
}
if ruleEnv.Rule.ID == "" {
    t.Fatal("expected rule id in response")
}
if ruleEnv.Rule.RuleType != "age_range" {
    t.Fatalf("expected rule_type age_range, got %q", ruleEnv.Rule.RuleType)
}
```

### Fix G — Add audit events for privileged actions
Following the pattern from `members/service.go`:

1. Define a `GroupAuditEvent` struct in `service.go` with fields: `ID`, `OrgID`, `ActorUserID`, `Action`, `Before`, `After`, `Metadata`, `CreatedAt`.
2. Add an `auditEvents []GroupAuditEvent` slice to the `Service` struct and initialize it.
3. Add a `recordAuditEventLocked()` helper (matching the members pattern).
4. Emit audit events in:
   - `CreateGroup`: action `"group.created"`
   - `UpdateGroup`: action `"group.updated"` with before/after diff
   - `AttachEligibilityRule`: action `"group.eligibility_rule_attached"`
   - `TransitionRegistrationStatus`: action `"registration.status_changed"` with before/after status
5. Return audit events in relevant HTTP response bodies (following the swimmer create/update pattern).
6. Add test assertions that verify audit events are returned and contain correct fields.

---

## 3) What Counts as Passing

This step passes the audit only when ALL of the following are true:

1. All dead code is removed or made active: `ErrRuleNotFound`, `ErrHouseholdNotFound` (if no lookup added), `householdRef`, `households` map.
2. `computeAge` uses month/day comparison (not `YearDay()`) and correctly handles leap day birthdays.
3. Import grouping in `service.go` follows the single-stdlib-block convention established in STEP 6.
4. `ruleEnvelope` is used in `TestEligibilityRuleAttachAndEvaluate` to validate the response body, or the type is removed.
5. Audit events (`GroupAuditEvent` or equivalent) are emitted for `CreateGroup`, `UpdateGroup`, `AttachEligibilityRule`, and `TransitionRegistrationStatus`. Events must be stored in the service and returned in HTTP responses, matching the STEP 6 swimmer audit pattern.
6. Tests assert on audit event fields for at least `CreateGroup` and `TransitionRegistrationStatus`.
7. `make test` and `make lint` both pass after all fixes.

---

## 4) Master List of Files Edited by THIS Session (STEP 7)

### Created
1. `db/migrations/20260315001400_create_groups.sql`
2. `db/migrations/20260315001500_create_eligibility_rules.sql`
3. `db/migrations/20260315001600_create_registrations.sql`
4. `db/migrations/20260315001700_create_waiver_acceptances.sql`
5. `server/internal/app/groups/service.go`
6. `server/internal/app/handlers/groups.go`
7. `server/internal/app/server_groups_test.go`
8. `docs/planning/STEP7-implementation-plan.md`
9. `docs/planning/STEP7-implementation-notes.md`

### Modified
10. `server/internal/app/server.go`
11. `server/internal/app/server_members_test.go`
12. `TODO.md`
13. `CLAUDE.md`
14. `failed/STEP7-post-implementation-audit-failure.md` (this file)
