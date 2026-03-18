-- +goose Up

-- Add soft-delete columns to all STEP 8 tables (architectural convention: deleted_at / deleted_by).
ALTER TABLE meets
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE meet_sessions
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE meet_events
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE commitments
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE entries
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE entry_legs
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- Add missing updated_at / updated_by columns to commitment_event_selections
-- (architectural convention: every table has id, created_at, updated_at, created_by, updated_by).
ALTER TABLE commitment_event_selections
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- Add index on commitment_event_selections.meet_event_id to support coach-review queries.
CREATE INDEX IF NOT EXISTS commitment_event_selections_meet_event_id_idx
  ON commitment_event_selections (meet_event_id);

-- +goose Down
DROP INDEX IF EXISTS commitment_event_selections_meet_event_id_idx;

ALTER TABLE commitment_event_selections
  DROP COLUMN IF EXISTS updated_by,
  DROP COLUMN IF EXISTS updated_at;

ALTER TABLE entry_legs
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE entries
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE commitments
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE meet_events
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE meet_sessions
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE meets
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;
