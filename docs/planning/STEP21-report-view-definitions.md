# STEP 21 Report View Definitions

These definitions capture the canonical "view" shape for the core v1 reporting pack while the app still runs on in-memory domain services. When the project moves to a SQL-backed reporting layer, these should become database views or equivalent materialized/reporting queries without changing the API contract.

## `report_membership_status_v1`
- Grain: one row per swimmer
- Source domains: households, contacts, swimmers, swimmer-group links, groups
- Columns:
  - `household_id`
  - `household_name`
  - `household_status`
  - `primary_email`
  - `primary_phone_e164`
  - `swimmer_id`
  - `swimmer_name`
  - `preferred_name`
  - `registration_status`
  - `primary_group_code`
  - `primary_group_name`
  - `group_count`
  - `last_profile_update_at`

## `report_meet_fees_v1`
- Grain: one row per meet-fee invoice item
- Source domains: invoices, invoice_items, meets, swimmers
- Filter: invoice item category = `meet_fee`
- Columns:
  - `invoice_id`
  - `invoice_number`
  - `invoice_status`
  - `household_id`
  - `meet_id`
  - `meet_name`
  - `swimmer_id`
  - `swimmer_name`
  - `line_item_id`
  - `line_description`
  - `quantity`
  - `unit_amount_cents`
  - `total_cents`
  - `due_date`
  - `issued_at`

## `report_invoice_aging_v1`
- Grain: one row per invoice
- Source domains: invoices
- Derived fields:
  - `days_past_due`
  - `aging_bucket`
  - `is_outstanding`
- Columns:
  - `invoice_id`
  - `invoice_number`
  - `household_id`
  - `status`
  - `due_date`
  - `days_past_due`
  - `aging_bucket`
  - `total_cents`
  - `amount_paid_cents`
  - `amount_due_cents`
  - `is_outstanding`

## `report_payment_reconciliation_v1`
- Grain: one row per payment
- Source domains: payments, refunds, ledger_events, invoices
- Derived checks:
  - payment ledger total matches payment amount
  - refund ledger total matches refund total
  - stripe aggregate mismatch may be attached when caller provides `stripe_reported_cents`
- Columns:
  - `invoice_id`
  - `invoice_number`
  - `payment_id`
  - `method`
  - `payment_status`
  - `payment_amount_cents`
  - `refunded_cents`
  - `net_collected_cents`
  - `ledger_payment_cents`
  - `ledger_refund_cents`
  - `is_reconciled`
  - `reconciliation_notes`

## `report_registration_status_v1`
- Grain: one row per registration
- Source domains: registrations, waiver acceptances, groups, households, swimmers
- Columns:
  - `registration_id`
  - `season`
  - `status`
  - `registration_date`
  - `group_id`
  - `group_code`
  - `group_name`
  - `household_id`
  - `household_name`
  - `swimmer_id`
  - `swimmer_name`
  - `waiver_count`
  - `last_waiver_accepted_at`
