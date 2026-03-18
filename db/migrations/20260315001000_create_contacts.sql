-- +goose Up
CREATE TABLE contacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  household_id UUID NOT NULL,
  relationship TEXT NOT NULL,
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  email CITEXT,
  phone_e164 TEXT,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  is_emergency_contact BOOLEAN NOT NULL DEFAULT false,
  can_pick_up BOOLEAN NOT NULL DEFAULT false,
  emergency_priority SMALLINT,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by UUID REFERENCES users(id) ON DELETE SET NULL
  ,
  CONSTRAINT contacts_household_org_fk
    FOREIGN KEY (org_id, household_id) REFERENCES households (org_id, id) ON DELETE CASCADE
);

CREATE INDEX contacts_org_id_idx ON contacts (org_id);
CREATE INDEX contacts_household_id_idx ON contacts (household_id);
CREATE UNIQUE INDEX contacts_household_email_unique_idx
  ON contacts (household_id, email)
  WHERE email IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER contacts_set_updated_at
BEFORE UPDATE ON contacts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS contacts;
