-- +goose Up
CREATE TABLE swimmer_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  swimmer_id UUID NOT NULL,
  group_code TEXT NOT NULL,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  deleted_by UUID REFERENCES users(id) ON DELETE SET NULL
  ,
  CONSTRAINT swimmer_groups_swimmer_org_fk
    FOREIGN KEY (org_id, swimmer_id) REFERENCES swimmers (org_id, id) ON DELETE CASCADE
);

CREATE INDEX swimmer_groups_org_id_idx ON swimmer_groups (org_id);
CREATE INDEX swimmer_groups_swimmer_id_idx ON swimmer_groups (swimmer_id);
CREATE INDEX swimmer_groups_org_group_code_idx ON swimmer_groups (org_id, group_code);

CREATE TRIGGER swimmer_groups_set_updated_at
BEFORE UPDATE ON swimmer_groups
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS swimmer_groups;
