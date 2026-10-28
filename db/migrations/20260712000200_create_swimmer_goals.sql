-- +goose Up
CREATE TABLE swimmer_goals (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           TEXT        NOT NULL,
    swimmer_id       UUID        NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,
    distance         INT         NOT NULL CHECK (distance > 0),
    stroke           TEXT        NOT NULL CHECK (stroke IN ('free','back','breast','fly','im')),
    course           TEXT        NOT NULL CHECK (course IN ('SCY','SCM','LCM')),
    target_time_ms   INT         NOT NULL CHECK (target_time_ms > 0),
    baseline_time_ms INT,
    deadline         DATE,
    notes            TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by       TEXT        NOT NULL DEFAULT '',
    updated_by       TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX swimmer_goals_org_swimmer_idx ON swimmer_goals (org_id, swimmer_id);

-- +goose Down
DROP TABLE IF EXISTS swimmer_goals;
