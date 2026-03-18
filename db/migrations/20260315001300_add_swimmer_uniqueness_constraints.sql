-- +goose Up
CREATE UNIQUE INDEX swimmers_org_swimmer_external_id_unique_idx
  ON swimmers (org_id, lower(swimmer_external_id))
  WHERE swimmer_external_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX swimmer_groups_org_swimmer_group_unique_idx
  ON swimmer_groups (org_id, swimmer_id, group_code)
  WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS swimmer_groups_org_swimmer_group_unique_idx;
DROP INDEX IF EXISTS swimmers_org_swimmer_external_id_unique_idx;
