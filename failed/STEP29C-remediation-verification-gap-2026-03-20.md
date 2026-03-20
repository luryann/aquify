# STEP29C Remediation Verification Gap

Date: 2026-03-20
Step: 29C
Status: Code remediation complete locally, final pass bar still not met

## Summary

The code-level failures from the earlier STEP29C audit were remediated in this session and the focused provider-related Go test suites passed locally.

STEP29C still does not qualify as fully passing because the required real-environment staging verification remains incomplete, and `TODO.md` correctly keeps the Step 29C checklist open until that work is executed.

## What Went Wrong

1. The STEP29C pass criteria require live Stripe and Postmark verification in staging with real credentials.
2. This workspace session did not have staging credentials, external provider access, or a real staging environment available for Stripe/Postmark execution.
3. Because of that limitation, the following `TODO.md` items remain intentionally unchecked:
   - Validate real Stripe webhook, reconciliation, refund, and balance-fetch flows in staging with live credentials.
   - Configure a real Postmark client for production sends and verify outbound delivery against sandbox/prod streams.
4. By the repo's own completion rules in `docs/planning/STEP29C-provider-hardening-plan.md`, Step 29C should not be marked complete until both code and real-environment verification are done.

## What Was Fixed In Code

1. Non-dev provider boot now fails closed instead of silently downgrading Stripe/Postmark to stub behavior.
2. Refund handling now performs local eligibility validation before calling Stripe for a refund.
3. Postmark bounce and complaint webhook handling now updates suppression state when `org_id` and recipient email are available even if the delivery record is missing.
4. Tests were added for refund preflight ordering, scoped Postmark suppression without a delivery row, and non-dev provider config validation.

## How To Fix The Remaining Gap

1. Provision valid staging Stripe credentials:
   - `STRIPE_SECRET_KEY`
   - `STRIPE_WEBHOOK_SECRET`
2. Provision valid staging Postmark credentials:
   - `POSTMARK_SERVER_TOKEN`
   - `POSTMARK_FROM_EMAIL`
   - optional webhook auth headers/basic auth values used by the deployment
3. Deploy the current remediation build to staging.
4. Execute and capture evidence for:
   - real Stripe payment intent or checkout flow
   - real Stripe webhook processing
   - real Stripe refund path
   - real Stripe reconciliation and balance-fetch behavior
   - real Postmark outbound send
   - real Postmark delivery/bounce/complaint/subscription-change webhook behavior
5. Update `TODO.md` Step 29C items only after that evidence is complete and reviewed.

## What Counts As Passing

STEP29C counts as passing only when all of the following are true:

1. The code remediation from this session remains in place and passing locally.
2. Staging/production cannot silently use stub Stripe or Postmark providers.
3. Stripe refunds are locally validated before the external refund call is made.
4. Postmark suppression state stays accurate even when a delivery record is missing but scoped metadata is present.
5. Real staging verification is completed for the Stripe and Postmark flows listed in `TODO.md`.
6. `TODO.md` Step 29C is updated only after the real-environment verification above is complete.

## Local Verification Completed

Reviewed documents:

- `failed/STEP29C-audit-failure.md`
- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`
- `docs/planning/STEP29C-provider-hardening-plan.md`

Reviewed implementation files:

- `server/cmd/api/main.go`
- `server/internal/app/server.go`
- `server/internal/app/billing/service.go`
- `server/internal/app/handlers/billing.go`
- `server/internal/app/comms/service.go`
- `server/internal/app/handlers/comms.go`
- `server/internal/app/server_billing_test.go`
- `server/internal/app/server_comms_test.go`
- `server/internal/app/server_provider_test.go`
- `server/internal/stripe/client.go`
- `.env.example`
- `CLAUDE.md`

Focused verification commands that passed:

```sh
cd server && GOCACHE=/Users/ryan/Documents/aquify/.gocache_local GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache_local go test ./internal/app/... ./internal/stripe ./internal/postmark
```

```sh
cd server && GOCACHE=/Users/ryan/Documents/aquify/.gocache_local GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache_local go test ./cmd/api ./internal/stripe ./internal/postmark ./internal/app/... ./internal/workers/jobhandlers/...
```
