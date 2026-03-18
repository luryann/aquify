-- +goose Up
DROP INDEX IF EXISTS user_roles_unique_assignment_idx;

-- Enforce uniqueness for org-scoped role assignments (scope_id IS NULL).
CREATE UNIQUE INDEX user_roles_unique_assignment_null_scope_idx
  ON user_roles (org_id, user_id, role_id, scope_type)
  WHERE scope_id IS NULL;

-- Enforce uniqueness for resource-scoped role assignments (scope_id IS NOT NULL).
CREATE UNIQUE INDEX user_roles_unique_assignment_scoped_idx
  ON user_roles (org_id, user_id, role_id, scope_type, scope_id)
  WHERE scope_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS user_roles_unique_assignment_scoped_idx;
DROP INDEX IF EXISTS user_roles_unique_assignment_null_scope_idx;

CREATE UNIQUE INDEX user_roles_unique_assignment_idx
  ON user_roles (org_id, user_id, role_id, scope_type, scope_id);
