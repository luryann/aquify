-- +goose Up
ALTER TABLE meets
  ADD COLUMN IF NOT EXISTS self_signup_enabled BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS max_entries_per_swimmer INT NOT NULL DEFAULT 3;

-- +goose Down
ALTER TABLE meets
  DROP COLUMN IF EXISTS max_entries_per_swimmer,
  DROP COLUMN IF EXISTS self_signup_enabled;
