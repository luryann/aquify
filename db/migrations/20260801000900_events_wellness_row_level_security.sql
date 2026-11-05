-- +goose Up
-- Tenancy tracker item 4.4, second step: row-level security on club events, RSVPs,
-- calendar feed tokens and wellness check-ins. Their repositories bind the org on
-- every statement (internal/tenantdb), exactly like the volunteer domain.
--
-- The calendar feed is fetched without a session: the secret token is the only
-- credential, so it cannot name an org. A second, SELECT-only policy lets a session
-- that presents the token's HASH (set_config('app.feed_token_hash', ...)) read that
-- one row and nothing else; the org is then taken from the row.
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['club_events', 'club_event_rsvps', 'calendar_feed_tokens', 'wellness_checkins']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
            USING (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)
            WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::uuid)$p$, t);
    END LOOP;
END $$;
-- +goose StatementEnd

CREATE POLICY feed_token_lookup ON calendar_feed_tokens FOR SELECT
    USING (token_hash = NULLIF(current_setting('app.feed_token_hash', true), ''));

-- +goose Down
DROP POLICY IF EXISTS feed_token_lookup ON calendar_feed_tokens;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['club_events', 'club_event_rsvps', 'calendar_feed_tokens', 'wellness_checkins']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
