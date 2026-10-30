-- +goose Up
-- Org-level USA Swimming (SWIMS / Data Hub) connection settings. The API key is
-- stored AES-GCM encrypted (api_key_encrypted) and is never returned by the API.
CREATE TABLE usa_swimming_integrations (
  org_id            UUID PRIMARY KEY,
  base_url          TEXT NOT NULL DEFAULT '',
  api_key_encrypted TEXT NOT NULL DEFAULT '',
  api_key_hint      TEXT NOT NULL DEFAULT '',
  enabled           BOOLEAN NOT NULL DEFAULT FALSE,
  last_test_at      TIMESTAMPTZ,
  last_test_ok      BOOLEAN NOT NULL DEFAULT FALSE,
  last_test_message TEXT NOT NULL DEFAULT '',
  last_sync_at      TIMESTAMPTZ,
  last_sync_message TEXT NOT NULL DEFAULT '',
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by        UUID REFERENCES users(id) ON DELETE SET NULL
);

-- +goose Down
DROP TABLE IF EXISTS usa_swimming_integrations;
