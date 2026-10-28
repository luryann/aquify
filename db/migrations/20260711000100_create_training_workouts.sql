-- +goose Up
-- Structured workouts (library templates and practice-owned instances).
-- sections holds the ordered section/set tree; totals are recomputed by the
-- API on read, total_distance is denormalised for listing/sorting only.
CREATE TABLE training_workouts (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            TEXT        NOT NULL,
    title             TEXT        NOT NULL CHECK (length(btrim(title)) > 0),
    description       TEXT        NOT NULL DEFAULT '',
    course            TEXT        NOT NULL DEFAULT 'SCY' CHECK (course IN ('SCY','SCM','LCM')),
    is_template       BOOLEAN     NOT NULL DEFAULT FALSE,
    tags              TEXT[]      NOT NULL DEFAULT '{}',
    sections          JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(sections) = 'array'),
    total_distance    INT         NOT NULL DEFAULT 0 CHECK (total_distance >= 0),
    source_workout_id UUID        REFERENCES training_workouts(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by        TEXT        NOT NULL DEFAULT '',
    updated_by        TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX training_workouts_org_template_idx ON training_workouts(org_id, is_template, updated_at DESC);
CREATE INDEX training_workouts_tags_idx ON training_workouts USING GIN (tags);

ALTER TABLE practices
    ADD COLUMN workout_id UUID REFERENCES training_workouts(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX practices_workout_id_unique ON practices(workout_id) WHERE workout_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS practices_workout_id_unique;
ALTER TABLE practices DROP COLUMN IF EXISTS workout_id;
DROP TABLE IF EXISTS training_workouts;
