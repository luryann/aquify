-- +goose Up
-- Tenancy tracker item 4.4, ninth step: row-level security on the performance tables
-- (results, splits, best times, time standards, team records, goals, relay lineups).
-- Every performance repository statement binds the org, and the startup load now reads
-- one club at a time under that club's binding. Members' swimmer-delete guard counts
-- results and best_times and already runs under a bound org (ListSwimmerDeleteDependencies).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['results', 'splits', 'best_times', 'time_standards',
                             'team_record_entries', 'swimmer_goals', 'relay_lineups']
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
    FOREACH t IN ARRAY ARRAY['results', 'splits', 'best_times', 'time_standards',
                             'team_record_entries', 'swimmer_goals', 'relay_lineups']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
