-- +goose Up
CREATE TABLE meet_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  meet_id UUID NOT NULL REFERENCES meets(id) ON DELETE CASCADE,
  session_id UUID REFERENCES meet_sessions(id) ON DELETE SET NULL,
  event_number TEXT NOT NULL,
  stroke TEXT NOT NULL,
  distance INT NOT NULL,
  course TEXT NOT NULL DEFAULT 'SCY',
  gender TEXT NOT NULL DEFAULT 'O',
  min_age INT,
  max_age INT,
  entry_fee_cents INT NOT NULL DEFAULT 0,
  is_relay BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT meet_events_stroke_check CHECK (
    stroke IN ('freestyle','backstroke','breaststroke','butterfly','individual_medley','medley_relay','freestyle_relay','other')
  ),
  CONSTRAINT meet_events_gender_check CHECK (
    gender IN ('M','F','X','O')
  ),
  CONSTRAINT meet_events_course_check CHECK (
    course IN ('SCY','SCM','LCM')
  ),
  CONSTRAINT meet_events_meet_event_number_unique UNIQUE (meet_id, event_number)
);

CREATE INDEX meet_events_org_id_idx ON meet_events (org_id);
CREATE INDEX meet_events_meet_id_idx ON meet_events (meet_id);
CREATE INDEX meet_events_session_id_idx ON meet_events (session_id);

-- +goose Down
DROP TABLE IF EXISTS meet_events;
