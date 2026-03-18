-- +goose Up
CREATE TABLE meets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  name TEXT NOT NULL,
  meet_type TEXT NOT NULL DEFAULT 'invitational',
  course TEXT NOT NULL DEFAULT 'SCY',
  start_date DATE,
  end_date DATE,
  entry_deadline TIMESTAMPTZ,
  commitment_deadline TIMESTAMPTZ,
  host_org TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  commitment_open_at TIMESTAMPTZ,
  commitment_closed_at TIMESTAMPTZ,
  entry_finalized_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT meets_status_check CHECK (
    status IN ('draft','published','commitment_open','commitment_closed','entry_finalized','completed','archived')
  ),
  CONSTRAINT meets_meet_type_check CHECK (
    meet_type IN ('invitational','dual_meet','time_trial','championship','other')
  ),
  CONSTRAINT meets_course_check CHECK (
    course IN ('SCY','SCM','LCM')
  )
);

CREATE INDEX meets_org_id_idx ON meets (org_id);
CREATE INDEX meets_org_status_idx ON meets (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS meets;
