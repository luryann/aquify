-- +goose Up
-- Vendor API (OAuth client credentials). api_key_encrypted now holds the client secret
-- when token_url is set; legacy rows (token_url = '') keep bearer-key behaviour.
ALTER TABLE usa_swimming_integrations
  ADD COLUMN token_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN client_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN club_id   TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE usa_swimming_integrations DROP COLUMN IF EXISTS token_url, DROP COLUMN IF EXISTS client_id, DROP COLUMN IF EXISTS club_id;
