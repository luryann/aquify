-- +goose Up
-- Tenancy tracker item 1.2: every org_id column becomes UUID.
--
-- Legacy TEXT columns are converted in place. The legacy local-dev literal
-- 'org-demo' maps to the canonical local-dev org UUID; any other non-UUID value
-- aborts the migration with a sample so nothing is silently nulled.
-- Foreign keys that touch the converted tables are dropped first and recreated
-- from their saved definitions afterwards. ALTER ... TYPE takes an ACCESS
-- EXCLUSIVE lock per table; run in a maintenance window on large installs.
-- +goose StatementBegin
DO $$
DECLARE
    uuid_re  constant text := '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';
    demo_id  constant text := '00000000-0000-0000-0000-00000000a001';
    r        record;
    bad      bigint;
    sample   text;
    fk       record;
BEGIN
    CREATE TEMP TABLE _org_text_cols ON COMMIT DROP AS
    SELECT c.table_name::text AS table_name
    FROM information_schema.columns c
    JOIN information_schema.tables t
      ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
    WHERE c.table_schema = current_schema()
      AND c.column_name = 'org_id'
      AND c.data_type IN ('text', 'character varying');

    -- 1. Refuse to convert values we cannot map.
    FOR r IN SELECT table_name FROM _org_text_cols LOOP
        EXECUTE format($q$SELECT count(*), min(org_id) FROM %I
                          WHERE org_id IS NOT NULL AND org_id <> 'org-demo' AND org_id !~* %L$q$, r.table_name, uuid_re)
           INTO bad, sample;
        IF bad > 0 THEN
            RAISE EXCEPTION 'org_id unify: table % has % row(s) with a non-UUID org_id (e.g. %); normalize them first', r.table_name, bad, sample;
        END IF;
    END LOOP;

    -- 2. Save and drop every FK that references or is declared on a converted table.
    CREATE TEMP TABLE _org_fk_saved ON COMMIT DROP AS
    SELECT con.conname::text AS conname,
           con.conrelid::regclass::text AS tbl,
           pg_get_constraintdef(con.oid) AS def
    FROM pg_constraint con
    WHERE con.contype = 'f'
      AND (con.conrelid::regclass::text IN (SELECT table_name FROM _org_text_cols)
        OR con.confrelid::regclass::text IN (SELECT table_name FROM _org_text_cols));
    FOR fk IN SELECT * FROM _org_fk_saved LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', fk.tbl, fk.conname);
    END LOOP;

    -- 3. Convert.
    FOR r IN SELECT table_name FROM _org_text_cols LOOP
        EXECUTE format('ALTER TABLE %I ALTER COLUMN org_id DROP DEFAULT', r.table_name);
        EXECUTE format($q$ALTER TABLE %I ALTER COLUMN org_id TYPE uuid USING (
                CASE WHEN org_id = 'org-demo' THEN %L::uuid ELSE org_id::uuid END)$q$, r.table_name, demo_id);
    END LOOP;

    -- 4. Recreate saved FKs (re-validating existing rows).
    FOR fk IN SELECT * FROM _org_fk_saved LOOP
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s', fk.tbl, fk.conname, fk.def);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Irreversible by design: the legacy 'org-demo' literal cannot be restored from a
-- UUID. Reverting would only re-introduce mixed-type tenant keys.
SELECT 1;
