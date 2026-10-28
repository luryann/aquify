-- +goose Up
-- Season periodization: per-group phases (base/build/peak/taper/recovery)
-- with a weekly yardage target. Seasons are owned by the seasons domain.
CREATE TABLE training_plan_phases (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                TEXT        NOT NULL,
    season_id             UUID        NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    group_id              TEXT        NOT NULL CHECK (length(btrim(group_id)) > 0),
    phase                 TEXT        NOT NULL CHECK (phase IN ('base','build','peak','taper','recovery')),
    name                  TEXT        NOT NULL DEFAULT '',
    start_date            DATE        NOT NULL,
    end_date              DATE        NOT NULL,
    weekly_yardage_target INT         NOT NULL DEFAULT 0 CHECK (weekly_yardage_target BETWEEN 0 AND 200000),
    focus                 TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by            TEXT        NOT NULL DEFAULT '',
    updated_by            TEXT        NOT NULL DEFAULT '',
    CONSTRAINT training_plan_phases_dates_check CHECK (end_date >= start_date)
);

CREATE INDEX training_plan_phases_season_group_idx ON training_plan_phases(org_id, season_id, group_id, start_date);

-- +goose Down
DROP TABLE IF EXISTS training_plan_phases;
