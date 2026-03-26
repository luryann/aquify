# Step 30B: User Management API Endpoints — Implementation Plan

## Goal
Build all HTTP endpoints for managing Keycloak-backed users from the DARE admin
dashboard. Every action must be org-isolated, role-gated, and audit-logged.

## Endpoints

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | /api/v1/users | owner/admin | List org users; search/role/status filters |
| GET | /api/v1/users/{id} | owner/admin | Get user detail |
| POST | /api/v1/users/invite | owner/admin | Provision Keycloak user + send welcome email |
| PUT | /api/v1/users/{id} | owner/admin | Update display name / email |
| POST | /api/v1/users/{id}/disable | owner/admin | Disable Keycloak account |
| POST | /api/v1/users/{id}/enable | owner/admin | Re-enable Keycloak account |
| DELETE | /api/v1/users/{id} | owner only | Delete user; requires MFA |
| GET | /api/v1/users/{id}/roles | owner/admin | List assigned roles |
| POST | /api/v1/users/{id}/roles | owner/admin | Assign role |
| DELETE | /api/v1/users/{id}/roles/{role} | owner/admin | Remove role |
| POST | /api/v1/users/{id}/password-reset | owner/admin | Send password reset email |

## Business Rules
- Admin cannot assign or remove the `owner` role (tier guard)
- Cannot remove last `owner` role assignment in an org
- All user management calls validate that the target user's `org_id` attribute
  matches the request principal's OrgID (enforced per-handler, not middleware)
- Delete requires MFA (`principal.MFAAuthenticated == true`); return 403 otherwise

## Audit Action Constants (additions to audit/service.go)
- `auth.user.invite`
- `auth.user.update`
- `auth.user.disable`
- `auth.user.enable`
- `auth.user.delete`
- `auth.user.password_reset`
- `auth.role.assign`  ← already exists
- `auth.role.remove`

## Files
1. `server/internal/app/handlers/users.go` — new handler file
2. `server/internal/app/audit/service.go` — add new action constants
3. `server/internal/app/server.go` — wire keycloak client + user handlers
4. `server/internal/app/server_step30b_test.go` — integration tests

## Status
- [x] handlers/users.go — 11 endpoints, org-isolation, tier-guard, MFA guard
- [x] audit constants added — auth.user.invite/update/disable/enable/delete/password_reset + auth.role.remove
- [x] server.go wired — keycloak.Client injected, user handler routes registered
- [x] tests written — 27 tests (all passing): invite, role assign/remove, disable/enable, delete, RBAC matrix, org isolation, audit emission
