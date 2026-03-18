-- +goose Up
CREATE TABLE email_campaigns (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL,
    name             TEXT NOT NULL,
    subject          TEXT NOT NULL,
    body_html        TEXT NOT NULL,
    body_text        TEXT,
    from_name        TEXT NOT NULL,
    from_email       TEXT NOT NULL,
    audience         TEXT NOT NULL DEFAULT 'all'
                         CHECK (audience IN ('all', 'group')),
    group_id         UUID,
    status           TEXT NOT NULL DEFAULT 'draft'
                         CHECK (status IN ('draft', 'sending', 'sent', 'failed', 'cancelled')),
    sent_at          TIMESTAMPTZ,
    total_recipients INT NOT NULL DEFAULT 0,
    sent_count       INT NOT NULL DEFAULT 0,
    failed_count     INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       UUID,
    updated_by       UUID
);

CREATE INDEX idx_email_campaigns_org_id ON email_campaigns (org_id);
CREATE INDEX idx_email_campaigns_status ON email_campaigns (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS email_campaigns;
