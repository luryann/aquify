-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS document_submissions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID NOT NULL,
    document_type_id   UUID NOT NULL REFERENCES document_types (id),
    household_id       UUID NOT NULL,
    -- NULL when applies_to = 'household'
    swimmer_id         UUID,

    -- status: pending | submitted | verified | rejected | expired
    status             TEXT NOT NULL DEFAULT 'pending',

    -- Object storage reference for the uploaded file
    file_object_key    TEXT,
    file_name          TEXT,
    file_size_bytes    BIGINT,
    file_mime          TEXT,

    -- Notes from the submitter / parent
    notes              TEXT,
    -- Review notes from the admin who verified/rejected
    review_notes       TEXT,

    submitted_at       TIMESTAMPTZ,
    verified_at        TIMESTAMPTZ,
    verified_by        TEXT,

    -- Computed expiry from document_type.expiry_months at submission time
    expires_at         TIMESTAMPTZ,

    -- Track when the last expiry reminder email was sent
    reminder_sent_at   TIMESTAMPTZ,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by         TEXT NOT NULL DEFAULT '',
    updated_by         TEXT NOT NULL DEFAULT '',
    deleted_at         TIMESTAMPTZ,
    deleted_by         TEXT
);

CREATE INDEX IF NOT EXISTS document_submissions_org_id_idx         ON document_submissions (org_id);
CREATE INDEX IF NOT EXISTS document_submissions_household_id_idx   ON document_submissions (household_id);
CREATE INDEX IF NOT EXISTS document_submissions_swimmer_id_idx     ON document_submissions (swimmer_id) WHERE swimmer_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS document_submissions_document_type_idx  ON document_submissions (document_type_id);
CREATE INDEX IF NOT EXISTS document_submissions_expires_at_idx     ON document_submissions (expires_at) WHERE expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS document_submissions;
-- +goose StatementEnd
