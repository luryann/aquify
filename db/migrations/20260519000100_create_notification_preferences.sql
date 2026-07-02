-- +goose Up
CREATE TABLE notification_preferences (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    user_id       TEXT NOT NULL,
    household_id  TEXT,
    event_id      TEXT NOT NULL,
    email_enabled BOOLEAN NOT NULL DEFAULT false,
    push_enabled  BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by    TEXT,
    UNIQUE (org_id, user_id, event_id)
);

CREATE INDEX idx_notification_preferences_org_user
    ON notification_preferences (org_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS notification_preferences;
