# STEP 21 Post-Implementation Audit Failure

## Summary

STEP 21 does not pass the required audit bar.

Condition 1 is not met: there is a real edge-case bug in the payment reconciliation report.

Because at least one required condition is not well-met, this step must be treated as failed pending remediation.

## 1) What Went Wrong

### Bug: `stripe_reported_cents=0` cannot be represented correctly

Files involved:
- `server/internal/app/reports/service.go`
- `server/internal/app/handlers/reports.go`

The payment reconciliation flow parses `stripe_reported_cents` as an `int` and uses `0` both for:

- query parameter absent
- query parameter explicitly provided as `0`

That means the report cannot distinguish:

- "no Stripe total was supplied"
- "Stripe says the total is zero"

Current behavior:

- `ParseStripeReportedCents("")` returns `0`
- `ParseStripeReportedCents("0")` also returns `0`
- `RunPaymentReconciliationReport(...)` only computes Stripe mismatch data when `stripeReportedCents > 0`

Result:

- a legitimate zero-total reconciliation request is silently treated as if no Stripe comparison was requested
- the response omits `stripe_reported_cents`, `stripe_mismatch_cents`, and `stripe_alert_triggered`
- this weakens auditability for an important financial edge case

This is not a theoretical concern. Zero-total windows are normal when:

- filtering to a quiet period
- reconciling a newly created org with no Stripe collections yet
- validating that ledger totals were fully reversed after refunds or test cleanup

## 2) How To Fix It

Use an "optional value" representation instead of collapsing absence and zero together.

Recommended fix:

1. Change the query parsing layer to return `(value int, provided bool, err error)` or `(*int, error)`.
2. Update `runPaymentReconciliation` to preserve whether the caller actually supplied `stripe_reported_cents`.
3. Update `RunPaymentReconciliationReport` so Stripe mismatch logic runs whenever the parameter was provided, including `0`.
4. Add tests for:
   - omitted `stripe_reported_cents`
   - `stripe_reported_cents=0`
   - positive Stripe total
   - invalid negative/non-integer values

One safe shape would be:

```go
func ParseStripeReportedCents(raw string) (*int, error)
```

Then:

- `nil` means "not provided"
- `0` means "provided and zero"

## 3) What Counts As Passing

STEP 21 can be considered passing only when all of the following are true:

1. `stripe_reported_cents=0` produces a report that includes Stripe comparison fields and computes mismatch correctly.
2. Omitting `stripe_reported_cents` still behaves as "no Stripe comparison requested."
3. Targeted Step 21 report tests pass, including the new zero-total test.
4. No new authorization, CSV, or report-shape regressions are introduced.
5. The implementation still matches the Step 21 deliverables:
   - five report endpoints
   - JSON and CSV output
   - audit event emission
   - stable report contracts/docs

## 4) Master List Of Files Edited By Me In STEP 21

- `CLAUDE.md`
- `docs/planning/STEP21-implementation-plan.md`
- `docs/planning/STEP21-report-view-definitions.md`
- `server/internal/app/audit/service.go`
- `server/internal/app/handlers/reports.go`
- `server/internal/app/reports/csv.go`
- `server/internal/app/reports/service.go`
- `server/internal/app/server.go`
- `server/internal/app/server_reports_test.go`

## Verification Notes

What I verified during this audit:

- Read `design-language-guide.html`
- Read `docs/planning/Developer Handoff Master Document.md`
- Read `docs/planning/Plan.md`
- Read `CLAUDE.md`
- Read `TODO.md`
- Read the STEP 21 implementation docs and the STEP 21 code surface listed above
- `env GOCACHE=/tmp/go-build-cache go test ./internal/app -run 'TestReports|TestPaymentReconciliation'` passed
- `env GOCACHE=/tmp/go-build-cache-meetfiles go test ./...` in `internal/meetfiles` passed

Environment limitations encountered during audit:

- Broad Go test runs hit sandbox restrictions in unrelated auth tests using `httptest.NewServer`
- `npm run build` in `admin/` failed because the sandbox could not resolve `fonts.googleapis.com`

Those environment issues do not change the main audit outcome above: STEP 21 fails because of the zero-total reconciliation bug.
