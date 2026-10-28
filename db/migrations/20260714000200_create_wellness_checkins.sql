-- +goose Up
CREATE TABLE wellness_checkins (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         TEXT        NOT NULL,
    swimmer_id     UUID        NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,
    household_id   TEXT        NOT NULL DEFAULT '',
    checkin_date   DATE        NOT NULL,
    sleep_hours    NUMERIC(3,1) NOT NULL CHECK (sleep_hours >= 0 AND sleep_hours <= 24),
    sleep_quality  SMALLINT    NOT NULL CHECK (sleep_quality BETWEEN 1 AND 5),
    soreness       SMALLINT    NOT NULL CHECK (soreness BETWEEN 1 AND 5),
    energy         SMALLINT    NOT NULL CHECK (energy BETWEEN 1 AND 5),
    stress         SMALLINT    NOT NULL CHECK (stress BETWEEN 1 AND 5),
    mood           SMALLINT    NOT NULL CHECK (mood BETWEEN 1 AND 5),
    note           TEXT        NOT NULL DEFAULT '',
    submitted_by   TEXT        NOT NULL DEFAULT '',
    submitted_role TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wellness_checkins_swimmer_day_unique UNIQUE (swimmer_id, checkin_date)
);

CREATE INDEX wellness_checkins_org_date_idx ON wellness_checkins(org_id, checkin_date);

-- +goose Down
DROP TABLE IF EXISTS wellness_checkins;
