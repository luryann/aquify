-- +goose Up
-- Per-club change counters for cache-resident domains (billing, performance), so a cache can
-- tell cheaply whether ANOTHER instance has written since it last loaded, instead of
-- reloading the whole club on every call.
--  * One row per (org, domain, writer). writer is the process id the statement carried in
--    app.writer (tenantdb sets it); a reader sums every writer except itself, so its own
--    write-through changes never force a reload. Statements with no writer count as foreign.
--  * Bumped by DEFERRED row triggers, i.e. at commit, so the counter row is locked last and
--    briefly (no new lock-order deadlocks), and at most once per transaction (last_txid).
--  * The counters hold no tenant data, only change counts, so the policy is deliberately
--    permissive: the triggers must be able to bump a club's counter whatever binding the
--    writing statement had (session lookups, offboarding, migrations).
CREATE TABLE org_data_versions (
    org_id    uuid   NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    domain    text   NOT NULL,
    writer    text   NOT NULL DEFAULT '',
    version   bigint NOT NULL DEFAULT 0,
    last_txid bigint,
    PRIMARY KEY (org_id, domain, writer)
);
ALTER TABLE org_data_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY shared_counters ON org_data_versions USING (true) WITH CHECK (true);

-- +goose StatementBegin
CREATE FUNCTION bump_org_data_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN o := OLD.org_id; ELSE o := NEW.org_id; END IF;
    INSERT INTO org_data_versions AS v (org_id, domain, writer, version, last_txid)
    VALUES (o, TG_ARGV[0], COALESCE(current_setting('app.writer', true), ''), 1, txid_current())
    ON CONFLICT (org_id, domain, writer) DO UPDATE
        SET version = v.version + 1, last_txid = txid_current()
        WHERE v.last_txid IS DISTINCT FROM txid_current();
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['billing_fee_schedules', 'billing_discount_rules', 'billing_settings', 'billing_adjustments', 'billing_charges', 'billing_runs', 'billing_late_fee_applications', 'household_autopay_profiles', 'household_credit_balances', 'household_credit_entries', 'invoices', 'invoice_items', 'ledger_events', 'payments', 'refunds']
    LOOP
        EXECUTE format('CREATE CONSTRAINT TRIGGER org_data_version AFTER INSERT OR UPDATE OR DELETE ON %I
            DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION bump_org_data_version(%L)', t, 'billing');
    END LOOP;
    FOREACH t IN ARRAY ARRAY['results', 'best_times', 'time_standards', 'team_record_entries', 'swimmer_goals', 'relay_lineups']
    LOOP
        EXECUTE format('CREATE CONSTRAINT TRIGGER org_data_version AFTER INSERT OR UPDATE OR DELETE ON %I
            DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION bump_org_data_version(%L)', t, 'performance');
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE r record;
BEGIN
    FOR r IN SELECT c.relname FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
             WHERE t.tgname = 'org_data_version' AND c.relnamespace = current_schema()::regnamespace
    LOOP
        EXECUTE format('DROP TRIGGER org_data_version ON %I', r.relname);
    END LOOP;
END $$;
-- +goose StatementEnd
DROP FUNCTION IF EXISTS bump_org_data_version();
DROP TABLE IF EXISTS org_data_versions;
