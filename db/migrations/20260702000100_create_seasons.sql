-- +goose Up
CREATE TABLE seasons (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    start_date DATE,
    end_date   DATE,
    status     TEXT        NOT NULL CHECK (status IN ('active','upcoming','completed')),
    goals      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT        NOT NULL DEFAULT '',
    updated_by TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX seasons_org_start_idx ON seasons(org_id, start_date DESC);

-- +goose Down
DROP TABLE IF EXISTS seasons;
