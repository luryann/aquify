-- +goose Up
-- Tenant registry (multi-tenancy tracker item 1.1). Additive: nothing references
-- orgs yet; foreign keys from tenant tables arrive in a later migration once the
-- legacy TEXT org_id columns are unified to UUID (item 1.2).
CREATE TABLE IF NOT EXISTS orgs (
    id            UUID        PRIMARY KEY,
    slug          TEXT,
    name          TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'active'
                  CHECK (status IN ('provisioning', 'active', 'suspended', 'offboarding', 'deleted')),
    plan          TEXT        NOT NULL DEFAULT '',
    region        TEXT        NOT NULL DEFAULT '',
    suspended_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orgs_slug_format CHECK (slug IS NULL OR slug ~ '^[a-z0-9-]{3,40}$')
);

CREATE UNIQUE INDEX IF NOT EXISTS orgs_slug_uidx ON orgs (slug) WHERE slug IS NOT NULL;

DROP TRIGGER IF EXISTS orgs_set_updated_at ON orgs;
CREATE TRIGGER orgs_set_updated_at
BEFORE UPDATE ON orgs
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- Custom domains resolve to exactly one org (verified before they are honoured).
CREATE TABLE IF NOT EXISTS org_domains (
    domain       TEXT        PRIMARY KEY CHECK (domain = lower(domain) AND domain <> ''),
    org_id       UUID        NOT NULL REFERENCES orgs (id) ON DELETE CASCADE,
    verified_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS org_domains_org_idx ON org_domains (org_id);

-- Backfill: every UUID-shaped org_id already present in any tenant table gets a
-- registry row. Legacy non-UUID TEXT values (e.g. 'org-demo') cannot be
-- registered until item 1.2 normalizes them; they are counted in a NOTICE.
-- +goose StatementBegin
DO $$
DECLARE
    r record;
    legacy bigint;
    total_legacy bigint := 0;
BEGIN
    FOR r IN
        SELECT c.table_name, c.data_type
        FROM information_schema.columns c
        JOIN information_schema.tables t
          ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
        WHERE c.table_schema = current_schema()
          AND c.column_name = 'org_id'
          AND c.table_name NOT IN ('orgs', 'org_domains')
    LOOP
        IF r.data_type = 'uuid' THEN
            EXECUTE format('INSERT INTO orgs (id) SELECT DISTINCT org_id FROM %I WHERE org_id IS NOT NULL ON CONFLICT (id) DO NOTHING', r.table_name);
        ELSE
            EXECUTE format($q$INSERT INTO orgs (id)
                SELECT DISTINCT org_id::uuid FROM %I
                WHERE org_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                ON CONFLICT (id) DO NOTHING$q$, r.table_name);
            EXECUTE format($q$SELECT count(DISTINCT org_id) FROM %I
                WHERE org_id IS NOT NULL
                  AND org_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'$q$, r.table_name) INTO legacy;
            total_legacy := total_legacy + COALESCE(legacy, 0);
        END IF;
    END LOOP;

    -- Identity fields come from org_settings where the club already set them.
    UPDATE orgs o
    SET name = CASE WHEN o.name = '' THEN s.team_name ELSE o.name END
    FROM org_settings s
    WHERE s.org_id = o.id;

    UPDATE orgs o
    SET slug = s.public_slug
    FROM org_settings s
    WHERE s.org_id = o.id
      AND o.slug IS NULL
      AND s.public_slug ~ '^[a-z0-9-]{3,40}$'
      AND NOT EXISTS (SELECT 1 FROM orgs x WHERE x.slug = s.public_slug AND x.id <> o.id);

    IF total_legacy > 0 THEN
        RAISE NOTICE 'orgs backfill: % legacy non-UUID org_id value(s) left unregistered (see tracker item 1.2)', total_legacy;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS org_domains;
DROP TABLE IF EXISTS orgs;
