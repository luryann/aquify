# STEP 13: Billing Core and Ledger — Implementation Plan

## Overview
Implement the billing core module: DB tables, in-memory service layer, and HTTP endpoints for
invoices, invoice items, payments, refunds, the append-only ledger, and webhook event storage.

## TODO Tasks
- [ ] Create `invoices` table migration
- [ ] Create `invoice_items` table migration
- [ ] Create `payments` table migration
- [ ] Create `refunds` table migration
- [ ] Create `ledger_events` append-only table migration
- [ ] Create `webhook_events` table migration
- [ ] Implement billing service (in-memory, following prior patterns)
- [ ] Implement billing handlers
- [ ] Wire billing service + handlers in server.go
- [ ] Add server billing tests
- [ ] Update CLAUDE.md
- [ ] Update TODO.md

## Schema Design

### invoices
- id, org_id, household_id (FK households), invoice_number TEXT UNIQUE per org
- status TEXT: draft | issued | partially_paid | paid | refunded_partial | refunded_full | voided | written_off
- subtotal_cents INT, tax_cents INT, total_cents INT, amount_paid_cents INT, amount_due_cents INT
- due_date DATE nullable, issued_at TIMESTAMPTZ nullable, voided_at nullable, written_off_at nullable
- notes TEXT, external_ref TEXT (Stripe invoice ID, etc.)
- created_at, updated_at, deleted_at, created_by, updated_by, deleted_by

### invoice_items
- id, org_id, invoice_id (FK invoices)
- description TEXT, quantity INT, unit_amount_cents INT, total_cents INT
- category TEXT (program_fee | meet_fee | equipment | manual)
- swimmer_id FK nullable, meet_id FK nullable
- created_at, updated_at, created_by, updated_by

### payments
- id, org_id, invoice_id (FK invoices)
- amount_cents INT NOT NULL
- method TEXT: stripe | check | cash | ach | other
- status TEXT: pending | succeeded | failed | disputed | reversed
- stripe_payment_intent_id TEXT nullable (idempotency key for Stripe)
- paid_at TIMESTAMPTZ nullable, failed_at TIMESTAMPTZ nullable
- external_ref TEXT, notes TEXT
- created_at, updated_at, created_by, updated_by

### refunds
- id, org_id, invoice_id (FK invoices), payment_id (FK payments)
- amount_cents INT NOT NULL
- reason TEXT NOT NULL
- status TEXT: pending | approved | processed | rejected
- stripe_refund_id TEXT nullable
- processed_at TIMESTAMPTZ nullable, rejected_at TIMESTAMPTZ nullable
- requested_by UUID FK users, approved_by UUID FK users nullable
- created_at, updated_at, created_by, updated_by

### ledger_events (append-only)
- id, org_id, invoice_id (FK invoices nullable), payment_id (FK payments nullable), refund_id (FK refunds nullable)
- event_type TEXT: invoice_issued | payment_succeeded | payment_failed | refund_processed | invoice_voided | invoice_written_off
- amount_cents INT NOT NULL (positive = debit to household; negative = credit)
- description TEXT
- actor_id UUID FK users nullable
- created_at TIMESTAMPTZ NOT NULL (no updated_at — immutable)

### webhook_events
- id, org_id
- provider TEXT: stripe
- event_id TEXT NOT NULL (Stripe event ID — idempotency key)
- event_type TEXT NOT NULL (e.g. payment_intent.succeeded)
- payload JSONB NOT NULL
- processed BOOLEAN NOT NULL DEFAULT FALSE
- processed_at TIMESTAMPTZ nullable
- error_message TEXT nullable
- created_at TIMESTAMPTZ NOT NULL
- UNIQUE(provider, event_id)

## API Endpoints

- `POST   /invoices`                              — create invoice (draft)
- `GET    /invoices/{invoiceID}`                  — get invoice
- `POST   /invoices/{invoiceID}/items`            — add item (draft only)
- `DELETE /invoices/{invoiceID}/items/{itemID}`   — remove item (draft only)
- `POST   /invoices/{invoiceID}/issue`            — issue invoice (draft → issued)
- `POST   /invoices/{invoiceID}/void`             — void invoice (admin only)
- `POST   /invoices/{invoiceID}/write-off`        — write off (admin only)
- `POST   /invoices/{invoiceID}/payments`         — record external payment
- `GET    /invoices/{invoiceID}/payments`         — list payments for invoice
- `POST   /invoices/{invoiceID}/refunds`          — create refund request
- `GET    /invoices/{invoiceID}/refunds`          — list refunds for invoice
- `GET    /households/{householdID}/invoices`     — list household invoices
- `GET    /ledger-events`                         — list ledger events (admin/billing)

## State Machine

Invoice transitions:
- draft → issued (issue)
- issued → partially_paid (payment recorded, partial)
- issued → paid (payment recorded, full)
- partially_paid → paid (additional payment covers balance)
- paid → refunded_partial (partial refund processed)
- paid → refunded_full (full refund processed)
- issued → voided (void action, no payments yet)
- issued | partially_paid → written_off (admin write-off)

Payment transitions:
- pending → succeeded (recorded external; or Stripe webhook)
- pending → failed (Stripe webhook)
- succeeded → reversed (refund processed in full)

## Ledger Writes
- invoice_issued: +total_cents (amount owed)
- payment_succeeded: -amount_cents (reduces balance)
- refund_processed: +amount_cents (increases balance back)
- invoice_voided: -total_cents (cancels the issued debit)
- invoice_written_off: -amount_due_cents (clears remaining balance)

## Role Checks
- Create/issue/void/write-off invoice: owner, admin, billing
- Add/remove invoice items: owner, admin, billing
- Record payment: owner, admin, billing
- Create refund: owner, admin, billing (step-up MFA for admin — noted, not enforced at service layer in this step)
- Read invoices/payments/refunds: owner, admin, billing, parent (own household — household scope enforced)
- Read ledger events: owner, admin, billing
