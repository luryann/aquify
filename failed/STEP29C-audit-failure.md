# STEP29C Audit Failure

Date: 2026-03-20
Step: 29C
Status: Failed audit

## Outcome

STEP 29C does not meet the required pass bar.

The audit conditions were not all met:

1. No bugs / UI issues / potential issues / edge cases: failed.
2. No regressions in code quality: failed.
3. No clear code security issues: not confidently met.
4. Followed all directions and completed fully: failed.

## What Went Wrong

### 1) Production runtime still silently falls back to stub providers

The Step 29C plan explicitly says Stripe should use the real client when production credentials are configured and only fall back to the stub in development/test scenarios. The current server wiring still falls back to stub clients unconditionally whenever real client construction fails.

Evidence:

- `docs/planning/STEP29C-provider-hardening-plan.md`
  - Scope says Stripe should only fall back to stub in development/test.
- `server/internal/app/server.go:131`
  - Stripe: if `NewSDKClient` fails, the code logs a warning and uses `NewStubClient()` with no environment guard.
- `server/internal/app/server.go:149`
  - Postmark: if `NewHTTPClient` cannot be created, the code falls back to `NewStubClient()` with no production guard.

Why this fails the audit:

- In production, a bad or incomplete provider configuration can silently route billing/email flows onto fake providers.
- That is a launch-readiness regression because the app can appear healthy while payments/emails are no longer using real infrastructure.
- It also directly contradicts the Step 29C implementation plan.

### 2) Refunds are executed against Stripe before the app validates local refund eligibility

The refund handler calls Stripe first, then attempts the local billing mutation afterward.

Evidence:

- `server/internal/app/handlers/billing.go:420`
  - `stripeClient.CreateRefund(...)` is called before `service.CreateRefund(...)`.
- `server/internal/app/billing/service.go:780`
  - Local validation for invoice refundability, cumulative refund amount, and state transitions happens inside `CreateRefund`.

Why this fails the audit:

- If Stripe accepts the refund but local validation fails afterward, the external refund is real while the internal ledger/refund record is rejected.
- That can create reconciliation drift, support incidents, and refund double-processing risk.
- This is a correctness bug in a financial workflow, so condition 1 is not met and condition 3 cannot be considered well-met.

### 3) Postmark bounce/complaint webhooks do not fully sync suppression state when the delivery record is missing

The stated goal for Step 29C is to accurately track Postmark delivery/suppression state. The current implementation only applies bounce/complaint suppression if it can first resolve a local delivery by `MessageID`.

Evidence:

- `server/internal/app/comms/service.go:565`
  - `Bounce` returns `ErrDeliveryNotFound` if no delivery is found.
- `server/internal/app/comms/service.go:578`
  - `SpamComplaint` does the same.
- `server/internal/app/handlers/comms.go:195`
  - `ErrDeliveryNotFound` is treated as a successful-but-ignored webhook response.

Why this fails the audit:

- If Postmark sends a valid bounce/complaint event with enough metadata to identify the org/email, but the local delivery record is missing, suppression is not updated.
- That means delivery-state accuracy is only partial, not robust.
- The Step 29C plan and `TODO.md` item require production delivery-state accuracy, so this is an unclosed edge case.

### 4) STEP 29C is not fully complete by the repo's own completion rules

The task list for Step 29C remains unchecked and includes real-environment verification items that are still outstanding.

Evidence:

- `TODO.md:533`
  - All four Step 29C checklist items remain unchecked.
- `docs/planning/STEP29C-provider-hardening-plan.md:17`
  - Live staging verification is listed as out of scope for this execution pass.
- `docs/planning/STEP29C-provider-hardening-plan.md:26`
  - The plan says Step 29C in `TODO.md` should only be checked off when code and real-environment verification are both complete.

Why this fails the audit:

- Condition 4 requires that directions from earlier steps were fully completed.
- By the repo's own rules, Step 29C is still incomplete even if the code changes landed.

## How To Fix It

### Fix 1) Remove silent stub fallback in non-dev environments

Update provider initialization so:

- Development, local, and test may use stubs when credentials are absent.
- Staging and production must fail fast if Stripe/Postmark is expected but cannot initialize.
- The boot path should return a startup error instead of silently substituting a stub client.

Suggested implementation shape:

- In `server/internal/app/server.go`, gate stub fallback by `cfg.AppEnv`.
- If `APP_ENV` is not `development`, `local`, or `test`:
  - missing/invalid Stripe config should stop startup;
  - missing/invalid Postmark config should stop startup.
