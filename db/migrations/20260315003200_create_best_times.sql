-- +goose Up
CREATE TABLE best_times (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id              UUID NOT NULL,
  swimmer_id          UUID NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,

  event_distance      INT NOT NULL,
  stroke              TEXT NOT NULL,
  course              TEXT NOT NULL,

  best_time_ms        INT,           -- NULL means no time on record (NT)
  result_id           UUID REFERENCES results(id) ON DELETE SET NULL,
  recorded_at         TIMESTAMPTZ,   -- date/time of the swim that set this BT

  is_manual_override  BOOLEAN NOT NULL DEFAULT FALSE,
  override_reason     TEXT,
  override_by         UUID REFERENCES users(id) ON DELETE SET NULL,

  -- Audit
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by          UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT best_times_stroke_check  CHECK (stroke IN ('free','back','breast','fly','im')),
  CONSTRAINT best_times_course_check  CHECK (course IN ('SCY','SCM','LCM')),
  CONSTRAINT best_times_swimmer_event_unique UNIQUE (org_id, swimmer_id, event_distance, stroke, course)
);

CREATE INDEX best_times_org_id_idx      ON best_times (org_id);
CREATE INDEX best_times_swimmer_id_idx  ON best_times (swimmer_id);
CREATE INDEX best_times_org_swimmer_idx ON best_times (org_id, swimmer_id);

-- +goose Down
DROP TABLE IF EXISTS best_times;
