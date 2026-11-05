-- +goose Up
-- Tenancy tracker item 4.4, seventh step: row-level security on file_exports. The
-- exports repository binds the org on every statement and loads one club at a time; the
-- purge job now fans out per club and passes the org with the generate-export task.
ALTER TABLE file_exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_exports FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON file_exports
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON file_exports;
ALTER TABLE file_exports NO FORCE ROW LEVEL SECURITY;
ALTER TABLE file_exports DISABLE ROW LEVEL SECURITY;
