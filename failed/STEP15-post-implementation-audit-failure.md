# STEP 15 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** STEP 15 — Communications Module
**Verdict:** FAIL (3 of 4 conditions not met)

---

## Condition Results Summary

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **FAIL** |
| 2 | No regressions in code quality | PASS (with warnings) |
| 3 | No clear code security issues | **FAIL** |
| 4 | All directions followed and completed fully | **FAIL** |

---

## Condition 1: Bugs / Edge Cases — FAIL

### Bug 1 (Critical): `SendCampaign` always transitions campaign to `"sent"` — `"failed"` state is unreachable

**File:** `server/internal/app/comms/service.go`, line 444

**What went wrong:**
After the send loop, `c.Status` is unconditionally set to `"sent"` regardless of how many recipients failed. If every single recipient fails to send (network error, bad token, etc.), the campaign is still marked `"sent"`. The `"failed"` terminal state defined in the DB migration (`check (status in ('draft', 'sending', 'sent', 'failed', 'cancelled'))`) is completely unreachable from the service layer.

**Affected code:**
```go
// service.go ~line 442-451
now := time.Now().UTC()
s.mu.Lock()
c.Status = "sent"   // ← ALWAYS "sent", even if sentCount == 0 and failedCount == N
...
```

**How to fix:**
Determine the final campaign status based on send outcomes:
```go
finalStatus := "sent"
if sentCount == 0 && suppressedCount == 0 && len(inp.Recipients) > 0 {
    finalStatus = "failed"
}
c.Status = finalStatus
```

**What counts as passing:** The `"failed"` status must be reachable when all non-suppressed recipients fail to send.

---

### Bug 2 (Important): `TotalRecipients` overcounts when blank-email entries are in the input

**File:** `server/internal/app/comms/service.go`, line 446

**What went wrong:**
The send loop skips entries where `email == ""` with `continue`, but `c.TotalRecipients` is set to `len(inp.Recipients)` — the pre-filter count. This means blank-email entries inflate `total_recipients` without contributing to any delivery record.

**Affected code:**
```go
c.TotalRecipients = len(inp.Recipients)  // ← should count only processed (non-blank) recipients
```

**How to fix:**
Count a `processedCount` variable that increments for every non-blank email inside the loop, then set `c.TotalRecipients = processedCount`.

**What counts as passing:** `total_recipients` must equal the number of delivery records actually created.

---

### Bug 3 (Important): `isSuppressed` is called while caller holds lock, but the function has no enforcement or documentation of this precondition

**File:** `server/internal/app/comms/service.go`, lines 194–197

**What went wrong:**
`isSuppressed` reads `s.suppression` without acquiring any lock itself. The caller at line 407 correctly acquires `s.mu.RLock()` before calling it, so at present there is no data race. However, `isSuppressed` carries no `// caller must hold mu (at least RLock)` comment, making it a latent data race risk for any future code path that calls it without the lock.

**How to fix:**
Add a comment to `isSuppressed`:
```go
// isSuppressed reports whether the email is suppressed for the org.
// Caller must hold s.mu (at minimum RLock) before calling.
func (s *Service) isSuppressed(orgID, email string) bool {
```

**What counts as passing:** The precondition is documented on the function.

---

## Condition 3: Security Issues — FAIL

### Security Issue 1 (High): Caller-supplied recipient list allows campaigns to be sent to arbitrary external email addresses

**File:** `server/internal/app/handlers/comms.go`, lines 310–328

**What went wrong:**
The `POST /api/v1/email-campaigns/{id}/send` endpoint accepts a `recipients` array from the HTTP request body without any validation that the provided emails belong to org members. A `coach`-or-higher principal could craft a request to send org-branded email to any arbitrary external addresses.

The STEP 15 plan (lines 84–88) describes server-side recipient resolution from the org's user list as the intended production behavior. The current implementation is an acknowledged stub but is not guarded — no feature flag, no warning comment, no `TODO(security)` marker.

**How to fix:**
Tag the handler with a blocking security TODO and consider rejecting the request if `APP_ENV != "development"`:
```go
// SECURITY TODO: In production, recipients MUST be resolved server-side from
// org membership. Accepting a caller-supplied recipient list is a stub for dev
// testing only. This MUST NOT be deployed to production as-is.
```

Alternatively, if the platform has a dev-only flag pattern (e.g., `AUTH_ENABLE_DEV_STUB_TOKENS`), add a corresponding `COMMS_ALLOW_ARBITRARY_RECIPIENTS` guard that defaults to `false`.

**What counts as passing:** The endpoint is either gated to reject arbitrary recipients in non-dev environments, OR it is prominently tagged with a `SECURITY TODO` that marks it as a deployment blocker.

---

### Security Issue 2 (Moderate): `PostmarkConfig.ServerToken` has no log-masking protection

**File:** `server/internal/app/config.go` (PostmarkConfig struct)

**What went wrong:**
The `ServerToken` field on `PostmarkConfig` has no `json:"-"` tag and no `String()` method. If `cfg` is ever marshalled to JSON for debug logging (a common operational pattern), the live Postmark server token will be exposed in log output. The same gap exists in `StripeConfig.SecretKey` and `StorageConfig.SecretKey` introduced in earlier steps.

