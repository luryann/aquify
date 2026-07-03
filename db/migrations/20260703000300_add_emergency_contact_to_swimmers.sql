-- +goose Up
ALTER TABLE swimmers
  ADD COLUMN emergency_contact_name TEXT,
  ADD COLUMN emergency_contact_phone TEXT,
  ADD COLUMN emergency_contact_relationship TEXT,
  ADD COLUMN allergies TEXT;

-- +goose Down
ALTER TABLE swimmers
  DROP COLUMN IF EXISTS emergency_contact_name,
  DROP COLUMN IF EXISTS emergency_contact_phone,
  DROP COLUMN IF EXISTS emergency_contact_relationship,
  DROP COLUMN IF EXISTS allergies;
