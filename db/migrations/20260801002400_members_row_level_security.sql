-- +goose Up
-- Tenancy tracker item 4.4, seventeenth step (the last domain before billing): row-level
-- security on the members tables (households, contacts, swimmers, swimmer groups, swimmer
-- link requests). Every members repository statement binds the org (deletes now carry it),
-- and the all-club load reads one club at a time.
-- The one lookup made before a club is known (which club owns this Keycloak user, used by
-- the sign-in hooks) binds the Keycloak user id: keycloak_lookup is a SELECT-only policy on
-- households and swimmers that exposes just the live row linked to the presented user.
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['households', 'contacts', 'swimmers', 'swimmer_groups', 'swimmer_link_requests']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd
CREATE POLICY keycloak_lookup ON households FOR SELECT
    USING (deleted_at IS NULL AND keycloak_user_id = NULLIF(current_setting('app.keycloak_user', true), ''));
CREATE POLICY keycloak_lookup ON swimmers FOR SELECT
    USING (deleted_at IS NULL AND keycloak_user_id = NULLIF(current_setting('app.keycloak_user', true), ''));

-- +goose Down
DROP POLICY IF EXISTS keycloak_lookup ON swimmers;
DROP POLICY IF EXISTS keycloak_lookup ON households;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['households', 'contacts', 'swimmers', 'swimmer_groups', 'swimmer_link_requests']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