**How to fix:**
Add `json:"-"` tags to sensitive fields, or implement a `String()` method that redacts them:
```go
type PostmarkConfig struct {
    ServerToken   string `env:"POSTMARK_SERVER_TOKEN" json:"-"`
    FromName      string `env:"POSTMARK_FROM_NAME"`
    FromEmail     string `env:"POSTMARK_FROM_EMAIL"`
}
```

**What counts as passing:** Sensitive credential fields cannot be accidentally serialized to JSON or exposed via `fmt.Sprintf("%+v", cfg)`.

---

## Condition 4: Directions Followed and Completed Fully — FAIL

### Schema/Model Mismatch: `email_campaigns` migration is missing the `suppressed_count` column

**File:** `db/migrations/20260315004200_create_email_campaigns.sql`, lines 17–19
**Cross-reference:** `server/internal/app/comms/service.go`, line 70 (`SuppressedCount int`)

**What went wrong:**
The `email_campaigns` table migration does not include a `suppressed_count` column:

```sql
-- Current migration (lines 17-19):
total_recipients INT NOT NULL DEFAULT 0,
sent_count       INT NOT NULL DEFAULT 0,
failed_count     INT NOT NULL DEFAULT 0,
-- suppressed_count is MISSING
```

The Go domain type `EmailCampaign` has `SuppressedCount int` and `SendCampaign` writes to it at line 449. When the in-memory layer is replaced with real DB queries (the intended next phase), there will be no column to write to, requiring an emergency corrective migration.

The CLAUDE.md spec explicitly lists `suppressed_count` as a tracked metric:
> "campaign tracks sent_count/failed_count/suppressed_count/total_recipients"

**How to fix:**
Add `suppressed_count INT NOT NULL DEFAULT 0` to the migration:
```sql
total_recipients  INT NOT NULL DEFAULT 0,
sent_count        INT NOT NULL DEFAULT 0,
failed_count      INT NOT NULL DEFAULT 0,
suppressed_count  INT NOT NULL DEFAULT 0,   -- ADD THIS
```

Since Goose migrations are already applied, this requires a new corrective migration:
```
db/migrations/20260315004500_add_suppressed_count_to_campaigns.sql
```

**What counts as passing:** The `email_campaigns` table schema includes `suppressed_count INT NOT NULL DEFAULT 0`, matching the Go domain model.

---

## Condition 2: Code Quality — PASS (with warnings)

The following code quality issues were found but do not constitute a failure condition. They should be addressed before production deployment:

1. **`hasAnyRole` helper is duplicated** across handler packages (`comms.go` and likely others). Should be consolidated into a shared utility in the `handlers` package.
2. **`NewServiceWithClient` wastes an allocation** — it calls `NewService()` (which creates and discards a `StubClient`) before overwriting `postmarkClient`. Minor.
3. **`doCommsReq` test helper sends `{}` as body on GET/DELETE requests** — unconventional but does not affect test correctness under `httptest`.
4. **`HTTPClient.SendEmail` does not check `resp.StatusCode` before JSON decode** — add a `resp.StatusCode >= 400` guard for robustness against proxy-level errors.

---

## Master List of Files Edited in STEP 15

The following files were created or modified by STEP 15:

### New files created:
1. `db/migrations/20260315004100_create_announcements.sql`
2. `db/migrations/20260315004200_create_email_campaigns.sql` ← **contains schema bug (missing suppressed_count)**
3. `db/migrations/20260315004300_create_email_deliveries.sql`
4. `db/migrations/20260315004400_create_suppression_list.sql`
5. `server/internal/postmark/client.go`
6. `server/internal/app/comms/service.go` ← **contains logic bugs**
7. `server/internal/app/handlers/comms.go` ← **contains security gap**
8. `server/internal/app/server_comms_test.go`
9. `docs/planning/STEP15-implementation-plan.md`

### Modified files:
10. `server/internal/app/server.go` — added 12th `*comms.Service` parameter to `NewRouterWithServices`
11. `server/cmd/api/main.go` — wired comms service into router construction
12. `server/internal/app/config.go` — added `PostmarkConfig` struct ← **contains security gap (no log masking)**

---

## What "Passing" Looks Like

STEP 15 passes when:

1. **Condition 1 (Bugs):**
   - `SendCampaign` correctly sets `c.Status = "failed"` when all non-suppressed sends fail
   - `c.TotalRecipients` counts only recipients that were actually processed (non-blank emails)
   - `isSuppressed` has a comment documenting its lock precondition

2. **Condition 3 (Security):**
   - The `send` endpoint is either gated in non-dev environments to reject arbitrary recipients, OR prominently tagged with a blocking `SECURITY TODO` comment
   - `PostmarkConfig.ServerToken` (and other credential fields) have `json:"-"` tags or equivalent masking

3. **Condition 4 (Completeness):**
   - A corrective migration `20260315004500_add_suppressed_count_to_campaigns.sql` is applied, adding `suppressed_count INT NOT NULL DEFAULT 0` to `email_campaigns`
   - OR the original migration `20260315004200` is updated if not yet applied

Condition 2 (Code Quality) is already passing and does not need additional work to meet the bar.
