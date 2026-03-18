-- +goose Up
CREATE TABLE meet_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  meet_id UUID NOT NULL REFERENCES meets(id) ON DELETE CASCADE,
  session_number INT NOT NULL,
  name TEXT NOT NULL,
  session_date DATE,
  warmup_time TIME,
  start_time TIME,
  notes TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT meet_sessions_meet_number_unique UNIQUE (meet_id, session_number)
);

CREATE INDEX meet_sessions_org_id_idx ON meet_sessions (org_id);
CREATE INDEX meet_sessions_meet_id_idx ON meet_sessions (meet_id);

-- +goose Down
DROP TABLE IF EXISTS meet_sessions;
