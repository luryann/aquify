-- +goose Up
-- Saved relay lineups per meet. Teams (A/B/C…) and their legs are stored as
-- JSONB because they are always read and written as one proposal.
CREATE TABLE relay_lineups (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      TEXT        NOT NULL,
    meet_id     UUID        NOT NULL REFERENCES meets(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL DEFAULT '',
    course      TEXT        NOT NULL CHECK (course IN ('SCY','SCM','LCM')),
    gender      TEXT        NOT NULL CHECK (gender IN ('M','F','X')),
    age_group   TEXT        NOT NULL,
    relay_type  TEXT        NOT NULL CHECK (relay_type IN ('free','medley')),
    distance    INT         NOT NULL CHECK (distance IN (200,400,800)),
    teams       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    notes       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  TEXT        NOT NULL DEFAULT '',
    updated_by  TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX relay_lineups_org_meet_idx ON relay_lineups (org_id, meet_id);

-- +goose Down
DROP TABLE IF EXISTS relay_lineups;
