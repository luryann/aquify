-- +goose Up
CREATE TABLE file_exports (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id                  UUID NOT NULL,
  meet_id                 UUID REFERENCES meets(id) ON DELETE SET NULL,
  export_type             TEXT NOT NULL,
  status                  TEXT NOT NULL DEFAULT 'pending',
  storage_key             TEXT,
  download_url            TEXT,
  download_url_expires_at TIMESTAMPTZ,
  entry_count             INT NOT NULL DEFAULT 0,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at              TIMESTAMPTZ,
  created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by              UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by              UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT file_exports_export_type_check CHECK (
    export_type IN ('standard_sd3','extended_sd3','sdif_roster')
  ),
  CONSTRAINT file_exports_status_check CHECK (
    status IN ('pending','generating','ready','failed','expired')
  )
);

CREATE INDEX file_exports_org_id_idx    ON file_exports (org_id);
CREATE INDEX file_exports_meet_id_idx   ON file_exports (meet_id) WHERE meet_id IS NOT NULL;
CREATE INDEX file_exports_org_status_idx ON file_exports (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS file_exports;