- If keeping `NewRouter` as a plain `http.Handler` is too restrictive, move environment validation to the startup path in `server/cmd/api/main.go` and refuse to launch there.

### Fix 2) Validate refund eligibility before calling Stripe

Reorder the refund flow so local validation happens before the external refund call.

Recommended approach:

- Add a preflight validation/helper in billing service that checks:
  - invoice exists and is refundable;
  - payment exists and belongs to invoice/org;
  - payment status is refundable;
  - requested amount does not exceed remaining refundable amount.
- Only after preflight succeeds should the handler call Stripe.
- After Stripe returns success, persist the refund atomically in local state.
- If persistence can still fail after Stripe succeeds, add a compensating path or operator alert so the inconsistency is surfaced immediately.

### Fix 3) Allow bounce/complaint suppression updates without a matched delivery record

Adjust Postmark webhook application so suppression can still be updated when:

- `org_id` is present in metadata, and
- recipient email is present,
- even if `MessageID` does not resolve to a local delivery.

Recommended behavior:

- For `Bounce` and `SpamComplaint`:
  - if delivery exists, update delivery status and suppression;
  - if delivery does not exist but `org_id` and email are present, still upsert suppression;
  - only return ignore/no-op when there is not enough information to safely scope the event.

### Fix 4) Keep Step 29C open until staging verification is done

Do not represent Step 29C as complete until the outstanding checklist items are executed:

- real Stripe webhook validation in staging;
- reconciliation verification with live credentials;
- refund flow validation against Stripe;
- balance-fetch verification;
- real Postmark outbound verification;
- inbound webhook verification in a real environment.

## What Counts As Passing

STEP 29C should count as passing only when all of the following are true:

1. Production and staging startup cannot silently use stub Stripe/Postmark clients when real provider configuration is expected.
2. Stripe refunds cannot be created externally before the app has already validated the refund locally.
3. Postmark delivery, bounce, complaint, and suppression webhook paths update state correctly even when local delivery records are incomplete but org/email metadata is sufficient.
4. The focused provider-related Go tests pass.
5. Staging verification is completed for the real Stripe and Postmark flows listed in `TODO.md`.
6. `TODO.md` Step 29C items are updated only after the above is finished.

## Verification Performed

Documents reviewed:

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`
- `docs/planning/STEP29C-provider-hardening-plan.md`

Implementation files reviewed:

- `server/internal/app/server.go`
- `server/internal/app/config.go`
- `server/internal/app/handlers/billing.go`
- `server/internal/app/billing/service.go`
- `server/internal/app/handlers/comms.go`
- `server/internal/app/comms/service.go`
- `server/internal/stripe/client.go`
- `server/internal/postmark/client.go`
- `server/internal/app/server_billing_test.go`
- `server/internal/app/server_comms_test.go`
- `docs/api/openapi.yaml`
- `.env.example`
- `server/go.mod`
- `server/go.sum`

Focused test command run successfully:

```sh
cd server && GOCACHE=/Users/ryan/Documents/aquify/.gocache_local GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache_local go test ./internal/stripe ./internal/postmark ./internal/app/... ./internal/workers/jobhandlers/...
```

Result:

- Tests passed.
- The audit still fails because passing tests did not eliminate the confirmed design/runtime issues above, and Step 29C is still incomplete by the repo's own criteria.

## Master List Of Files Edited By Me In STEP 29C

Based on the Step 29C edit window and the provider-related code/docs touched in this step, the edited files are:

- `/Users/ryan/Documents/aquify/.env.example`
- `/Users/ryan/Documents/aquify/CLAUDE.md`
- `/Users/ryan/Documents/aquify/docs/api/openapi.yaml`
- `/Users/ryan/Documents/aquify/docs/planning/STEP29C-provider-hardening-plan.md`
- `/Users/ryan/Documents/aquify/server/go.mod`
- `/Users/ryan/Documents/aquify/server/go.sum`
- `/Users/ryan/Documents/aquify/server/internal/app/config.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server.go`
- `/Users/ryan/Documents/aquify/server/internal/app/billing/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/comms/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/billing.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/comms.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server_billing_test.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server_comms_test.go`
- `/Users/ryan/Documents/aquify/server/internal/postmark/client.go`
- `/Users/ryan/Documents/aquify/server/internal/stripe/client.go`
