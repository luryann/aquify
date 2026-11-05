-- +goose Up
-- Tenancy tracker item 4.4, tenth step: row-level security on the communications tables
-- (announcements, campaigns, deliveries, suppression list, notification preferences and
-- the message thread tables). Every comms repository statement binds the org; the
-- all-club load reads one club at a time; a provider event that names no club is not
-- attributed to any delivery (it could previously be matched by provider message id alone).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['announcements', 'email_campaigns', 'email_deliveries', 'suppression_list', 'notification_preferences',
                             'messages', 'message_edits', 'message_reactions', 'message_attachments', 'message_reads']
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
    FOREACH t IN ARRAY ARRAY['announcements', 'email_campaigns', 'email_deliveries', 'suppression_list', 'notification_preferences',
                             'messages', 'message_edits', 'message_reactions', 'message_attachments', 'message_reads']
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
