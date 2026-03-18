-- +goose Up
CREATE TABLE refunds (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id           UUID NOT NULL,
  invoice_id       UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
  payment_id       UUID NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,

  amount_cents     INT NOT NULL,
  reason           TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending',

  stripe_refund_id TEXT,
  processed_at     TIMESTAMPTZ,
  rejected_at      TIMESTAMPTZ,

  requested_by     UUID REFERENCES users(id) ON DELETE SET NULL,
  approved_by      UUID REFERENCES users(id) ON DELETE SET NULL,

  -- Audit
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by       UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by       UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT refunds_status_check CHECK (
    status IN ('pending','approved','processed','rejected')
  ),
  CONSTRAINT refunds_amount_positive CHECK (amount_cents > 0)
);

CREATE INDEX refunds_invoice_id_idx ON refunds (invoice_id);
CREATE INDEX refunds_payment_id_idx ON refunds (payment_id);
CREATE INDEX refunds_org_id_idx     ON refunds (org_id);

-- +goose Down
DROP TABLE IF EXISTS refunds;
