# STEP 0–20E Full Platform Audit Failure

**Date:** 2026-03-17
**Scope:** Full platform audit — STEP 0 through STEP 20E
**Result:** FAIL — Conditions 1, 3, and 4 are not met

---

## Conditions Evaluated

| # | Condition | Status |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **FAIL** |
| 2 | No regressions in code quality | PASS (minor quality issues noted but no regressions) |
| 3 | No clear code security issues | **FAIL** |
| 4 | All directions followed and completed fully | **FAIL** (nav dead links, missing dev stubs, HY3 not wired) |

---

## Condition 1 — Bugs, Potential Issues, Edge Cases

### BUG-1: HTTP Server Missing Timeouts (Critical)
**File:** `server/cmd/api/main.go` ~line 73
**What went wrong:** `http.Server` is configured with only `ReadHeaderTimeout: 10s`. No `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` are set. This allows clients to hold connections indefinitely (Slowloris-style DoS).
**How to fix:**
```go
httpServer := &http.Server{
    Addr:              cfg.Addr,
    Handler:           handler,
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      60 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```
**What counts as passing:** All four timeout fields set with production-appropriate values.

---

### BUG-2: Scheduler Goroutine Leaked in `NewRouter` (Critical / Important)
**File:** `server/internal/app/server.go` ~line 30
**What went wrong:** `BuildWorkers` returns both a `*workers.Server` (`ws`) and a `*workers.Scheduler` (`sched`). `NewRouter` calls `ws.Start()` and `sched.Start()` then passes only `ws` into `NewRouterWithServices`. `sched` is a local variable that goes out of scope — there is no way to call `sched.Stop()` at shutdown. The scheduler goroutine runs forever, enqueuing jobs into a server that may already be stopped.

Additionally, `NewRouterWithServices` has a `if workerServer == nil` block (~lines 136–141) that constructs a second worker server and discards the returned scheduler with `_`. Any call path through this branch leaks a scheduler.

**How to fix:**
1. In `NewRouter`, store `sched` in a returned struct or make it part of the application lifecycle so it can be stopped on shutdown alongside `httpServer.Shutdown`.
2. In `NewRouterWithServices`, do not silently discard the scheduler with `_`. Either store it or require callers always pass a non-nil `workerServer`.

**What counts as passing:** Both `ws.Stop()` and `sched.Stop()` are called on graceful shutdown via the same `<-quit` channel path.

---

### BUG-3: `VoidInvoice` Ledger Reversal Uses Wrong Amount (Critical)
**File:** `server/internal/app/billing/service.go` ~line 601
**What went wrong:** `VoidInvoice` writes a ledger entry for `-inv.TotalCents` rather than `-inv.AmountDueCents`. If any partial payments have been applied, the ledger reversal overcorrects. Additionally, the function does not assert that no payments exist (the state machine only allows voiding from `issued`, but this is not validated in code — only the `Status` field is checked).
**How to fix:**
1. Add a guard: `if inv.AmountPaidCents > 0 { return nil, nil, errors.New("cannot void invoice with existing payments") }`
2. Change the ledger entry amount from `inv.TotalCents` to `inv.AmountDueCents`.

**What counts as passing:** The ledger amount on void equals `TotalCents - AmountPaidCents` and a guard rejects voiding any invoice with existing payments.

---

### BUG-4: Billing Service Getters Return Live Internal Pointers — Data Race (High)
**File:** `server/internal/app/billing/service.go` ~line 430
**What went wrong:** `GetInvoice`, `ListInvoices`, `ListPayments`, and `ListRefunds` return the actual `*Invoice`/`*Payment`/`*Refund` pointers stored in the internal maps. The read lock is released when the function returns. Any concurrent write (e.g., `IssueInvoice` setting `inv.Status`) races against the caller reading the returned pointer.
**How to fix:** Return value copies from all read methods:
```go
// Instead of returning inv directly:
inv := s.invoices[id]
copy := *inv
return &copy, nil
```
For slice returns, copy each element. The `LedgerEvents()` method already does this correctly with `copy(out, s.ledger)` — apply the same pattern to all invoice/payment/refund getters.
**What counts as passing:** No method in `billing/service.go` returns a pointer that is also held in an internal map.

