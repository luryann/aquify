-- +goose Up
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
    file_type IN ('cl2','sd3','zip','unknown')
  ),
  CONSTRAINT file_imports_status_check CHECK (
    status IN ('uploaded','scanning','parsing','matching','conflicts_pending','applied','failed','archived')
  )
);

CREATE INDEX file_imports_org_id_idx    ON file_imports (org_id);
CREATE INDEX file_imports_org_status_idx ON file_imports (org_id, status);
CREATE INDEX file_imports_meet_id_idx   ON file_imports (meet_id) WHERE meet_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS file_imports;
