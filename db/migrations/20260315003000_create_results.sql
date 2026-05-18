-- +goose Up
CREATE TABLE results (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            UUID NOT NULL,
  swimmer_id        UUID REFERENCES swimmers(id) ON DELETE SET NULL,
  meet_id           UUID REFERENCES meets(id) ON DELETE SET NULL,
  meet_event_id     UUID REFERENCES meet_events(id) ON DELETE SET NULL,
  import_id         UUID REFERENCES file_imports(id) ON DELETE SET NULL,

  -- Event descriptor (redundant with meet_event_id but preserved for stand-alone results)
  event_number      TEXT,
  distance          INT NOT NULL,
  stroke            TEXT NOT NULL,
  course            TEXT NOT NULL,
  gender            TEXT NOT NULL,

  -- Heat sheet metadata
  heat              INT,
  lane              INT,
  place             INT,

  -- Times
  seed_time_ms      INT,
  fin_time_ms       INT,

  -- Outcome
  dq                BOOLEAN NOT NULL DEFAULT FALSE,
  dq_code           TEXT,
  dq_note           TEXT,

  -- Provenance
  is_official       BOOLEAN NOT NULL DEFAULT TRUE,
  is_manual_override BOOLEAN NOT NULL DEFAULT FALSE,
  override_reason   TEXT,
  override_by       UUID REFERENCES users(id) ON DELETE SET NULL,

  status            TEXT NOT NULL DEFAULT 'active',

  -- Audit
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ,
  created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by        UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by        UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT results_course_check CHECK (course IN ('SCY','SCM','LCM')),
  CONSTRAINT results_gender_check CHECK (gender IN ('M','F','X')),
  CONSTRAINT results_stroke_check CHECK (stroke IN ('free','back','breast','fly','im','free_relay','medley_relay')),
  CONSTRAINT results_status_check CHECK (status IN ('active','voided','archived'))
);

CREATE INDEX results_org_id_idx       ON results (org_id);
CREATE INDEX results_swimmer_id_idx   ON results (swimmer_id) WHERE swimmer_id IS NOT NULL;
CREATE INDEX results_meet_id_idx      ON results (meet_id)    WHERE meet_id IS NOT NULL;
CREATE INDEX results_import_id_idx    ON results (import_id)  WHERE import_id IS NOT NULL;
CREATE INDEX results_org_swimmer_idx  ON results (org_id, swimmer_id, stroke, distance, course);

-- +goose Down
DROP TABLE IF EXISTS results;
