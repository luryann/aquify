-- +goose Up
-- Corrective migration: add suppressed_count column to email_campaigns.
-- This column was present in the Go domain model (EmailCampaign.SuppressedCount)
-- and tracked as a required metric in CLAUDE.md but was omitted from the
-- original 20260315004200 migration.
ALTER TABLE email_campaigns
    ADD COLUMN suppressed_count INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE email_campaigns
    DROP COLUMN IF EXISTS suppressed_count;
