-- +goose Up

-- Align legacy mfa_factors tables with the schema expected by authn service.
ALTER TABLE IF EXISTS mfa_factors
  ADD COLUMN IF NOT EXISTS totp_secret TEXT;

-- Backfill totp_secret from legacy secret_encrypted for existing TOTP factors.
UPDATE mfa_factors
SET totp_secret = secret_encrypted
WHERE factor_type = 'totp'
  AND secret_encrypted IS NOT NULL
  AND totp_secret IS NULL;

ALTER TABLE IF EXISTS mfa_factors
  ADD COLUMN IF NOT EXISTS backup_codes JSONB;

ALTER TABLE IF EXISTS mfa_factors
  ADD COLUMN IF NOT EXISTS last_challenged_at TIMESTAMPTZ;

UPDATE mfa_factors
SET label = ''
WHERE label IS NULL;

ALTER TABLE IF EXISTS mfa_factors
  ALTER COLUMN label SET DEFAULT '';

ALTER TABLE IF EXISTS mfa_factors
  DROP CONSTRAINT IF EXISTS mfa_factors_factor_identifier_check;

ALTER TABLE IF EXISTS mfa_factors
  DROP CONSTRAINT IF EXISTS mfa_factors_user_id_fkey;

ALTER TABLE IF EXISTS mfa_factors
  ALTER COLUMN id DROP DEFAULT;

ALTER TABLE IF EXISTS mfa_factors
  ALTER COLUMN id TYPE TEXT USING id::text;

ALTER TABLE IF EXISTS mfa_factors
  ALTER COLUMN user_id TYPE TEXT USING user_id::text;

ALTER TABLE IF EXISTS mfa_factors
  ALTER COLUMN org_id TYPE TEXT USING org_id::text;

CREATE INDEX IF NOT EXISTS mfa_factors_user_id_idx ON mfa_factors (user_id);

-- +goose Down
SELECT 1;
