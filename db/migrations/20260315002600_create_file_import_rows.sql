-- +goose Up
CREATE TABLE file_import_rows (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  import_id      UUID NOT NULL REFERENCES file_imports(id) ON DELETE CASCADE,
  org_id         UUID NOT NULL,
  row_number     INT NOT NULL,
  record_type    TEXT,
  raw_data       TEXT NOT NULL DEFAULT '',
  parsed_data    JSONB,
  status         TEXT NOT NULL DEFAULT 'pending',
  swimmer_id     UUID REFERENCES swimmers(id) ON DELETE SET NULL,
  meet_event_id  UUID REFERENCES meet_events(id) ON DELETE SET NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT file_import_rows_status_check CHECK (
    status IN ('pending','matched','conflict','error')
  )
);

CREATE INDEX file_import_rows_import_id_idx     ON file_import_rows (import_id);
CREATE INDEX file_import_rows_swimmer_id_idx    ON file_import_rows (swimmer_id) WHERE swimmer_id IS NOT NULL;
CREATE UNIQUE INDEX file_import_rows_import_row_idx ON file_import_rows (import_id, row_number);

-- +goose Down
DROP TABLE IF EXISTS file_import_rows;
