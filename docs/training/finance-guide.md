# Finance Operations Guide — DARE Platform

**Audience:** Finance staff, club treasurer, billing administrators
**Version:** 1.0
**Last Updated:** 2026-03-23

---

## 1. Invoice Lifecycle

Every invoice in DARE follows a defined state machine:

```
draft → issued → paid
              ↘ partially_paid → paid
              ↘ overdue
```

| State | Meaning | Next Actions |
|---|---|---|
| `draft` | Created but not sent to family | Issue, Edit, Delete |
| `issued` | Sent to family; payment expected | Record payment, Send reminder |
| `partially_paid` | Some payments recorded; balance remains | Record additional payment |
| `paid` | Fully paid | Issue refund if needed |
| `overdue` | Past due date with outstanding balance | Send reminder, escalate |

Invoices can be in the admin portal under **Billing → Invoices**. Use the status filter to find overdue invoices.

---

## 2. Payment Recording

### 2.1 Stripe Payments (Online)

Stripe payments are recorded automatically when the parent completes checkout on the portal. The webhook from Stripe fires `payment_intent.succeeded`, which DARE processes and records on the invoice. No manual action is required for normal Stripe payments.

To verify a Stripe payment was received:
1. Open the invoice in DARE and check the **Payments** tab.
2. Cross-reference with the Stripe Dashboard (Payments → All Transactions).
3. If the payment shows in Stripe but not in DARE, see Section 7 (Common Scenarios: Missing Stripe Payment).

### 2.2 Manual Payments (Check, Cash, ACH)

To record an external payment:
1. Open the invoice.
2. Click **Record Payment**.
3. Fill in:
   - Amount
   - Payment method: `check`, `cash`, or `ach`
   - Reference number (check number or transaction reference)
   - Payment date
4. Click **Save**.
5. If the payment fully covers the invoice balance, the invoice transitions to `paid`.

---

## 3. Refund Process

### 3.1 When Refunds Are Allowed

- Refunds may be issued for any `paid` or `partially_paid` invoice.
- Refund amount cannot exceed the total amount paid on the invoice.
- Refunds require **admin or owner role** plus **MFA verification**.
- Partial refunds are supported.

### 3.2 How to Initiate a Refund

1. Open the invoice.
2. Find the payment to refund in the **Payments** tab.
3. Click **Refund** next to the payment.
4. Enter the refund amount and a mandatory reason.
5. Complete the MFA prompt (TOTP authenticator or backup code).
6. Click **Confirm Refund**.

### 3.3 Stripe vs Manual Refunds

| Payment Type | Refund Behavior |
|---|---|
| Stripe payment | DARE calls Stripe API automatically; the refund appears in the Stripe Dashboard within minutes; the cardholder sees the credit in 5–10 business days |
| Manual payment (check/cash) | DARE records the refund in the ledger; you must issue the physical refund (write a check, return cash) independently |

After a refund, the invoice remains in the system with a `refund` ledger event. The invoice balance is updated.

---

## 4. Monthly Reconciliation

Reconciliation ensures that what DARE records matches what Stripe has processed.

### 4.1 Running the Reconciliation Report

1. Navigate to **Reports → Payment Reconciliation**.
2. Set the date range (first day to last day of the month).
3. Click **Generate Report**.
4. The report shows:
   - Total payments recorded in DARE
   - Total charges in Stripe for the same period
   - Discrepancy amount (if any)
   - Individual unreconciled rows

### 4.2 Interpreting Results

**Expected:** `discrepancy = $0.00` and `unreconciled_count = 0`.

If unreconciled rows are present:
1. Open the unreconciled row to see the Stripe charge ID.
2. Look up the charge in the Stripe Dashboard.
3. Determine if the payment was captured but the webhook failed (see Section 7).
4. Manually record the payment in DARE if confirmed.
5. Re-run the report to verify the discrepancy is resolved.

### 4.3 When to Escalate

Escalate to engineering if:
- The discrepancy is more than $10.
- Multiple consecutive months show the same unreconciled charges.
- The reconciliation report fails to generate (500 error).

---

## 5. Stripe Dashboard

Use the Stripe Dashboard (`https://dashboard.stripe.com`) in parallel with DARE for the following tasks:

