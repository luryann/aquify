-- +goose Up
-- Tenancy tracker item 7.2: when an org entered the offboarding state. Permanent
-- deletion is only allowed once the cooling-off period since this moment has passed.
ALTER TABLE orgs ADD COLUMN offboarding_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE orgs DROP COLUMN IF EXISTS offboarding_at;
