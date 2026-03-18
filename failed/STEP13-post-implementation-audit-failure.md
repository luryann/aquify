# STEP 13 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** 13 — Billing Core and Ledger
**Result:** FAIL — Condition #1 not met (bugs/edge cases present)

---

## Conditions Checked

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **FAIL** |
| 2 | No regressions in code quality | Pass |
| 3 | No clear code security issues | Pass |
| 4 | All directions fully completed | Pass (in-scope items complete) |

---

## What Went Wrong

### Bug #1 (CRITICAL) — `CreateRefund` Allows Refunds on `written_off` Invoices

**File:** `server/internal/app/billing/service.go`, function `CreateRefund` (~line 673)

`CreateRefund` does not validate the current invoice status before proceeding. It only checks that the target payment exists and has `status == "succeeded"`. This means an invoice that has been written off (status `written_off`) via `WriteOffInvoice` can still have a refund processed against it if a partial payment was recorded before the write-off.

**Exact exploitable sequence via the live API:**

1. `POST /api/v1/invoices` → create draft invoice
2. Add items, then `POST /api/v1/invoices/{id}/issue` → status: `issued`
3. `POST /api/v1/invoices/{id}/payments` with a partial amount → status: `partially_paid`
4. `POST /api/v1/invoices/{id}/write-off` (admin) → status: `written_off`, `AmountDueCents = 0`
   - The ledger event `invoice_written_off` is written for the remaining balance
   - But `AmountPaidCents` is **not cleared** — the payment remains in `paymentsByID` with `status == "succeeded"`
5. `POST /api/v1/invoices/{id}/refunds` → **SUCCEEDS** (no status guard)
   - `inv.AmountPaidCents -= refundAmount` → reaches 0
   - Status is set to `"refunded_full"` — **an invalid transition from `written_off`**
   - Ledger event `refund_processed` is written — **ledger is now double-counted** (the outstanding balance was already written off, and now a refund is credited on top)

**Result:** The state machine is violated (`written_off → refunded_full` is not a defined transition), and the append-only ledger contains a phantom `refund_processed` credit for money that was already written off. This is a financial integrity bug.

**The state machine as specified:**
```
draft → issued → partially_paid → paid → refunded_partial | refunded_full
                               → voided | written_off
```
Refunds should only flow from `paid`, `partially_paid`, or `refunded_partial`.

---

### Bug #2 (MINOR) — `TestListHouseholdInvoices` Never Tests Household Isolation

**File:** `server/internal/app/server_billing_test.go`, function `TestListHouseholdInvoices`

The test comment says "Create 2 invoices for hh-001 and 1 for hh-002", but no `hh-002` invoice is actually created. The test calls `createInvoice` twice (both for `hh-001`), then asserts `Count == 2`. The test passes, but it never verifies that invoices belonging to another household are excluded from results. Household-scoped data isolation is a critical security property and this test does not validate it.

---

### Non-Bug Issues (Informational)

These do not constitute failures against the four conditions, but are noted for follow-up:

**A. VoidInvoice ledger amount is `−TotalCents` regardless of payments applied**
File: `service.go` line 559. After voiding, the ledger writes `−TotalCents`. This is correct because `VoidInvoice` is guarded to `issued` status only (no payments can exist when status is `issued`). However, the comment on the function says "no payments must exist" — this invariant is implicitly maintained by the status check, not explicitly enforced. When the DB layer is wired, this should be an explicit guard as a defensive measure.

**B. `updateInvoiceStatusAfterPayment` would regress to `issued` if called post-refund**
File: `service.go` line 304. This helper resets status to `issued` when `AmountPaidCents <= 0`. It is correctly called only from `RecordPayment`, not from `CreateRefund`. But the two code paths are not unified — a future developer calling this helper from a refund path would introduce a regression. Low immediate risk, but a latent maintenance trap.

**C. No pagination on billing list endpoints**
All list endpoints (`ListInvoices`, `ListPayments`, `ListRefunds`, `ListLedgerEvents`) return all records without pagination. CLAUDE.md mandates "cursor or offset pagination." The ledger is append-only and will grow monotonically, making `GET /api/v1/ledger-events` unbounded. This is consistent with other in-memory service stubs in prior steps, but is worth calling out for the billing domain specifically.

---

## How to Fix

### Fix for Bug #1 (Required to pass)

In `CreateRefund` in `server/internal/app/billing/service.go`, after retrieving the invoice and before processing the refund, add a status validation guard:

```go
// Add after line 688 (after inv, ok := s.invoices[invoiceID]):
validRefundStatuses := map[string]bool{
    "paid":             true,
    "partially_paid":   true,
    "refunded_partial": true,
}
if !validRefundStatuses[inv.Status] {
    return nil, nil, ErrInvalidTransition
}
```

### Fix for Bug #2 (Recommended)

In `TestListHouseholdInvoices`, add a second invoice creation with `household_id: "hh-002"` and verify that the list for `hh-001` still returns exactly 2 (not 3), proving household isolation:

```go
// Create 1 invoice for hh-002 to verify isolation
createInvoiceForHousehold(t, ts, "stub-admin", "hh-002")

// List hh-001 — must still return 2, not 3
resp = doRequest(t, ts, http.MethodGet, "/api/v1/households/hh-001/invoices", nil, "stub-admin")
// assert count == 2
```

---

## What Counts as Passing

Condition #1 passes when:

1. `CreateRefund` explicitly validates that `inv.Status` is one of `paid`, `partially_paid`, or `refunded_partial` before processing, and returns an appropriate error (e.g., `ErrInvalidTransition` or a new `ErrRefundNotAllowed`) for any other status including `written_off`, `voided`, `draft`, and `issued`.
2. The test `TestListHouseholdInvoices` creates invoices for at least two households and verifies that listing one household's invoices does not include the other's.
3. All existing tests continue to pass (`go test ./...` in `server/`).

---

## Master List of All Files Edited in STEP 13

### New files created:

| File | Description |
|------|-------------|
| `db/migrations/20260315003500_create_invoices.sql` | Invoices table migration |
| `db/migrations/20260315003600_create_invoice_items.sql` | Invoice items table migration |
| `db/migrations/20260315003700_create_payments.sql` | Payments table migration |
| `db/migrations/20260315003800_create_refunds.sql` | Refunds table migration |
| `db/migrations/20260315003900_create_ledger_events.sql` | Append-only ledger events migration |
| `db/migrations/20260315004000_create_webhook_events.sql` | Webhook events table migration |
| `server/internal/app/billing/service.go` | In-memory billing service (invoices, items, payments, refunds, ledger) |
| `server/internal/app/handlers/billing.go` | HTTP handler layer for all billing endpoints |
| `server/internal/app/server_billing_test.go` | Integration tests for billing endpoints |
| `docs/planning/STEP13-implementation-plan.md` | STEP 13 implementation planning trace |

### Modified files:

| File | Change |
|------|--------|
| `server/internal/app/server.go` | Added billing service as 10th parameter to `NewRouterWithServices`; registered all billing routes |
| `CLAUDE.md` | Updated Current Bootstrap Artifacts and STEP 13 status |
| `TODO.md` | Checked off STEP 13 tasks |
