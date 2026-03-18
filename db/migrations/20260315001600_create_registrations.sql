-- +goose Up
CREATE TABLE registrations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  household_id UUID NOT NULL,
  swimmer_id UUID NOT NULL,
  group_id UUID NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
  season TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  registration_date DATE NOT NULL DEFAULT CURRENT_DATE,
  notes TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT registrations_swimmer_group_season_unique UNIQUE (org_id, swimmer_id, group_id, season),
  CONSTRAINT registrations_swimmer_org_fk
    FOREIGN KEY (org_id, swimmer_id) REFERENCES swimmers (org_id, id) ON DELETE CASCADE,
  CONSTRAINT registrations_household_org_fk
    FOREIGN KEY (org_id, household_id) REFERENCES households (org_id, id) ON DELETE CASCADE
);

CREATE INDEX registrations_org_id_idx ON registrations (org_id);
CREATE INDEX registrations_swimmer_id_idx ON registrations (swimmer_id);
CREATE INDEX registrations_household_id_idx ON registrations (household_id);
CREATE INDEX registrations_group_id_idx ON registrations (group_id);
CREATE INDEX registrations_status_idx ON registrations (org_id, status);

CREATE TRIGGER registrations_set_updated_at
BEFORE UPDATE ON registrations
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS registrations;
