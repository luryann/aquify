-- +goose Up
CREATE TABLE announcements (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft', 'published', 'archived')),
    audience    TEXT NOT NULL DEFAULT 'all'
                    CHECK (audience IN ('all', 'group')),
    group_id    UUID,
    published_at TIMESTAMPTZ,
    archived_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID,
    updated_by  UUID
);

CREATE INDEX idx_announcements_org_id     ON announcements (org_id);
CREATE INDEX idx_announcements_status     ON announcements (org_id, status);
CREATE INDEX idx_announcements_group_id   ON announcements (group_id) WHERE group_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS announcements;
