-- +goose Up
-- Team records: every accepted/pending/rejected record entry per
-- course × gender × age group × event. The current record is the fastest
-- accepted entry; the record history is the progression of accepted entries.
CREATE TABLE team_record_entries (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       TEXT        NOT NULL,
    course       TEXT        NOT NULL CHECK (course IN ('SCY','SCM','LCM')),
    gender       TEXT        NOT NULL CHECK (gender IN ('M','F')),
    age_group    TEXT        NOT NULL CHECK (age_group IN ('8U','9-10','11-12','13-14','15-16','17-18','OPEN')),
    distance     INT         NOT NULL CHECK (distance > 0),
    stroke       TEXT        NOT NULL CHECK (stroke IN ('free','back','breast','fly','im')),
    time_ms      INT         NOT NULL CHECK (time_ms > 0),
    swimmer_id   UUID        REFERENCES swimmers(id) ON DELETE SET NULL,
    swimmer_name TEXT        NOT NULL,
    swimmer_age  INT,
    swim_date    DATE        NOT NULL,
    meet_id      UUID        REFERENCES meets(id) ON DELETE SET NULL,
    meet_name    TEXT        NOT NULL DEFAULT '',
    result_id    UUID        REFERENCES results(id) ON DELETE SET NULL,
    source       TEXT        NOT NULL CHECK (source IN ('result','manual')),
    status       TEXT        NOT NULL CHECK (status IN ('accepted','pending','rejected')),
    locked       BOOLEAN     NOT NULL DEFAULT FALSE,
    verified     BOOLEAN     NOT NULL DEFAULT FALSE,
    verified_by  TEXT        NOT NULL DEFAULT '',
    verified_at  TIMESTAMPTZ,
    notes        TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   TEXT        NOT NULL DEFAULT '',
    updated_by   TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX team_record_entries_key_idx
    ON team_record_entries (org_id, course, gender, age_group, distance, stroke, status);
CREATE UNIQUE INDEX team_record_entries_result_unique
    ON team_record_entries (org_id, result_id, age_group) WHERE result_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS team_record_entries;
