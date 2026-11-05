-- +goose Up
-- Tenancy tracker item 4.4, final step: row-level security on the billing and finance
-- tables. Every billing repository statement and transaction binds the org, and the
-- all-club reload reads one club at a time. The advisory run/late-fee locks use a session
-- lock connection that touches no table.
-- webhook_events is the provider delivery ledger: dedupe is global (one provider event is
-- processed once, whichever club it belongs to) and the club may not be known when the
-- event is claimed, so its claim/finish statements bind the event key instead:
-- webhook_event exposes exactly the row for the presented provider:event_id. Reading the
-- ledger back is per club (tenant_isolation).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['billing_fee_schedules', 'billing_discount_rules', 'billing_settings', 'billing_adjustments', 'billing_charges',
                             'billing_runs', 'billing_late_fee_applications', 'household_autopay_profiles',
                             'household_credit_balances', 'household_credit_entries', 'invoices', 'invoice_items',
                             'ledger_events', 'payments', 'refunds', 'webhook_events']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd
CREATE POLICY webhook_event ON webhook_events
    USING (provider || ':' || event_id = NULLIF(current_setting('app.webhook_event', true), ''))
    WITH CHECK (provider || ':' || event_id = NULLIF(current_setting('app.webhook_event', true), ''));

-- +goose Down
DROP POLICY IF EXISTS webhook_event ON webhook_events;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['billing_fee_schedules', 'billing_discount_rules', 'billing_settings', 'billing_adjustments', 'billing_charges',
                             'billing_runs', 'billing_late_fee_applications', 'household_autopay_profiles',
                             'household_credit_balances', 'household_credit_entries', 'invoices', 'invoice_items',
                             'ledger_events', 'payments', 'refunds', 'webhook_events']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
