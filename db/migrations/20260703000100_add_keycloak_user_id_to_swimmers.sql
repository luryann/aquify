-- +goose Up
ALTER TABLE swimmers ADD COLUMN keycloak_user_id TEXT;
CREATE UNIQUE INDEX swimmers_keycloak_user_id_idx
    ON swimmers (keycloak_user_id)
    WHERE keycloak_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS swimmers_keycloak_user_id_idx;
ALTER TABLE swimmers DROP COLUMN IF EXISTS keycloak_user_id;
