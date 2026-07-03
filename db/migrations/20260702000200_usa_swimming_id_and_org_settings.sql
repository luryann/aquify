-- +goose Up

-- First-class USA-Swimming registration ID on swimmers (distinct from the
-- generic swimmer_external_id, which stays for other/legacy external systems).
ALTER TABLE swimmers ADD COLUMN usa_swimming_id TEXT;

-- USA-Swimming registration numbers are unique per athlete; enforce case-insensitive
-- uniqueness within an org, ignoring blanks.
CREATE UNIQUE INDEX swimmers_org_usa_swimming_id_unique_idx
  ON swimmers (org_id, lower(usa_swimming_id))
  WHERE usa_swimming_id IS NOT NULL AND usa_swimming_id <> '';

-- Org-level export identity used to populate USA-Swimming meet reports:
-- the club's team code, display name, and LSC (Local Swimming Committee) code.
CREATE TABLE org_settings (
  org_id     UUID PRIMARY KEY,
  team_code  TEXT NOT NULL DEFAULT '',
  team_name  TEXT NOT NULL DEFAULT '',
  lsc_code   TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE TRIGGER org_settings_set_updated_at
BEFORE UPDATE ON org_settings
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS org_settings;
DROP INDEX IF EXISTS swimmers_org_usa_swimming_id_unique_idx;
ALTER TABLE swimmers DROP COLUMN IF EXISTS usa_swimming_id;
