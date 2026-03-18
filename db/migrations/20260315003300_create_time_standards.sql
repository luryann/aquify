-- +goose Up
CREATE TABLE time_standards (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID NOT NULL,

  name            TEXT NOT NULL,
  season_year     INT  NOT NULL,
  standard_code   TEXT NOT NULL,  -- AA, A, BB, B, CC, C, etc.
  description     TEXT,

  event_distance  INT  NOT NULL,
  stroke          TEXT NOT NULL,
  course          TEXT NOT NULL,
  gender          TEXT NOT NULL,

  age_min         INT,            -- NULL means no lower bound
  age_max         INT,            -- NULL means no upper bound

  time_ms         INT  NOT NULL,  -- qualifying cut time in milliseconds

  is_active       BOOLEAN NOT NULL DEFAULT TRUE,

  -- Audit
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at      TIMESTAMPTZ,
  created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by      UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by      UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT time_standards_stroke_check CHECK (stroke IN ('free','back','breast','fly','im')),
  CONSTRAINT time_standards_course_check CHECK (course IN ('SCY','SCM','LCM')),
  CONSTRAINT time_standards_gender_check CHECK (gender IN ('M','F','X'))
);

CREATE INDEX time_standards_org_id_idx    ON time_standards (org_id);
CREATE INDEX time_standards_org_year_idx  ON time_standards (org_id, season_year);
CREATE INDEX time_standards_active_idx    ON time_standards (org_id, is_active) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS time_standards;
