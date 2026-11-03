-- +goose Up
-- Soft-deleted swimmers/households must not hold unique identifiers.
DROP INDEX IF EXISTS swimmers_org_usa_swimming_id_unique_idx;
CREATE UNIQUE INDEX swimmers_org_usa_swimming_id_unique_idx
  ON swimmers (org_id, lower(usa_swimming_id))
  WHERE usa_swimming_id IS NOT NULL AND usa_swimming_id <> '' AND deleted_at IS NULL;

DROP INDEX IF EXISTS households_keycloak_user_id_idx;
CREATE UNIQUE INDEX households_keycloak_user_id_idx
  ON households (keycloak_user_id)
  WHERE keycloak_user_id IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS swimmers_keycloak_user_id_idx;
CREATE UNIQUE INDEX swimmers_keycloak_user_id_idx
  ON swimmers (keycloak_user_id)
  WHERE keycloak_user_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
-- Intentionally a no-op: restoring the stricter indexes could fail on soft-deleted duplicates.
SELECT 1;