| Task | Where to Do It |
|---|---|
| View payment status | Stripe Dashboard → Payments |
| Issue refund manually (emergency) | Stripe Dashboard (also record in DARE) |
| View webhook delivery history | Stripe Dashboard → Developers → Webhooks |
| Check for failed payments | Stripe Dashboard → Payments (filter `failed`) |
| Download Stripe payout reports | Stripe Dashboard → Reporting → Payouts |
| Tax reporting (1099-K) | Stripe Dashboard → Reporting → Tax |

DARE is the system of record for invoices and line items. Stripe is the system of record for payment processing and payouts.

---

## 6. Tax Considerations

DARE does not provide tax advice. Consult your club's accountant. The following data is available for tax preparation:

### 6.1 Exporting Financial Data

- **Invoice aging report** (CSV) — all invoices with amounts, statuses, and dates.
- **Payment reconciliation report** (CSV) — all payments with Stripe charge IDs and amounts.
- **Ledger export** — pull ledger events via the API for a custom date range.

### 6.2 Stripe Tax Reports

Stripe generates a 1099-K if your club processes over the IRS threshold. Download from the Stripe Dashboard → Reporting → Tax Documents.

### 6.3 Relevant Data Fields

For tax preparation, relevant DARE data includes:
- Household name and address (from household records)
- Invoice date and amount (from invoice records)
- Payment date and method (from payment records)
- Refund amounts and dates (from refund records)

---

## 7. Audit Trail for Finance

### 7.1 Pulling Financial Audit Events

All financial actions (invoice creation, payment recording, refund initiation) are logged in the audit trail.

To export audit events for a period:
```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "X-Org-Id: $ORG_ID" \
     "https://api.dare.club/api/v1/orgs/$ORG_ID/audit?category=billing&from=2026-01-01&to=2026-01-31"
```

Or use the admin UI: **Audit → filter by Category: billing**.

Each event records: who performed the action, what changed, the before/after state, and a timestamp.

### 7.2 Immutable Ledger

The `ledger_events` table is append-only. This means:
- No ledger entry can be deleted or modified.
- Every financial action (payment, refund, adjustment) creates a new ledger entry.
- The current invoice balance is the sum of all ledger events for that invoice.

This design ensures a complete, tamper-evident financial history.

---

## 8. Common Scenarios

### 8.1 NSF Check Reversal

When a check bounces (NSF):
1. Do not delete the original payment — the ledger must remain intact.
2. Navigate to the invoice and click **Record Adjustment**.
3. Enter a negative amount equal to the bounced check amount.
4. Reason: "NSF — check #XXXX returned YYYY-MM-DD".
5. The invoice balance will increase back to the original amount.
6. Send a reminder to the family to make payment by alternative method.

### 8.2 Duplicate Payment

If a family is charged twice (e.g., a Stripe retry that was also manually recorded):
1. Identify the duplicate payment in the invoice Payments tab.
2. Issue a refund for the duplicate amount (see Section 3.2).
3. Reason: "Duplicate payment — original charge [charge_id]".
4. Verify the reconciliation report balances after the refund.

### 8.3 Partial Payment Agreement

When a family requests to pay in installments:
1. Open the outstanding invoice.
2. Click **Create Payment Plan**.
3. Specify the number of installments and frequency (weekly, monthly).
4. The system creates sub-invoices for each installment.
5. Track each installment's due date in the invoice list.
6. Record payments as each installment is received.

### 8.4 Missing Stripe Payment (Webhook Missed)

If a parent reports they paid via Stripe but the invoice still shows `issued`:
1. Log in to the Stripe Dashboard.
2. Find the payment intent by the parent's email or card last 4 digits.
3. Verify the payment status is `succeeded`.
4. Note the `payment_intent_id` (format: `pi_XXXX`).
5. Check DARE audit log for any webhook event with that intent ID.
6. If the webhook was missed: manually record the payment in DARE using the Stripe charge amount, method `stripe`, and the payment intent ID as the reference.
7. Notify engineering to investigate the webhook delivery failure.

### 8.5 Refund After Season Close

Refunds can still be issued after the season is closed, as long as the Stripe charge has not expired (typically 180 days). For older charges:
- If within 180 days: issue via Stripe refund.
- If over 180 days: Stripe cannot refund. Issue a manual refund (check) and record as a manual refund in DARE.
