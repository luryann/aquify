-- +goose Up
ALTER TABLE billing_settings
  ADD COLUMN meet_fee_per_event_cents INT NOT NULL DEFAULT 0 CHECK (meet_fee_per_event_cents BETWEEN 0 AND 10000000),
  ADD COLUMN meet_fee_flat_cents      INT NOT NULL DEFAULT 0 CHECK (meet_fee_flat_cents BETWEEN 0 AND 10000000),
  ADD COLUMN meet_fee_invoice_status  TEXT NOT NULL DEFAULT 'draft' CHECK (meet_fee_invoice_status IN ('draft','issued')),
  ADD COLUMN meet_fee_due_days        INT NOT NULL DEFAULT 15 CHECK (meet_fee_due_days BETWEEN 0 AND 120);

-- +goose Down
ALTER TABLE billing_settings
  DROP COLUMN IF EXISTS meet_fee_due_days,
  DROP COLUMN IF EXISTS meet_fee_invoice_status,
  DROP COLUMN IF EXISTS meet_fee_flat_cents,
  DROP COLUMN IF EXISTS meet_fee_per_event_cents;
