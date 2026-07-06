-- +goose Up

-- Track whether a message has been edited, and keep a history of prior bodies
-- for support/audit purposes. Edits are sender-only and unrestricted (no time
-- window) per product decision.
ALTER TABLE messages ADD COLUMN edited_at TIMESTAMPTZ;

CREATE TABLE message_edits (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id    UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    previous_body TEXT NOT NULL,
    edited_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_by     UUID NOT NULL
);

CREATE INDEX idx_message_edits_message_id ON message_edits (message_id);

-- +goose Down
DROP TABLE IF EXISTS message_edits;
ALTER TABLE messages DROP COLUMN IF EXISTS edited_at;
