-- +goose Up
-- The file-imports domain (upload -> parse -> swimmer-match -> conflict
-- resolve -> apply) is dead: no frontend surface ever created an import,
-- meet creation uses the separate parse-entry-file flow instead, and even
-- the "apply" step never wrote results/best-times anywhere. Removing the
-- backing tables now that the Go service/handlers/worker wiring is deleted.
--
-- Drop order respects FKs: match_conflicts, file_import_rows, and
-- file_import_errors all reference file_imports(id) ON DELETE CASCADE, so
-- they must go first.
DROP TABLE IF EXISTS match_conflicts;
DROP TABLE IF EXISTS file_import_rows;
DROP TABLE IF EXISTS file_import_errors;
DROP TABLE IF EXISTS file_imports;

-- +goose Down
-- Best-effort reversal: restores the table shapes as they existed
-- immediately before this migration (including the hy3/hyv/ev3 file_type
-- expansion from 20260326000200_expand_file_import_types.sql). No data can
-- be recovered.
CREATE TABLE file_imports (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id                 UUID NOT NULL,
  meet_id                UUID REFERENCES meets(id) ON DELETE SET NULL,
  filename               TEXT NOT NULL,
  file_type              TEXT NOT NULL DEFAULT 'unknown',
  status                 TEXT NOT NULL DEFAULT 'uploaded',
  storage_key            TEXT,
  upload_url             TEXT,
  upload_url_expires_at  TIMESTAMPTZ,
  row_count              INT NOT NULL DEFAULT 0,
  error_count            INT NOT NULL DEFAULT 0,
  conflict_count         INT NOT NULL DEFAULT 0,
  import_report          JSONB,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at             TIMESTAMPTZ,
  created_by             UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by             UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by             UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT file_imports_file_type_check CHECK (
    file_type IN ('cl2','sd3','zip','hy3','hyv','ev3','unknown')
  ),
  CONSTRAINT file_imports_status_check CHECK (
    status IN ('uploaded','scanning','parsing','matching','conflicts_pending','applied','failed','archived')
  )
);

CREATE INDEX file_imports_org_id_idx    ON file_imports (org_id);
CREATE INDEX file_imports_org_status_idx ON file_imports (org_id, status);
CREATE INDEX file_imports_meet_id_idx   ON file_imports (meet_id) WHERE meet_id IS NOT NULL;

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

CREATE TABLE match_conflicts (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  import_id              UUID NOT NULL REFERENCES file_imports(id) ON DELETE CASCADE,
  org_id                 UUID NOT NULL,
  row_number             INT,
  raw_data               TEXT,
  status                 TEXT NOT NULL DEFAULT 'open',
  conflict_type          TEXT NOT NULL,
  candidate_swimmer_ids  UUID[] NOT NULL DEFAULT '{}',
  resolved_swimmer_id    UUID REFERENCES swimmers(id) ON DELETE SET NULL,
  resolver_id            UUID REFERENCES users(id) ON DELETE SET NULL,
  resolution_note        TEXT,
  resolved_at            TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT match_conflicts_status_check CHECK (
    status IN ('open','resolved_manual','resolved_auto','rejected','closed')
  ),
  CONSTRAINT match_conflicts_conflict_type_check CHECK (
    conflict_type IN ('unknown_athlete','ambiguous_match','duplicate_result')
  )
);

CREATE INDEX match_conflicts_import_id_idx  ON match_conflicts (import_id);
CREATE INDEX match_conflicts_org_status_idx ON match_conflicts (org_id, status);
