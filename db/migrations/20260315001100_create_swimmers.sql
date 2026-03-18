-- +goose Up
CREATE TABLE swimmers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  household_id UUID NOT NULL,
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  preferred_name TEXT,
  date_of_birth DATE NOT NULL,
  gender TEXT NOT NULL CHECK (gender IN ('female', 'male', 'non_binary', 'undisclosed')),
  swimmer_external_id TEXT,
  registration_status TEXT NOT NULL DEFAULT 'active' CHECK (registration_status IN ('pending', 'active', 'inactive', 'archived')),
  medical_flags JSONB NOT NULL DEFAULT '{}'::JSONB,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by UUID REFERENCES users(id) ON DELETE SET NULL
  ,
  CONSTRAINT swimmers_household_org_fk
    FOREIGN KEY (org_id, household_id) REFERENCES households (org_id, id) ON DELETE CASCADE
);

CREATE INDEX swimmers_org_id_idx ON swimmers (org_id);
CREATE INDEX swimmers_household_id_idx ON swimmers (household_id);
CREATE INDEX swimmers_org_name_idx ON swimmers (org_id, last_name, first_name);
CREATE INDEX swimmers_org_registration_status_idx ON swimmers (org_id, registration_status);
CREATE UNIQUE INDEX swimmers_org_id_id_unique_idx ON swimmers (org_id, id);

CREATE TRIGGER swimmers_set_updated_at
BEFORE UPDATE ON swimmers
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS swimmers;
