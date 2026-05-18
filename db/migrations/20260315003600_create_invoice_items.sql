-- +goose Up
CREATE TABLE invoice_items (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            UUID NOT NULL,
  invoice_id        UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,

  description       TEXT NOT NULL,
  quantity          INT NOT NULL DEFAULT 1,
  unit_amount_cents INT NOT NULL,
  total_cents       INT NOT NULL,

  category          TEXT NOT NULL DEFAULT 'manual',

  swimmer_id        UUID REFERENCES swimmers(id) ON DELETE SET NULL,
  meet_id           UUID REFERENCES meets(id)    ON DELETE SET NULL,

  -- Audit
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by        UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT invoice_items_category_check CHECK (
    category IN ('program_fee','meet_fee','equipment','manual','late_fee','credit')
  ),
  CONSTRAINT invoice_items_quantity_positive CHECK (quantity > 0)
);

CREATE INDEX invoice_items_invoice_id_idx ON invoice_items (invoice_id);
CREATE INDEX invoice_items_org_id_idx     ON invoice_items (org_id);

-- +goose Down
DROP TABLE IF EXISTS invoice_items;
