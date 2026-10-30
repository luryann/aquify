-- +goose Up
ALTER TABLE org_settings ADD COLUMN swimmer_accounts_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE swimmers ADD COLUMN portal_access_disabled BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE swimmers DROP COLUMN IF EXISTS portal_access_disabled;
ALTER TABLE org_settings DROP COLUMN IF EXISTS swimmer_accounts_enabled;
