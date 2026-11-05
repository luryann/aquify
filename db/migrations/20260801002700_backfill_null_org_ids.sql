-- +goose Up
-- Rows written before multi-tenancy can have a NULL org_id in sessions and mfa_factors. Under
-- row-level security such rows are invisible to every club (an MFA factor would silently stop
-- being loaded, so the user would have to re-enrol). Give each the club of its user wherever
-- that is known. Rows whose user has no club either stay NULL: sessions are still reachable
-- through the token-hash lookup and simply expire, and an orphaned factor belongs to no club.
-- FORCE row-level security applies to the table owner too, so lift it for the backfill only
-- (same transaction, restored before commit).
ALTER TABLE mfa_factors NO FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions NO FORCE ROW LEVEL SECURITY;
-- mfa_factors.user_id is text (legacy shape), hence the casts; this is a one-off backfill.
UPDATE mfa_factors f SET org_id = u.org_id
  FROM users u WHERE f.org_id IS NULL AND f.user_id::text = u.id::text AND u.org_id IS NOT NULL;
UPDATE sessions s SET org_id = u.org_id
  FROM users u WHERE s.org_id IS NULL AND s.user_id = u.id AND u.org_id IS NOT NULL;
ALTER TABLE mfa_factors FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;

-- +goose Down
-- The backfill is not reversible (the previous NULLs are not recorded) and is harmless to keep.
SELECT 1;
