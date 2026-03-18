-- +goose Up
CREATE TABLE entry_legs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  entry_id UUID NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
  leg_number INT NOT NULL,
  swimmer_id UUID NOT NULL,
  seed_time_ms INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT entry_legs_entry_leg_unique UNIQUE (entry_id, leg_number)
);

CREATE INDEX entry_legs_org_id_idx ON entry_legs (org_id);
CREATE INDEX entry_legs_entry_id_idx ON entry_legs (entry_id);
CREATE INDEX entry_legs_swimmer_id_idx ON entry_legs (swimmer_id);

-- +goose Down
DROP TABLE IF EXISTS entry_legs;
