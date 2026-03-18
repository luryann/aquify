-- +goose Up
CREATE TABLE roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID,
  name TEXT NOT NULL CHECK (name ~ '^[a-z][a-z0-9_-]{1,62}$'),
  description TEXT,
  is_system BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX roles_name_global_key ON roles (name) WHERE org_id IS NULL;
CREATE UNIQUE INDEX roles_org_id_name_key ON roles (org_id, name) WHERE org_id IS NOT NULL;
CREATE INDEX roles_org_id_idx ON roles (org_id);

CREATE TRIGGER roles_set_updated_at
BEFORE UPDATE ON roles
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS roles;
