-- +goose Up
CREATE TABLE email_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL,
    campaign_id         UUID REFERENCES email_campaigns (id) ON DELETE SET NULL,
    recipient_email     TEXT NOT NULL,
    recipient_user_id   UUID,
    status              TEXT NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'sent', 'delivered', 'bounced', 'failed', 'suppressed')),
    provider_message_id TEXT,
    error_message       TEXT,
    sent_at             TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    bounced_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_deliveries_campaign_id ON email_deliveries (campaign_id);
CREATE INDEX idx_email_deliveries_recipient   ON email_deliveries (org_id, recipient_email);
CREATE INDEX idx_email_deliveries_status      ON email_deliveries (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS email_deliveries;
