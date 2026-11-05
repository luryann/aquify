-- +goose Up
-- Tenancy tracker item 5.4: per-club profile settings. Empty string = "use the
-- platform default" (UTC / the deployment's STRIPE_CURRENCY / the club's team name).
ALTER TABLE org_settings
    ADD COLUMN timezone   TEXT NOT NULL DEFAULT '',
    ADD COLUMN currency   TEXT NOT NULL DEFAULT '',
    ADD COLUMN legal_name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE org_settings
    DROP COLUMN IF EXISTS legal_name,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS timezone;
