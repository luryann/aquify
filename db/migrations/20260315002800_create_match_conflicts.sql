-- +goose Up
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

-- +goose Down
DROP TABLE IF EXISTS match_conflicts;
