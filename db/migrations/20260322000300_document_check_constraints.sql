-- +goose Up
-- +goose StatementBegin
ALTER TABLE document_submissions
  ADD CONSTRAINT document_submissions_status_chk
    CHECK (status IN ('pending','submitted','verified','rejected','expired'));

ALTER TABLE document_types
  ADD CONSTRAINT document_types_applies_to_chk
    CHECK (applies_to IN ('household','swimmer','all'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE document_submissions DROP CONSTRAINT IF EXISTS document_submissions_status_chk;
ALTER TABLE document_types DROP CONSTRAINT IF EXISTS document_types_applies_to_chk;
-- +goose StatementEnd
