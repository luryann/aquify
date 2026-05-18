-- +goose Up
CREATE TABLE invoices (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id              UUID NOT NULL,
  household_id        UUID REFERENCES households(id) ON DELETE SET NULL,

  invoice_number      TEXT NOT NULL,
  status              TEXT NOT NULL DEFAULT 'draft',

  -- Financials (all in cents)
  subtotal_cents      INT NOT NULL DEFAULT 0,
  tax_cents           INT NOT NULL DEFAULT 0,
  total_cents         INT NOT NULL DEFAULT 0,
  amount_paid_cents   INT NOT NULL DEFAULT 0,
  amount_due_cents    INT NOT NULL DEFAULT 0,

  -- Dates
  due_date            DATE,
  issued_at           TIMESTAMPTZ,
  voided_at           TIMESTAMPTZ,
  written_off_at      TIMESTAMPTZ,

  notes               TEXT,
  external_ref        TEXT,  -- Stripe invoice ID etc.

  -- Audit
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at          TIMESTAMPTZ,
  created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by          UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by          UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT invoices_status_check CHECK (
    status IN (
      'draft','issued','partially_paid','paid',
      'refunded_partial','refunded_full','voided','written_off'
    )
  ),
  CONSTRAINT invoices_number_org_unique UNIQUE (org_id, invoice_number)
);

CREATE INDEX invoices_org_id_idx         ON invoices (org_id);
CREATE INDEX invoices_household_id_idx   ON invoices (household_id) WHERE household_id IS NOT NULL;
CREATE INDEX invoices_org_status_idx     ON invoices (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS invoices;
