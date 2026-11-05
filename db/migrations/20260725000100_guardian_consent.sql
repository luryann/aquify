-- +goose Up
-- COPPA: a parent's recorded consent to a child having their own swimmer account.
ALTER TABLE swimmers ADD COLUMN guardian_consent_at TIMESTAMPTZ;
ALTER TABLE swimmers ADD COLUMN guardian_consent_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE swimmers ADD COLUMN guardian_consent_version TEXT;

-- +goose Down
ALTER TABLE swimmers DROP COLUMN IF EXISTS guardian_consent_version;
ALTER TABLE swimmers DROP COLUMN IF EXISTS guardian_consent_by;
ALTER TABLE swimmers DROP COLUMN IF EXISTS guardian_consent_at;
