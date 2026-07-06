-- +goose Up
-- RBAC role model collapse: 7 roles -> 4 (admin, coach, parent, swimmer).
-- Policy: owner -> admin, billing -> admin. Accounts whose only role is
-- read-only/read_only_staff are removed from user_roles (see note below on
-- why we do not also delete the underlying users row).
--
-- Schema recap (see db/migrations/20260315000300_create_roles.sql and
-- db/migrations/20260315000400_create_user_roles.sql):
--   roles(id, org_id, name, description, is_system, ...)
--     - system roles are seeded with org_id IS NULL (see
--       db/migrations/20260315000800_seed_default_roles.sql)
--   user_roles(id, org_id, user_id, role_id, scope_type, scope_id, ...)
--     - role assignments reference roles.id, not the role name directly
--     - unique on (org_id, user_id, role_id, scope_type[, scope_id])

-- Step 1: for every user_roles row currently pointing at an 'owner' or
-- 'billing' role, find the matching 'admin' role in the same org scope
-- (global system role rows have org_id IS NULL; org-scoped custom roles,
-- if any exist, are matched by org_id).
CREATE TEMP TABLE _role_reassignment AS
SELECT r_old.id AS old_role_id, r_new.id AS new_role_id
FROM roles r_old
JOIN roles r_new
  ON r_new.name = 'admin'
  AND r_new.org_id IS NOT DISTINCT FROM r_old.org_id
WHERE r_old.name IN ('owner', 'billing');

-- Step 2: drop any user_roles row that would collide with an assignment the
-- user already holds for 'admin' in the same scope (the unique index would
-- otherwise reject the reassignment in step 3).
DELETE FROM user_roles ur
USING _role_reassignment rr
WHERE ur.role_id = rr.old_role_id
  AND EXISTS (
    SELECT 1
    FROM user_roles ur2
    WHERE ur2.role_id = rr.new_role_id
      AND ur2.org_id = ur.org_id
      AND ur2.user_id = ur.user_id
      AND ur2.scope_type = ur.scope_type
      AND ur2.scope_id IS NOT DISTINCT FROM ur.scope_id
  );

-- Step 3: reassign the remaining owner/billing assignments to admin.
UPDATE user_roles ur
SET role_id = rr.new_role_id
FROM _role_reassignment rr
WHERE ur.role_id = rr.old_role_id;

DROP TABLE _role_reassignment;

-- Step 4: drop read-only / read_only_staff role assignments entirely.
-- We intentionally do NOT also delete the underlying users rows here: the
-- users table is referenced by 30+ FKs across the schema (households,
-- swimmers, audit events, sessions, created_by/updated_by columns, etc.),
-- there is no existing precedent in db/migrations/ for deleting user rows,
-- and silently cascading/nulling that many references from a role-cleanup
-- migration is too destructive to guess at here. If a user is left with zero
-- role assignments after this step, that is expected; a follow-up
-- app-level/ops decision should handle disabling or removing those accounts.
DELETE FROM user_roles
WHERE role_id IN (
  SELECT id FROM roles WHERE name IN ('read-only', 'read_only_staff')
);

-- Step 5: the legacy role rows are now unreferenced; remove them so the
-- roles table only advertises the 4 canonical roles going forward.
DELETE FROM roles
WHERE name IN ('owner', 'billing', 'read-only', 'read_only_staff');

-- Step 6: restrict system role names to the canonical 4-role set so a
-- future insert can't reintroduce a retired role name for a global role.
ALTER TABLE roles
  ADD CONSTRAINT roles_system_role_name_allowed
  CHECK (
    NOT (is_system AND org_id IS NULL)
    OR name IN ('admin', 'coach', 'parent', 'swimmer')
  );

-- +goose Down
-- Best-effort reversal only. Re-widening the constraint and re-seeding the
-- retired role rows is mechanical; reversing WHO holds which role is not
-- meaningfully reversible (we no longer know which admin assignments used to
-- be owner vs. billing vs. admin-natively, and read-only assignments were
-- deleted outright), so this restores the schema shape, not the prior data.
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_system_role_name_allowed;

INSERT INTO roles (org_id, name, description, is_system)
VALUES
  (NULL, 'owner', 'Organization owner with full system authority', true),
  (NULL, 'billing', 'Billing-focused role for invoicing and payment workflows', true),
  (NULL, 'read-only', 'Staff role with read-only platform access', true)
ON CONFLICT (name) WHERE org_id IS NULL DO NOTHING;
