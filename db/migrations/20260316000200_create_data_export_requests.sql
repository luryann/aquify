-- +goose Up
-- data_export_requests tracks GDPR/compliance household-data export jobs.
-- Exports are generated asynchronously; download_url is populated when ready.
CREATE TABLE IF NOT EXISTS data_export_requests (
    id            TEXT        NOT NULL PRIMARY KEY,
    org_id        TEXT        NOT NULL,
    household_id  TEXT        NOT NULL,
    requested_by  TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    -- status values: pending | processing | ready | failed | expired
    download_url  TEXT,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT data_export_requests_status_check
        CHECK (status IN ('pending', 'processing', 'ready', 'failed', 'expired'))
);

CREATE INDEX IF NOT EXISTS data_export_requests_org_idx
    ON data_export_requests (org_id, requested_at DESC);

CREATE INDEX IF NOT EXISTS data_export_requests_household_idx
    ON data_export_requests (household_id, requested_at DESC);

-- +goose Down
DROP TABLE IF EXISTS data_export_requests;
