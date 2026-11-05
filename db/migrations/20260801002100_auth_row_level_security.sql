-- +goose Up
-- Tenancy tracker item 4.4, fourteenth step: row-level security on the sign-in tables
-- (users, roles, user_roles, sessions, MFA factors).
--
-- * Every statement binds the org, except session lookups: a bearer token arrives before
--   the club is known, so those bind the token's hash (policy session_lookup exposes only
--   the row whose access or refresh hash is presented; refresh rotation binds both).
-- * users: a row with no org yet is visible and claimable by the first club that signs
--   the user in (the upsert sets org_id); WITH CHECK still requires the bound org, so a
--   club can never write a user into another club. No code lists users from this table.
-- * roles: system roles (org_id IS NULL) are shared reference data, readable by everyone.
-- Legacy rows with a NULL org_id in sessions/mfa_factors are reachable only through the
-- session hash lookup (sessions) and are not loaded for any club (factors).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['roles', 'user_roles', 'sessions', 'mfa_factors']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users
    USING (org_id IS NULL OR org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);
CREATE POLICY roles_global ON roles FOR SELECT USING (org_id IS NULL);
CREATE POLICY session_lookup ON sessions
    USING (access_token_hash = NULLIF(current_setting('app.session_hash', true), '')
        OR refresh_token_hash = NULLIF(current_setting('app.session_hash', true), ''))
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid);

-- +goose Down
DROP POLICY IF EXISTS session_lookup ON sessions;
DROP POLICY IF EXISTS roles_global ON roles;
DROP POLICY IF EXISTS tenant_isolation ON users;
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['roles', 'user_roles', 'sessions', 'mfa_factors']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
