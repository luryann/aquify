-- +goose Up
CREATE TABLE entries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  meet_id UUID NOT NULL REFERENCES meets(id) ON DELETE CASCADE,
  swimmer_id UUID NOT NULL,
  meet_event_id UUID NOT NULL REFERENCES meet_events(id) ON DELETE CASCADE,
  commitment_id UUID REFERENCES commitments(id) ON DELETE SET NULL,
  seed_time_ms INT,
  is_relay BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'draft',
  override_reason TEXT NOT NULL DEFAULT '',
  proposed_by UUID REFERENCES users(id) ON DELETE SET NULL,
  approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT entries_status_check CHECK (
    status IN ('draft','submitted','approved','exported','updated_from_results','archived')
  ),
  CONSTRAINT entries_swimmer_event_unique UNIQUE (meet_id, swimmer_id, meet_event_id)
);

CREATE INDEX entries_org_id_idx ON entries (org_id);
CREATE INDEX entries_meet_id_idx ON entries (meet_id);
CREATE INDEX entries_swimmer_id_idx ON entries (swimmer_id);
CREATE INDEX entries_meet_event_id_idx ON entries (meet_event_id);
CREATE INDEX entries_commitment_id_idx ON entries (commitment_id);

-- +goose Down
DROP TABLE IF EXISTS entries;
