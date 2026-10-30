-- +goose Up
-- Persist the coach's scratch reason on the entry itself (previously only kept
-- in the audit event). commitment_id, is_relay, proposed_by and approved_by
-- already exist on entries but were never written/loaded by the meets service.
ALTER TABLE entries ADD COLUMN IF NOT EXISTS scratch_reason TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE entries DROP COLUMN IF EXISTS scratch_reason;
