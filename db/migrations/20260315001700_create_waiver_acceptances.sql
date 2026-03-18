-- +goose Up
CREATE TABLE waiver_acceptances (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  registration_id UUID NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
  waiver_type TEXT NOT NULL,
  accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  ip_address TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT waiver_acceptances_registration_type_unique UNIQUE (registration_id, waiver_type)
);

CREATE INDEX waiver_acceptances_org_id_idx ON waiver_acceptances (org_id);
CREATE INDEX waiver_acceptances_registration_id_idx ON waiver_acceptances (registration_id);

-- +goose Down
DROP TABLE IF EXISTS waiver_acceptances;
