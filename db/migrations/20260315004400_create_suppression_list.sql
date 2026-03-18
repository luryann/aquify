-- +goose Up
CREATE TABLE suppression_list (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    email      TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT 'manual'
                   CHECK (reason IN ('bounce', 'complaint', 'manual')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_suppression_list_org_email ON suppression_list (org_id, email);

-- +goose Down
DROP TABLE IF EXISTS suppression_list;
