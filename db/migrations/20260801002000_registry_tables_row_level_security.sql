-- +goose Up
-- Tenancy tracker item 4.4, thirteenth step: row-level security on the club-owned
-- registry tables. Every statement binds the org, except the three lookups made before a
-- club is known, which bind the credential they were given instead:
--   org_domains          domain_lookup   SELECT of the verified row for the presented host
--   org_payment_accounts account_lookup  SELECT, and account_status UPDATE, of the row for
--                                         the presented Stripe account id (webhook paths)
-- Invites need no lookup policy: redemption is already per club (org id + code hash).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['org_domains', 'org_invites', 'org_payment_accounts']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd
CREATE POLICY domain_lookup ON org_domains FOR SELECT
    USING (domain = NULLIF(current_setting('app.domain', true), '') AND verified_at IS NOT NULL);
CREATE POLICY account_lookup ON org_payment_accounts FOR SELECT
    USING (stripe_account_id = NULLIF(current_setting('app.stripe_account', true), ''));
CREATE POLICY account_status ON org_payment_accounts FOR UPDATE
    USING (stripe_account_id = NULLIF(current_setting('app.stripe_account', true), ''))
    WITH CHECK (stripe_account_id = NULLIF(current_setting('app.stripe_account', true), ''));

-- +goose Down
DROP POLICY IF EXISTS account_status ON org_payment_accounts;
DROP POLICY IF EXISTS account_lookup ON org_payment_accounts;
DROP POLICY IF EXISTS domain_lookup ON org_domains;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['org_domains', 'org_invites', 'org_payment_accounts']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
