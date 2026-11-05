-- +goose Up
-- Tenancy tracker item 4.4, sixth step: row-level security on the audit trail. The
-- audit repository binds the org on every insert and query (queries are always for one
-- club); the table stays append-only. Offboarding keeps these rows by design.
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_events
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON audit_events;
ALTER TABLE audit_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events DISABLE ROW LEVEL SECURITY;