---

### BUG-5: `NewRouterWithServices` Null-Worker Path Discards Scheduler (Important)
**File:** `server/internal/app/server.go` ~line 136
**What went wrong:** Inside the `if workerServer == nil` block, `BuildWorkers` is called and the second return value (scheduler) is assigned to `_`, making it permanently inaccessible and unstoppable.
**How to fix:** See BUG-2 fix — ensure all scheduler instances are stoppable regardless of which `NewRouter*` path is taken.
**What counts as passing:** Scheduler is always stoppable regardless of which code path is used.

---

### BUG-6: TOTP Code Replay Not Prevented (High)
**File:** `server/internal/app/authn/service.go` ~line 481
**What went wrong:** `verifyTOTP` accepts codes within a ±30-second window but does not record used codes. The same TOTP code can be used to authenticate multiple times within the 90-second acceptance window.
**How to fix:** Add a short-lived in-memory set of recently-used TOTP codes keyed by `(factorID + code)` with a 90-second TTL. Reject codes already in the set.
**What counts as passing:** `ChallengeMFA` returns an error if the exact same TOTP code is submitted twice within the replay window.

---

### BUG-7: `randomAlphaNumeric` Modulo Bias in Backup Code Generation (High)
**File:** `server/internal/app/authn/service.go` ~line 987
**What went wrong:** `int(value) % len(charset)` where `value` is a random byte (0–255) and `charset` has 36 characters. Since 256 is not evenly divisible by 36, characters at index 0–3 are ~0.4% more likely. For security-critical backup codes this is a correctness defect.
**How to fix:** Use rejection sampling or `crypto/rand.Int(rand.Reader, big.NewInt(int64(len(charset))))`.
**What counts as passing:** Character selection is provably unbiased across all 36 charset positions.

---

### BUG-8: JWKS Fetched on Every Login Without Caching (High)
**File:** `server/internal/app/authn/service.go` ~line 734
**What went wrong:** `verifyAndParseIDToken` calls `fetchOIDCJWKS()` on every invocation. This is a synchronous outbound HTTP request on the hot login path. JWKS keys rotate very infrequently; fetching on every request adds latency and causes all logins to fail if the IdP's JWKS endpoint is briefly unreachable.
**How to fix:** Add a JWKS cache in the `Service` struct that caches keys for at least 5 minutes, refreshing only when a token references an unknown `kid` or the cache TTL expires.
**What counts as passing:** JWKS is fetched at most once per 5 minutes; a token with a known `kid` does not trigger a network call.

---

## Condition 3 — Security Issues

### SEC-1: Missing Dev Stub Tokens `stub-parent`, `stub-readonly`, `stub-swimmer` on Backend (Critical)
**File:** `server/internal/app/authn/service.go` ~line 219
**What went wrong:** The backend `staticPrincipals` map defines `stub-owner`, `stub-admin`, `stub-admin-mfa`, `stub-coach`, and `stub-billing`. The frontend `DEV_SESSIONS` in `admin/lib/session.tsx` includes `stub-parent`, `stub-readonly`, and `stub-swimmer`. All API calls from the parent portal and swimmer portal in development will be rejected with 401 Unauthorized, making both portals completely non-functional during development.
**How to fix:** Add the missing stubs to `staticPrincipals` in `authn/service.go`:
```go
"stub-parent":   {ID: "stub-parent-id",   OrgID: "stub-org", Roles: []string{"parent"}},
"stub-readonly": {ID: "stub-readonly-id", OrgID: "stub-org", Roles: []string{"read-only"}},
"stub-swimmer":  {ID: "stub-swimmer-id",  OrgID: "stub-org", Roles: []string{"swimmer"}},
```
**What counts as passing:** A request with `Authorization: Bearer stub-parent` receives a 200 from parent-accessible endpoints when `AUTH_ENABLE_DEV_STUB_TOKENS=true`.

