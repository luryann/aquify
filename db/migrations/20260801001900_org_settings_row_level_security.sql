-- +goose Up
-- Tenancy tracker item 4.4, twelfth step: row-level security on org_settings. The
-- repository binds the org on every statement. The one lookup made before any club is
-- known (the public site resolves a club by slug, and slug uniqueness is checked across
-- clubs) binds the slug instead: slug_lookup is a SELECT-only policy that exposes exactly
-- the row whose public slug is presented. Slugs are public by design.
ALTER TABLE org_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org_settings
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);
CREATE POLICY slug_lookup ON org_settings FOR SELECT
    USING (public_slug = NULLIF(current_setting('app.public_slug', true), ''));

-- +goose Down
DROP POLICY IF EXISTS slug_lookup ON org_settings;
DROP POLICY IF EXISTS tenant_isolation ON org_settings;
ALTER TABLE org_settings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE org_settings DISABLE ROW LEVEL SECURITY;
