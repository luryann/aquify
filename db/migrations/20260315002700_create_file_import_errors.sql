-- +goose Up
CREATE TABLE file_import_errors (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  import_id     UUID NOT NULL REFERENCES file_imports(id) ON DELETE CASCADE,
  org_id        UUID NOT NULL,
  row_number    INT,
  error_code    TEXT NOT NULL,
  error_message TEXT NOT NULL,
  raw_data      TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT file_import_errors_error_code_check CHECK (
    error_code IN ('unknown_athlete','bad_event_code','duplicate_result','parse_error','type_mismatch','other')
  )
);

CREATE INDEX file_import_errors_import_id_idx    ON file_import_errors (import_id);
CREATE INDEX file_import_errors_error_code_idx   ON file_import_errors (import_id, error_code);

-- +goose Down
DROP TABLE IF EXISTS file_import_errors;
