-- +goose Up
ALTER TABLE households ADD COLUMN keycloak_user_id TEXT;
CREATE UNIQUE INDEX households_keycloak_user_id_idx
    ON households (keycloak_user_id)
    WHERE keycloak_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS households_keycloak_user_id_idx;
ALTER TABLE households DROP COLUMN IF EXISTS keycloak_user_id;
