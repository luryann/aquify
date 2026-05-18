-- +goose Up
CREATE TABLE households (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  household_name TEXT NOT NULL,
  primary_email CITEXT,
  primary_phone_e164 TEXT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'archived')),
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX households_org_id_idx ON households (org_id);
CREATE INDEX households_org_status_idx ON households (org_id, status);
CREATE UNIQUE INDEX households_org_id_id_unique_idx ON households (org_id, id);

CREATE TRIGGER households_set_updated_at
BEFORE UPDATE ON households
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS households;
