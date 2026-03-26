-- +goose Up
CREATE TABLE practices (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        TEXT        NOT NULL,
    date          DATE        NOT NULL,
    group_id      TEXT,
    type          TEXT        NOT NULL CHECK (type IN ('water','dryland','video','rest')),
    description   TEXT        NOT NULL DEFAULT '',
    yardage_target INT        NOT NULL DEFAULT 0,
    attendance    INT,
    strokes_free   INT        NOT NULL DEFAULT 0,
    strokes_back   INT        NOT NULL DEFAULT 0,
    strokes_breast INT        NOT NULL DEFAULT 0,
    strokes_fly    INT        NOT NULL DEFAULT 0,
    strokes_im     INT        NOT NULL DEFAULT 0,
    notes         TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by    TEXT        NOT NULL DEFAULT '',
    updated_by    TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX practices_org_date_idx ON practices(org_id, date DESC);

-- +goose Down
DROP TABLE IF EXISTS practices;
