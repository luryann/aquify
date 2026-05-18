-- +goose Up
-- +goose StatementBegin

-- Drop the global unique constraint that prevents reusing names after soft-delete.
ALTER TABLE document_types DROP CONSTRAINT IF EXISTS document_types_org_name_uq;

-- Replace with a partial unique index scoped to active (non-deleted) rows.
-- Case-insensitive via lower() so "Waiver" and "waiver" are treated as the same name.
CREATE UNIQUE INDEX IF NOT EXISTS document_types_org_name_active_uq
    ON document_types (org_id, lower(name))
    WHERE deleted_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS document_types_org_name_active_uq;

ALTER TABLE document_types
    ADD CONSTRAINT document_types_org_name_uq UNIQUE (org_id, name);

-- +goose StatementEnd
