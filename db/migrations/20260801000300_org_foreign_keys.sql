-- +goose Up
-- Tenancy tracker items 1.3, 1.4, 1.5.
--  1.4 message_edits / message_reactions / message_attachments gain org_id
--      (backfilled from the parent message) with a composite FK.
--  1.3 every tenant table's org_id references orgs(id) (ON DELETE RESTRICT; an
--      org is removed only by the explicit offboarding job).
--  1.5 every single-column FK child.col -> parent.id between tenant tables gets a
--      composite twin (org_id, col) -> parent(org_id, id), so the database itself
--      rejects a child pointing at another org's parent.
-- Constraints are added NOT VALID then validated to keep locks short.
-- runtime_snapshots is intentionally global: it is the legacy single-process
-- snapshot bridge, disabled whenever a database pool backs the services.

-- 1.4 ---------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS messages_org_id_id_uidx ON messages (org_id, id);

ALTER TABLE message_edits ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE message_reactions ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE message_attachments ADD COLUMN IF NOT EXISTS org_id UUID;

UPDATE message_edits c SET org_id = m.org_id FROM messages m WHERE m.id = c.message_id AND c.org_id IS NULL;
UPDATE message_reactions c SET org_id = m.org_id FROM messages m WHERE m.id = c.message_id AND c.org_id IS NULL;
UPDATE message_attachments c SET org_id = m.org_id FROM messages m WHERE m.id = c.message_id AND c.org_id IS NULL;

ALTER TABLE message_edits ALTER COLUMN org_id SET NOT NULL;
ALTER TABLE message_reactions ALTER COLUMN org_id SET NOT NULL;
ALTER TABLE message_attachments ALTER COLUMN org_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS message_edits_org_idx ON message_edits (org_id);
CREATE INDEX IF NOT EXISTS message_reactions_org_idx ON message_reactions (org_id);
CREATE INDEX IF NOT EXISTS message_attachments_org_idx ON message_attachments (org_id);

-- +goose StatementBegin
DO $$
DECLARE
    uuid_tbl record;
    fk       record;
    child_attr text;
    parent_attr text;
    action   text;
    cname    text;
    pidx     text;
BEGIN
    -- 1.3 register every org already in use, then FK to orgs --------------
    FOR uuid_tbl IN
        SELECT c.table_name::text AS t
        FROM information_schema.columns c
        JOIN information_schema.tables x
          ON x.table_schema = c.table_schema AND x.table_name = c.table_name AND x.table_type = 'BASE TABLE'
        WHERE c.table_schema = current_schema() AND c.column_name = 'org_id' AND c.data_type = 'uuid'
          AND c.table_name NOT IN ('orgs', 'org_domains')
    LOOP
        EXECUTE format('INSERT INTO orgs (id) SELECT DISTINCT org_id FROM %I WHERE org_id IS NOT NULL ON CONFLICT (id) DO NOTHING', uuid_tbl.t);
    END LOOP;

    FOR uuid_tbl IN
        SELECT c.table_name::text AS t
        FROM information_schema.columns c
        JOIN information_schema.tables x
          ON x.table_schema = c.table_schema AND x.table_name = c.table_name AND x.table_type = 'BASE TABLE'
        WHERE c.table_schema = current_schema() AND c.column_name = 'org_id' AND c.data_type = 'uuid'
          AND c.table_name NOT IN ('orgs', 'org_domains')
          AND NOT EXISTS (
              SELECT 1 FROM pg_constraint k
              WHERE k.contype = 'f' AND k.conrelid = c.table_name::regclass AND k.confrelid = 'orgs'::regclass)
    LOOP
        cname := left(uuid_tbl.t, 50) || '_org_fk';
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (org_id) REFERENCES orgs (id) ON DELETE RESTRICT NOT VALID', uuid_tbl.t, cname);
        EXECUTE format('ALTER TABLE %I VALIDATE CONSTRAINT %I', uuid_tbl.t, cname);
    END LOOP;

    -- 1.5 composite twins for single-column FKs to parent(id) --------------
    FOR fk IN
        SELECT con.oid AS oid,
               con.conrelid::regclass::text AS child,
               con.confrelid::regclass::text AS parent,
               con.conkey[1] AS ckey, con.confkey[1] AS pkey,
               con.confdeltype AS deltype,
               con.conrelid AS child_oid, con.confrelid AS parent_oid
        FROM pg_constraint con
        WHERE con.contype = 'f'
          AND array_length(con.conkey, 1) = 1
          AND con.connamespace = current_schema()::regnamespace
    LOOP
        SELECT attname INTO child_attr FROM pg_attribute WHERE attrelid = fk.child_oid AND attnum = fk.ckey;
        SELECT attname INTO parent_attr FROM pg_attribute WHERE attrelid = fk.parent_oid AND attnum = fk.pkey;
        CONTINUE WHEN parent_attr <> 'id' OR child_attr = 'org_id';
        CONTINUE WHEN fk.parent IN ('users', 'roles', 'orgs');
        -- both sides need a NOT NULL uuid org_id
        CONTINUE WHEN NOT EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = fk.child_oid AND a.attname = 'org_id' AND a.attnotnull AND NOT a.attisdropped);
        CONTINUE WHEN NOT EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = fk.parent_oid AND a.attname = 'org_id' AND a.attnotnull AND NOT a.attisdropped);
        -- already has a composite FK covering (org_id, col)
        CONTINUE WHEN EXISTS (
            SELECT 1 FROM pg_constraint k
            WHERE k.contype = 'f' AND k.conrelid = fk.child_oid AND k.confrelid = fk.parent_oid
              AND array_length(k.conkey, 1) = 2
              AND (SELECT attname FROM pg_attribute WHERE attrelid = fk.child_oid AND attnum = k.conkey[1]) = 'org_id'
              AND (SELECT attname FROM pg_attribute WHERE attrelid = fk.child_oid AND attnum = k.conkey[2]) = child_attr);

        pidx := left(replace(fk.parent, '"', ''), 48) || '_org_id_id_uidx';
        EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS %I ON %s (org_id, id)', pidx, fk.parent);

        action := CASE fk.deltype
            WHEN 'c' THEN 'ON DELETE CASCADE'
            WHEN 'n' THEN format('ON DELETE SET NULL (%I)', child_attr)
            WHEN 'r' THEN 'ON DELETE RESTRICT'
            WHEN 'd' THEN format('ON DELETE SET DEFAULT (%I)', child_attr)
            ELSE '' END;
        cname := left(replace(fk.child, '"', ''), 28) || '_' || left(child_attr, 24) || '_org_fk';
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I FOREIGN KEY (org_id, %I) REFERENCES %s (org_id, id) %s NOT VALID',
                       fk.child, cname, child_attr, fk.parent, action);
        EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', fk.child, cname);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Constraints added by the DO block are named *_org_fk; drop them by pattern.
-- +goose StatementBegin
DO $$
DECLARE r record;
BEGIN
    FOR r IN
        SELECT conrelid::regclass::text AS tbl, conname
        FROM pg_constraint
        WHERE contype = 'f' AND conname LIKE '%\_org\_fk' AND connamespace = current_schema()::regnamespace
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE message_attachments DROP COLUMN IF EXISTS org_id;
ALTER TABLE message_reactions DROP COLUMN IF EXISTS org_id;
ALTER TABLE message_edits DROP COLUMN IF EXISTS org_id;
