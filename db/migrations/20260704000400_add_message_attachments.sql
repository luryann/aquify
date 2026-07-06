-- +goose Up

-- Attachments sent with a message. object_key points at the presigned-upload
-- object in storage; download URLs are signed lazily at read time, not stored.
CREATE TABLE message_attachments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    filename   TEXT NOT NULL,
    mime_type  TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_message_attachments_message_id ON message_attachments (message_id);

-- +goose Down
DROP TABLE IF EXISTS message_attachments;
