-- +goose Up
-- A purge claims its org (offboarding -> purging) before any irreversible step, so a
-- concurrent cancel-offboarding cannot strand a half-deleted org, and a failed purge can
-- be retried. Access stays off while purging (only 'active' orgs serve requests).
ALTER TABLE orgs DROP CONSTRAINT IF EXISTS orgs_status_check;
ALTER TABLE orgs ADD CONSTRAINT orgs_status_check
    CHECK (status IN ('provisioning', 'active', 'suspended', 'offboarding', 'purging', 'deleted'));

-- +goose Down
UPDATE orgs SET status = 'offboarding' WHERE status = 'purging';
ALTER TABLE orgs DROP CONSTRAINT IF EXISTS orgs_status_check;
ALTER TABLE orgs ADD CONSTRAINT orgs_status_check
    CHECK (status IN ('provisioning', 'active', 'suspended', 'offboarding', 'deleted'));
