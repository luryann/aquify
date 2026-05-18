-- +goose Up
ALTER TABLE IF EXISTS sessions
  ADD COLUMN IF NOT EXISTS access_token_hash TEXT;

ALTER TABLE IF EXISTS sessions
  ADD COLUMN IF NOT EXISTS principal JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE IF EXISTS sessions
  ADD COLUMN IF NOT EXISTS id_token TEXT;

UPDATE sessions
SET access_token_hash = COALESCE(access_token_hash, 'legacy-' || id::text)
WHERE access_token_hash IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS sessions_access_token_hash_key
  ON sessions (access_token_hash);

-- +goose Down
DROP INDEX IF EXISTS sessions_access_token_hash_key;

ALTER TABLE IF EXISTS sessions
  DROP COLUMN IF EXISTS id_token;

ALTER TABLE IF EXISTS sessions
  DROP COLUMN IF EXISTS principal;

ALTER TABLE IF EXISTS sessions
  DROP COLUMN IF EXISTS access_token_hash;
