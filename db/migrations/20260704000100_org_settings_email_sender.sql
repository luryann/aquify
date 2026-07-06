-- +goose Up

-- Org-level default sender identity for email campaigns, so campaign creation
-- stops defaulting to a hardcoded third-party club name/address.
ALTER TABLE org_settings ADD COLUMN email_sender_name TEXT NOT NULL DEFAULT '';
ALTER TABLE org_settings ADD COLUMN email_sender_address TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE org_settings DROP COLUMN IF EXISTS email_sender_address;
ALTER TABLE org_settings DROP COLUMN IF EXISTS email_sender_name;
