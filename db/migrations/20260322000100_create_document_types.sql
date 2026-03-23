-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS document_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,

    -- Human-readable name, e.g. "Season Liability Waiver"
    name        TEXT NOT NULL,

    -- category: waiver | medical | insurance | release | registration | other
    category    TEXT NOT NULL DEFAULT 'other',

    description TEXT,

    -- Whether every household must have this document on file
    is_required BOOLEAN NOT NULL DEFAULT true,

    -- applies_to: swimmer | household | all
    applies_to  TEXT NOT NULL DEFAULT 'swimmer',

    -- How many months before the document expires (NULL = never)
    expiry_months INTEGER,

    is_active   BOOLEAN NOT NULL DEFAULT true,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  TEXT NOT NULL DEFAULT '',
    updated_by  TEXT NOT NULL DEFAULT '',
    deleted_at  TIMESTAMPTZ,
    deleted_by  TEXT,

    CONSTRAINT document_types_org_name_uq UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS document_types_org_id_idx ON document_types (org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS document_types;
-- +goose StatementEnd
