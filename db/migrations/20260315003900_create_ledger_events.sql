-- +goose Up
-- Append-only ledger: no updated_at, no deletes, no soft-delete.
CREATE TABLE ledger_events (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id       UUID NOT NULL,

  -- Source references (one is typically set)
  invoice_id   UUID REFERENCES invoices(id) ON DELETE SET NULL,
  payment_id   UUID REFERENCES payments(id) ON DELETE SET NULL,
  refund_id    UUID REFERENCES refunds(id)  ON DELETE SET NULL,

  event_type   TEXT NOT NULL,
  -- Positive = amount owed/debited to household; negative = credit to household
  amount_cents INT NOT NULL,
  description  TEXT,
  actor_id     UUID REFERENCES users(id) ON DELETE SET NULL,

  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT ledger_events_type_check CHECK (
    event_type IN (
      'invoice_issued',
      'payment_succeeded',
      'payment_failed',
      'refund_processed',
      'invoice_voided',
      'invoice_written_off'
    )
  )
);

CREATE INDEX ledger_events_org_id_idx       ON ledger_events (org_id);
CREATE INDEX ledger_events_invoice_id_idx   ON ledger_events (invoice_id) WHERE invoice_id IS NOT NULL;
CREATE INDEX ledger_events_org_created_idx  ON ledger_events (org_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS ledger_events;
