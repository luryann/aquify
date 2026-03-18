-- +goose Up
CREATE TABLE commitments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  meet_id UUID NOT NULL REFERENCES meets(id) ON DELETE CASCADE,
  household_id UUID NOT NULL,
  swimmer_id UUID NOT NULL,
  status TEXT NOT NULL DEFAULT 'invited',
  notes TEXT NOT NULL DEFAULT '',
  committed_at TIMESTAMPTZ,
  coach_reviewed_at TIMESTAMPTZ,
  locked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT commitments_status_check CHECK (
    status IN ('invited','committed_yes','committed_no','coach_reviewed','locked')
  ),
  CONSTRAINT commitments_meet_swimmer_unique UNIQUE (meet_id, swimmer_id)
);

-- Parent event selections per commitment.
-- Records which meet events a parent has selected for their swimmer.
CREATE TABLE commitment_event_selections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  commitment_id UUID NOT NULL REFERENCES commitments(id) ON DELETE CASCADE,
  meet_event_id UUID NOT NULL REFERENCES meet_events(id) ON DELETE CASCADE,
  seed_time_ms INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT commitment_event_selections_unique UNIQUE (commitment_id, meet_event_id)
);

CREATE INDEX commitments_org_id_idx ON commitments (org_id);
CREATE INDEX commitments_meet_id_idx ON commitments (meet_id);
CREATE INDEX commitments_swimmer_id_idx ON commitments (swimmer_id);
CREATE INDEX commitment_event_selections_commitment_id_idx ON commitment_event_selections (commitment_id);

-- +goose Down
DROP TABLE IF EXISTS commitment_event_selections;
DROP TABLE IF EXISTS commitments;