---

### SEC-2: No Security Headers Middleware (High)
**File:** `server/internal/app/server.go`
**What went wrong:** No middleware sets baseline HTTP security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, etc.).
**How to fix:** Add a middleware before all routes that sets at minimum:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`

**What counts as passing:** All API responses include the above headers.

---

### SEC-3: No Rate Limiting on Auth Endpoints (High)
**File:** `server/internal/app/server.go` and auth handlers
**What went wrong:** `/auth/sessions/issue`, `/auth/sessions/refresh`, `/auth/mfa/challenge`, and `/auth/oidc/callback` have no rate limiting, making brute-force attacks against these endpoints unrestricted.
**How to fix:** Apply per-IP rate limiting middleware on the `/auth` subrouter. For MFA specifically, enforce at most 5 failed attempts per user per minute before a temporary lockout.
**What counts as passing:** More than 10 requests per minute to `/auth/sessions/issue` from the same IP receive 429.

---

### SEC-4: Admin Layout `AuthGuard` Has No `requiredRoles` (Important)
**File:** `admin/app/(admin)/layout.tsx`
**What went wrong:** `<AuthGuard>` wraps the admin layout without `requiredRoles`. Any authenticated user — including a `parent` or `swimmer` — can access the admin route group shell. While individual pages may check roles, the layout itself renders for all authenticated users.
**How to fix:**
```tsx
<AuthGuard requiredRoles={['owner', 'admin', 'coach', 'billing', 'read-only']}>
```
**What counts as passing:** A session with role `parent` navigating directly to `/dashboard` is redirected to `/unauthorized` before the admin shell renders.

---

### SEC-5: Default MinIO Credentials Hardcoded in Config (Important)
**File:** `server/internal/app/config.go` ~line 118
**What went wrong:** `STORAGE_ACCESS_KEY` defaults to `"minioadmin"` and `STORAGE_SECRET_KEY` defaults to `"minioadmin"`. A non-development deployment without these env vars will silently use insecure default credentials.
**How to fix:** Default to empty string and add startup validation:
```go
if cfg.AppEnv != "development" && (cfg.StorageAccessKey == "" || cfg.StorageSecretKey == "") {
    return nil, errors.New("STORAGE_ACCESS_KEY and STORAGE_SECRET_KEY must be explicitly set in non-development environments")
}
```
**What counts as passing:** Server refuses to start in non-development mode without explicit storage credentials.

---

### SEC-6: TOTP Replay (High — cross-referenced from BUG-6)
See BUG-6 above.

---

### SEC-7: Backup Code Modulo Bias (High — cross-referenced from BUG-7)
See BUG-7 above.

---

## Condition 4 — Directions Not Fully Followed

### COMP-1: Sidebar Nav Has Dead Links for `/commitments` and `/entries` (Medium)
**File:** `admin/lib/roles.ts` ~line 81
**What went wrong:** The sidebar contains nav items with paths `/commitments` and `/entries`. These pages do not exist as standalone routes — commitments and entries are tabs within `admin/app/(admin)/meets/[id]/page.tsx`. Clicking these nav items returns a Next.js 404.
**How to fix:**
Option A: Remove `/commitments` and `/entries` nav items from `NAV_ITEMS`.
Option B: Create redirect or list pages at `admin/app/(admin)/commitments/page.tsx` and `admin/app/(admin)/entries/page.tsx`.
Option C: Update the nav items to point to `/meets` instead.
**What counts as passing:** No sidebar nav item links to a route that returns 404.

---

### COMP-2: HY3 Import Not Reachable via API (High)
**File:** `server/internal/app/handlers/imports.go` ~line 55 and `server/internal/app/imports/service.go` ~line 147
**What went wrong:** STEP 10 implemented the full HY3 parsing pipeline (`internal/meetfiles/hy3.go`) but the API import handler validation uses `oneof=cl2 sd3 zip`, excluding `hy3`. The entire HY3 pipeline is unreachable from the API. The `ErrInvalidFileType` message also omits `hy3`.
**How to fix:**
1. Change validator tag to `oneof=cl2 sd3 zip hy3`.
2. Update `ErrInvalidFileType` message to include `hy3`.
3. Ensure `FinalizeImport` routes `hy3` file type through the HY3 reader in `internal/meetfiles`.

**What counts as passing:** A request with `file_type: "hy3"` passes handler validation and the HY3 parser is invoked during finalization.

---

## Master List of All Files Edited in STEP 0–20E

### Go Backend — `server/`
```
server/go.mod
server/go.sum
server/cmd/api/main.go
server/internal/app/config.go
server/internal/app/server.go
server/internal/app/router.go (if separate from server.go)
server/internal/app/middleware/ (all files)
server/internal/app/authn/service.go
server/internal/app/members/service.go
server/internal/app/groups/service.go
server/internal/app/meets/service.go
server/internal/app/imports/service.go
server/internal/app/exports/service.go
server/internal/app/performance/service.go
server/internal/app/billing/service.go
server/internal/app/comms/service.go
server/internal/app/audit/service.go
server/internal/app/compliance/service.go
server/internal/app/handlers/auth.go
server/internal/app/handlers/members.go
server/internal/app/handlers/groups.go
server/internal/app/handlers/meets.go
server/internal/app/handlers/imports.go
server/internal/app/handlers/exports.go
server/internal/app/handlers/performance.go
server/internal/app/handlers/billing.go
server/internal/app/handlers/stripe.go
server/internal/app/handlers/comms.go
server/internal/app/handlers/audit.go
server/internal/app/handlers/compliance.go
server/internal/app/handlers/jobs.go
server/internal/stripe/client.go
server/internal/postmark/client.go
server/internal/storage/client.go
server/internal/workers/worker.go
server/internal/workers/scheduler.go
server/internal/workers/metrics.go
server/internal/workers/jobhandlers/ (all files)
```

### Meet Files Library — `internal/meetfiles/`
```
internal/meetfiles/go.mod
internal/meetfiles/go.sum
internal/meetfiles/detector.go
internal/meetfiles/matcher.go
internal/meetfiles/conflicts.go
internal/meetfiles/report.go
internal/meetfiles/zip.go
internal/meetfiles/hy3.go
internal/meetfiles/sdif/parser.go
internal/meetfiles/sdif/exporter.go
internal/meetfiles/cl2/parser.go
internal/meetfiles/*_test.go
```

### Database Migrations — `db/migrations/`
```
db/migrations/20260315000100_init_extensions.sql
db/migrations/20260315000200_create_users.sql
db/migrations/20260315000300_create_roles.sql
db/migrations/20260315000400_create_org_members.sql
db/migrations/20260315000500_create_sessions.sql
db/migrations/20260315000600_create_mfa_factors.sql
db/migrations/20260315000700_harden_user_roles_uniqueness.sql
db/migrations/20260315000800_seed_default_roles.sql
db/migrations/20260315000900_create_households.sql
db/migrations/20260315001000_create_contacts.sql
db/migrations/20260315001100_create_swimmers.sql
db/migrations/20260315001200_create_swimmer_group_memberships.sql
db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql
db/migrations/20260315001400_create_groups.sql
db/migrations/20260315001500_create_eligibility_rules.sql
db/migrations/20260315001600_create_registrations.sql
db/migrations/20260315001700_create_waiver_acceptances.sql
db/migrations/20260315001800_create_meets.sql
db/migrations/20260315001900_create_meet_sessions.sql
db/migrations/20260315002000_create_meet_events.sql
db/migrations/20260315002100_create_commitments.sql
db/migrations/20260315002200_create_meet_entries.sql
db/migrations/20260315002300_create_entry_legs.sql
db/migrations/20260315002500_create_file_imports.sql
db/migrations/20260315002600_create_import_rows.sql
db/migrations/20260315002700_create_import_conflicts.sql
db/migrations/20260315002800_create_import_reports.sql
db/migrations/20260315002900_create_file_exports.sql
db/migrations/20260315003000_create_results.sql
db/migrations/20260315003100_create_best_times.sql
db/migrations/20260315003200_create_progression_events.sql
db/migrations/20260315003300_create_time_standards.sql
db/migrations/20260315003500_create_invoices.sql
db/migrations/20260315003600_create_invoice_items.sql
db/migrations/20260315003700_create_payments.sql
db/migrations/20260315003800_create_refunds.sql
db/migrations/20260315003900_create_ledger_events.sql
db/migrations/20260315004000_create_webhook_events.sql
db/migrations/20260315004100_create_announcements.sql
db/migrations/20260315004200_create_email_campaigns.sql
db/migrations/20260315004300_create_campaign_deliveries.sql
db/migrations/20260315004400_create_suppression_list.sql
db/migrations/20260316000100_create_audit_events.sql
db/migrations/20260316000200_create_data_export_requests.sql
db/migrations/20260316000300_create_data_deletion_requests.sql
```

### Admin Frontend — `admin/`
```
admin/package.json
admin/tsconfig.json
admin/next.config.ts
admin/.eslintrc.json
admin/app/layout.tsx
admin/app/globals.css
admin/app/providers.tsx
admin/app/page.tsx
admin/app/auth/callback/page.tsx
admin/app/mfa/page.tsx
admin/app/logout/page.tsx
admin/app/unauthorized/page.tsx
admin/app/(admin)/layout.tsx
admin/app/(admin)/dashboard/page.tsx
admin/app/(admin)/members/households/page.tsx
admin/app/(admin)/members/households/[id]/page.tsx
admin/app/(admin)/members/swimmers/page.tsx
admin/app/(admin)/members/swimmers/[id]/page.tsx
admin/app/(admin)/members/groups/page.tsx
admin/app/(admin)/members/groups/[id]/page.tsx
admin/app/(admin)/members/registrations/page.tsx
admin/app/(admin)/meets/page.tsx
admin/app/(admin)/meets/[id]/page.tsx
admin/app/(admin)/meets/new/page.tsx
admin/app/(admin)/imports/page.tsx
admin/app/(admin)/imports/[id]/page.tsx
admin/app/(admin)/exports/page.tsx
admin/app/(admin)/invoices/page.tsx
admin/app/(admin)/invoices/[id]/page.tsx
admin/app/(admin)/ledger/page.tsx
admin/app/(admin)/billing/page.tsx
admin/app/(admin)/billing/stripe/page.tsx
admin/app/(admin)/announcements/page.tsx
admin/app/(admin)/announcements/[id]/page.tsx
admin/app/(admin)/campaigns/page.tsx
admin/app/(admin)/campaigns/[id]/page.tsx
admin/app/(admin)/comms/suppression/page.tsx
admin/app/(admin)/audit/page.tsx
admin/app/(admin)/jobs/page.tsx
admin/app/(admin)/reports/page.tsx
admin/app/(admin)/performance/time-standards/page.tsx
admin/app/(admin)/performance/best-times/page.tsx
admin/app/(admin)/compliance/data-exports/page.tsx
admin/app/(admin)/compliance/data-deletions/page.tsx
admin/app/(admin)/settings/page.tsx
admin/app/(admin)/settings/roles/page.tsx
admin/app/(portal)/layout.tsx
admin/app/(portal)/portal/dashboard/page.tsx
admin/app/(portal)/portal/household/page.tsx
admin/app/(portal)/portal/household/swimmers/[id]/page.tsx
admin/app/(portal)/portal/registrations/page.tsx
admin/app/(portal)/portal/registrations/new/page.tsx
admin/app/(portal)/portal/meets/page.tsx
admin/app/(portal)/portal/meets/[id]/page.tsx
admin/app/(portal)/portal/billing/page.tsx
admin/app/(portal)/portal/billing/invoices/[id]/page.tsx
admin/app/(portal)/portal/documents/page.tsx
admin/app/(portal)/portal/communications/page.tsx
admin/app/(portal)/portal/calendar/page.tsx
admin/app/(swimmer)/layout.tsx
admin/app/(swimmer)/swim/page.tsx
admin/app/(swimmer)/swim/performance/page.tsx
admin/app/(swimmer)/swim/profile/page.tsx
admin/app/(swimmer)/swim/schedule/page.tsx
admin/components/auth/AuthGuard.tsx
admin/components/layout/Sidebar.tsx
admin/components/layout/Topbar.tsx
admin/components/portal/PortalShell.tsx
admin/components/portal/PortalNav.tsx
admin/components/swimmer/SwimmerShell.tsx
admin/components/swimmer/SwimmerNav.tsx
admin/components/ui/icons.tsx
admin/components/ui/Toast.tsx
admin/components/ui/Skeleton.tsx
admin/components/ui/EmptyState.tsx
admin/components/ui/Table.tsx
admin/components/ui/Tabs.tsx
admin/components/forms/FormWrapper.tsx
admin/lib/session.tsx
admin/lib/api.ts
admin/lib/roles.ts
admin/lib/featureFlags.ts
```

### Docs, Config, and Root Files
```
docs/api/openapi.yaml
docs/api/.spectral.yaml
docs/decisions/2026-03-15-hy3-format-handling.md
docs/decisions/2026-03-15-hy3-export-v1.md
docs/planning/STEP1-implementation-notes.md
docs/planning/STEP2-implementation-notes.md
docs/planning/STEP3-implementation-notes.md
docs/planning/STEP4-implementation-notes.md
docs/planning/STEP5-implementation-notes.md
docs/planning/STEP5-stub-retirement-hardening-plan.md
docs/planning/STEP6-implementation-notes.md
docs/planning/STEP7-implementation-notes.md
docs/planning/STEP8-implementation-notes.md
docs/planning/STEP9-implementation-notes.md
docs/planning/STEP10-implementation-notes.md
docs/planning/STEP11-implementation-notes.md
docs/planning/STEP12-implementation-plan.md
docs/planning/STEP13-implementation-plan.md
docs/planning/STEP14-implementation-plan.md
docs/planning/STEP15-implementation-plan.md
docs/planning/STEP16-implementation-plan.md
docs/planning/STEP17-implementation-plan.md
docs/planning/STEP18-implementation-plan.md
docs/planning/STEP19-implementation-plan.md
docs/planning/STEP20-implementation-plan.md
docs/planning/STEP20E-gap-closure-plan.md
docs/security/api-key-secret-storage-strategy.md
docs/compliance/breach-response-runbook.md
docs/testing/e2e-parent-flows.md
docs/testing/mobile-qa-checklist.md
Makefile
docker-compose.yml
.env.example
.env.local.example
README.md
CONTRIBUTING.md
DECISIONS.md
TODO.md
CLAUDE.md
```

---

## What Counts as "Passing" for This Audit (Full Checklist)

All 12 of the following must be true before the audit can pass:

1. HTTP server has all four timeout fields set (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`)
2. Worker scheduler is reachable and stoppable from the graceful shutdown path in `main.go`
3. `stub-parent`, `stub-readonly`, and `stub-swimmer` exist in backend `staticPrincipals` with correct roles
4. `VoidInvoice` guards against invoices with existing payments and uses `AmountDueCents` (not `TotalCents`) in the ledger write
5. All billing service read methods (`GetInvoice`, `ListInvoices`, `ListPayments`, `ListRefunds`) return value copies, not live pointers
6. Import handler and service accept `hy3` as a valid file type and route it to the HY3 parser
7. TOTP codes cannot be replayed within the 90-second acceptance window
8. Backup code generation uses unbiased character selection (rejection sampling or `crypto/rand.Int`)
9. JWKS is cached with a configurable TTL (minimum 5 minutes)
10. Admin route group `AuthGuard` has `requiredRoles` that excludes `parent` and `swimmer` roles
11. MinIO credentials default to empty string with startup validation for non-development environments
12. No sidebar nav item in `admin/lib/roles.ts` links to a non-existent route (no 404s from nav)
