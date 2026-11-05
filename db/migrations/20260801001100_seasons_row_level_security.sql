-- +goose Up
-- Tenancy tracker item 4.4, fourth step: row-level security on seasons. The seasons
-- repository binds the org on every statement and nothing else reads the table
-- (training and volunteer reference it by foreign key only, and referential-integrity
-- checks bypass row security).
ALTER TABLE seasons ENABLE ROW LEVEL SECURITY;
ALTER TABLE seasons FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON seasons
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON seasons;
ALTER TABLE seasons NO FORCE ROW LEVEL SECURITY;
ALTER TABLE seasons DISABLE ROW LEVEL SECURITY;
