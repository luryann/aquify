-- +goose Up
INSERT INTO roles (org_id, name, description, is_system)
VALUES
  (NULL, 'owner', 'Organization owner with full system authority', true),
  (NULL, 'admin', 'Administrative operator with elevated management permissions', true),
  (NULL, 'coach', 'Coaching role with meet and swimmer operations access', true),
  (NULL, 'parent', 'Household owner role for member self-service', true),
  (NULL, 'swimmer', 'Swimmer read-only role for personal performance visibility', true),
  (NULL, 'billing', 'Billing-focused role for invoicing and payment workflows', true),
  (NULL, 'read-only', 'Staff role with read-only platform access', true)
ON CONFLICT (name) WHERE org_id IS NULL DO NOTHING;

-- +goose Down
DELETE FROM roles
WHERE org_id IS NULL
  AND is_system = true
  AND name IN ('owner', 'admin', 'coach', 'parent', 'swimmer', 'billing', 'read-only');
