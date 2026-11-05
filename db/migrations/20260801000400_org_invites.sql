-- +goose Up
-- Tenancy tracker item 3.6: org-scoped invite codes. Only a SHA-256 hash of the
-- code is stored; the plaintext is shown once at creation. A code is bound to one
-- org and one non-admin role and can be limited by uses and expiry.
CREATE TABLE org_invites (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID        NOT NULL,
    code_hash   TEXT        NOT NULL,
    label       TEXT        NOT NULL DEFAULT '',
    role        TEXT        NOT NULL CHECK (role IN ('parent', 'swimmer', 'coach')),
    max_uses    INTEGER     NOT NULL DEFAULT 1 CHECK (max_uses >= 1),
    uses        INTEGER     NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_by  UUID        REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ,
    CONSTRAINT org_invites_uses_within_max CHECK (uses <= max_uses),
    CONSTRAINT org_invites_org_fk FOREIGN KEY (org_id) REFERENCES orgs (id) ON DELETE RESTRICT,
    CONSTRAINT org_invites_org_code_uidx UNIQUE (org_id, code_hash)
);
CREATE INDEX org_invites_org_idx ON org_invites (org_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS org_invites;
