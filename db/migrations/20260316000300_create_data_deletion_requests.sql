-- +goose Up
-- data_deletion_requests tracks GDPR/compliance right-to-erasure (data deletion
-- and anonymisation) jobs for household data.
-- Deletions require admin review before processing.
CREATE TABLE IF NOT EXISTS data_deletion_requests (
    id            TEXT        NOT NULL PRIMARY KEY,
    org_id        TEXT        NOT NULL,
    household_id  TEXT        NOT NULL,
    requested_by  TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    -- status values: pending | reviewing | approved | processing | completed | rejected
    reason        TEXT,
    review_notes  TEXT,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT data_deletion_requests_status_check
        CHECK (status IN ('pending', 'reviewing', 'approved', 'processing', 'completed', 'rejected'))
);

CREATE INDEX IF NOT EXISTS data_deletion_requests_org_idx
    ON data_deletion_requests (org_id, requested_at DESC);

CREATE INDEX IF NOT EXISTS data_deletion_requests_household_idx
    ON data_deletion_requests (household_id, requested_at DESC);

-- +goose Down
DROP TABLE IF EXISTS data_deletion_requests;
