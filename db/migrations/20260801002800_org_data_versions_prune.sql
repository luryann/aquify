-- +goose Up
-- Each process restart adds one org_data_versions row per club and domain (the writer id is per
-- process). touched_at lets old writers be folded into one '~retired' row; the sum other
-- writers see can only grow, so no cache ever mistakes a pruned state for an unchanged one.
-- The API/worker calls prune_org_data_versions() once at startup (restarts are what add rows).
ALTER TABLE org_data_versions ADD COLUMN touched_at timestamptz NOT NULL DEFAULT now();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_org_data_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN o := OLD.org_id; ELSE o := NEW.org_id; END IF;
    INSERT INTO org_data_versions AS v (org_id, domain, writer, version, last_txid, touched_at)
    VALUES (o, TG_ARGV[0], COALESCE(current_setting('app.writer', true), ''), 1, txid_current(), now())
    ON CONFLICT (org_id, domain, writer) DO UPDATE
        SET version = v.version + 1, last_txid = txid_current(), touched_at = now()
        WHERE v.last_txid IS DISTINCT FROM txid_current();
    RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION prune_org_data_versions(older interval) RETURNS integer LANGUAGE plpgsql AS $$
DECLARE n integer;
BEGIN
    WITH gone AS (
        DELETE FROM org_data_versions
        WHERE writer <> '' AND writer <> '~retired' AND touched_at < now() - older
        RETURNING org_id, domain, version
    )
    INSERT INTO org_data_versions AS v (org_id, domain, writer, version, touched_at)
    SELECT org_id, domain, '~retired', SUM(version), now() FROM gone GROUP BY org_id, domain
    ON CONFLICT (org_id, domain, writer) DO UPDATE
        SET version = v.version + EXCLUDED.version, touched_at = now();
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS prune_org_data_versions(interval);
-- Restore the 20260801002600 trigger function before touched_at disappears, or every
-- write to a triggered table would fail at commit.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_org_data_version() RETURNS trigger LANGUAGE plpgsql AS $$
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
ALTER TABLE org_data_versions DROP COLUMN IF EXISTS touched_at;
