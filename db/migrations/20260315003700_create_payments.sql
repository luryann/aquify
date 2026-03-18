-- +goose Up
CREATE TABLE payments (
  id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id                    UUID NOT NULL,
  invoice_id                UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,

  amount_cents              INT NOT NULL,
  method                    TEXT NOT NULL DEFAULT 'other',
  status                    TEXT NOT NULL DEFAULT 'pending',

  stripe_payment_intent_id  TEXT,  -- Stripe idempotency key
  paid_at                   TIMESTAMPTZ,
  failed_at                 TIMESTAMPTZ,

  external_ref              TEXT,
  notes                     TEXT,

  -- Audit
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by                UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by                UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT payments_method_check CHECK (
    method IN ('stripe','check','cash','ach','other')
  ),
  CONSTRAINT payments_status_check CHECK (
    status IN ('pending','succeeded','failed','disputed','reversed')
  ),
  CONSTRAINT payments_amount_positive CHECK (amount_cents > 0)
);

CREATE INDEX payments_invoice_id_idx             ON payments (invoice_id);
CREATE INDEX payments_org_id_idx                 ON payments (org_id);
CREATE UNIQUE INDEX payments_stripe_intent_unique ON payments (stripe_payment_intent_id)
  WHERE stripe_payment_intent_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS payments;
