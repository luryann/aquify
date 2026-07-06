-- +goose Up

-- Basic emoji reactions on messages. The allowed emoji set is enforced in the
-- application layer (a small fixed list), not a DB constraint, so it can change
-- without a migration.
CREATE TABLE message_reactions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    emoji      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (message_id, user_id, emoji)
);

CREATE INDEX idx_message_reactions_message_id ON message_reactions (message_id);

-- +goose Down
DROP TABLE IF EXISTS message_reactions;
