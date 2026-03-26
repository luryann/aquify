-- +goose Up
-- audit_events is an immutable append-only table that records every
-- privileged or sensitive action taken within the platform.
-- No updates or deletes are permitted; retention is enforced at the
-- application layer (archival/purge jobs governed by AUDIT_RETENTION_DAYS).
CREATE TABLE IF NOT EXISTS audit_events (
    id            TEXT        NOT NULL PRIMARY KEY,
    org_id        TEXT        NOT NULL,
    actor_user_id TEXT        NOT NULL,
    actor_roles   TEXT[]      NOT NULL DEFAULT '{}',
    action        TEXT        NOT NULL,
    resource_type TEXT        NOT NULL,
    resource_id   TEXT        NOT NULL,
    before_state  JSONB,
    after_state   JSONB,
    metadata      JSONB,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite index for org-scoped time-range queries (most common access pattern).
CREATE INDEX IF NOT EXISTS audit_events_org_time_idx
    ON audit_events (org_id, occurred_at DESC);

-- Index for filtering by actor.
CREATE INDEX IF NOT EXISTS audit_events_actor_idx
    ON audit_events (actor_user_id, occurred_at DESC);

-- Index for filtering by resource.
CREATE INDEX IF NOT EXISTS audit_events_resource_idx
    ON audit_events (resource_type, resource_id, occurred_at DESC);

-- Index for filtering by action.
CREATE INDEX IF NOT EXISTS audit_events_action_idx
    ON audit_events (action, occurred_at DESC);

-- Enforce immutability at the database level: any attempt to UPDATE or DELETE
-- an audit record raises an exception. This protects the compliance audit trail
-- from tampering by any DB user with table-level write access.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_events records are immutable and cannot be updated or deleted';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_events_no_update
  BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

CREATE TRIGGER audit_events_no_delete
  BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_no_delete ON audit_events;
DROP TRIGGER IF EXISTS audit_events_no_update ON audit_events;
DROP FUNCTION IF EXISTS audit_events_immutable();
DROP TABLE IF EXISTS audit_events;
