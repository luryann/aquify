-- +goose Up
CREATE TABLE IF NOT EXISTS runtime_snapshots (
    domain text PRIMARY KEY,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runtime_snapshots_updated_at
    ON runtime_snapshots (updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS runtime_snapshots;
