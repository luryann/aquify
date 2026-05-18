-- +goose Up

-- entries: code expects entry_time_ms and notes columns
ALTER TABLE entries ADD COLUMN IF NOT EXISTS entry_time_ms INT;
ALTER TABLE entries ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE entries DROP COLUMN IF EXISTS notes;
ALTER TABLE entries DROP COLUMN IF EXISTS entry_time_ms;
