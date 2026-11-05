-- +goose Up
-- Tenancy tracker item 4.4 (pilot): row-level security on the volunteer domain, the
-- first domain whose repository binds the org to every statement
-- (internal/tenantdb: set_config('app.org_id', <org>, true)).
--
-- The policy lets a session see and write only rows of the org it bound; with no org
-- bound it sees nothing (fail closed). FORCE applies it to the table owner too.
-- Superusers and roles with BYPASSRLS (typical for migrations and break-glass
-- access) are unaffected, so the application's database role must not have either.
--
-- Do NOT add other tables here until their repositories bind the org (see the
-- tracker): RLS on a table whose queries do not bind an org would hide every row.
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['volunteer_settings', 'volunteer_jobs', 'volunteer_signups',
                             'volunteer_requirements', 'volunteer_credits', 'volunteer_buyouts']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['volunteer_settings', 'volunteer_jobs', 'volunteer_signups',
                             'volunteer_requirements', 'volunteer_credits', 'volunteer_buyouts']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
