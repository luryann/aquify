-- +goose Up
-- Tenancy tracker item 4.4, eleventh step: row-level security on the USA Swimming
-- integration settings (encrypted credentials). The repository binds the org on every
-- statement and nothing else reads the table.
ALTER TABLE usa_swimming_integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE usa_swimming_integrations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usa_swimming_integrations
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON usa_swimming_integrations;
ALTER TABLE usa_swimming_integrations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE usa_swimming_integrations DISABLE ROW LEVEL SECURITY;
