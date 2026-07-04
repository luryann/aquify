-- +goose Up
CREATE TABLE messages (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    household_id   UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    swimmer_id     UUID NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL,
    sender_role    TEXT NOT NULL,
    body           TEXT NOT NULL,
    read_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_org_id     ON messages (org_id);
CREATE INDEX idx_messages_thread     ON messages (org_id, household_id, swimmer_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS messages;
