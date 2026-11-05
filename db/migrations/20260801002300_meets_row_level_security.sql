-- +goose Up
-- Tenancy tracker item 4.4, sixteenth step: row-level security on the meet tables (meets,
-- sessions, events, commitments, event selections, entries, entry legs). Every meets
-- repository statement binds the org and the all-club load reads one club at a time.
-- Members' swimmer-delete guard counts commitments/entries/entry_legs under a bound org,
-- and performance's result insert reads meet_events under the result's own org.
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['meets', 'meet_sessions', 'meet_events', 'commitments',
                             'commitment_event_selections', 'entries', 'entry_legs']
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
    FOREACH t IN ARRAY ARRAY['meets', 'meet_sessions', 'meet_events', 'commitments',
                             'commitment_event_selections', 'entries', 'entry_legs']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
