-- +goose Up

-- Per-user read watermark for a household+swimmer thread. Unread counts are
-- derived as "messages from other users newer than last_read_at" — cheaper
-- than a row-per-message read receipt table and sufficient for an inbox badge.
CREATE TABLE message_reads (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    household_id  UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    swimmer_id    UUID NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL,
    last_read_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, household_id, swimmer_id, user_id)
);

CREATE INDEX idx_message_reads_thread ON message_reads (org_id, household_id, swimmer_id);

-- +goose Down
DROP TABLE IF EXISTS message_reads;
