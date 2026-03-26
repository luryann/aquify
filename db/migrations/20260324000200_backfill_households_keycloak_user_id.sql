-- +goose Up
-- This migration is a placeholder for the Keycloak user ID backfill.
-- The actual backfill must be performed by the application-level script
-- scripts/backfill-household-keycloak-ids.sh, which queries the Keycloak
-- Admin API to match household primary_email values against Keycloak user
-- emails and updates keycloak_user_id accordingly.
--
-- To run the backfill after applying this migration:
--   ./scripts/backfill-household-keycloak-ids.sh
--
-- No SQL changes are made here; the column was added in 20260324000100.
SELECT 1;

-- +goose Down
SELECT 1;
