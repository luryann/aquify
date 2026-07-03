-- +goose Up
CREATE TABLE swimmer_link_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  requested_swimmer_name TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved', 'dismissed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  resolved_by UUID REFERENCES users(id) ON DELETE SET NULL,
  resolved_swimmer_id UUID REFERENCES swimmers(id) ON DELETE SET NULL
);

CREATE INDEX swimmer_link_requests_org_id_idx ON swimmer_link_requests (org_id);
CREATE INDEX swimmer_link_requests_org_status_idx ON swimmer_link_requests (org_id, status);

-- +goose Down
DROP TABLE IF EXISTS swimmer_link_requests;
