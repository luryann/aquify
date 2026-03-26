# Step 30A: Keycloak Admin API Client — Implementation Plan

## Goal
Build `server/internal/keycloak` package with full Admin REST API client, stub, and tests.

## Files to Create/Modify
1. `server/internal/keycloak/client.go` — interface, types, AdminClient (real), StubClient
2. `server/internal/keycloak/client_test.go` — integration tests with httptest mock server
3. `server/internal/app/config.go` — add `KeycloakAdminConfig` struct and `LoadConfig` wiring
4. `.env.local.example` — document new env vars

## API Endpoints (Keycloak Admin REST)
- Token: `POST /realms/{realm}/protocol/openid-connect/token` (client_credentials)
- List users: `GET /admin/realms/{realm}/users?q=org_id:{orgID}&search={search}&max={limit}&first={offset}`
- Get user: `GET /admin/realms/{realm}/users/{id}`
- Create user: `POST /admin/realms/{realm}/users`
- Update user: `PUT /admin/realms/{realm}/users/{id}`
- Delete user: `DELETE /admin/realms/{realm}/users/{id}`
- Send actions email: `PUT /admin/realms/{realm}/users/{id}/execute-actions-email`
- List realm roles for user: `GET /admin/realms/{realm}/users/{id}/role-mappings/realm`
- Assign realm roles: `POST /admin/realms/{realm}/users/{id}/role-mappings/realm`
- Remove realm roles: `DELETE /admin/realms/{realm}/users/{id}/role-mappings/realm`
- List realm roles: `GET /admin/realms/{realm}/roles`
- Get role by name: `GET /admin/realms/{realm}/roles/{name}`
- Create realm role: `POST /admin/realms/{realm}/roles`

## Config Env Vars
- `KEYCLOAK_ADMIN_URL` — base URL of Keycloak (e.g. `http://localhost:8180`)
- `KEYCLOAK_ADMIN_CLIENT_ID` — service account client ID
- `KEYCLOAK_ADMIN_CLIENT_SECRET` — service account client secret
- `KEYCLOAK_REALM` — target realm name

## Token Strategy
- Fetch via client_credentials grant
- Cache token with expiry
- Refresh when within 30s of expiry (mutex-protected)

## Status
- [x] client.go written
- [x] config.go updated
- [x] client_test.go written (27 tests, all passing)
- [x] env example updated
- [x] Tests passing
