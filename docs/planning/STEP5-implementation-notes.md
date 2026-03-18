# STEP 5 Implementation Notes

Date: 2026-03-15  
Owner: Codex session

Purpose: execute `TODO.md` section `5) Auth, MFA, and RBAC`.

## Checklist

- [x] Implement OIDC login callback endpoint.
- [x] Implement session issue endpoint.
- [x] Implement session refresh endpoint.
- [x] Implement session revoke endpoint.
- [x] Add MFA challenge endpoint for privileged roles.
- [x] Add MFA enrollment endpoint.
- [x] Add API key/secret storage strategy doc (if needed).
- [x] Seed default roles: owner, admin, coach, parent, swimmer, billing, read-only.
- [x] Implement role assignment endpoint (owner/admin only).
- [x] Implement permission check utility by action.
- [x] Add deny-by-default auth policy tests.
- [x] Add endpoint test for unauthorized access.
- [x] Add endpoint test for scope violation.
- [x] Add endpoint test for privileged action requiring MFA.

## Implementation Strategy

1. Introduce an in-memory auth service (`server/internal/app/authn`) to support STEP 5 endpoint behavior without blocking on external IdP/database adapters.
2. Keep local developer stub tokens while adding issued session token support for access/refresh flows.
3. Gate privileged role assignment via a centralized permission utility that requires both role (`owner`/`admin`) and MFA state.
4. Enforce org scoping on role assignment by requiring `principal.org_id == X-Org-Id == request.org_id`.
5. Add explicit tests for deny-by-default and auth failure modes.

## Notes

- OIDC callback currently implements a deterministic local exchange stub based on supplied `code` and `state`.
- MFA challenge uses a local-dev challenge code (`123456`) and issues short-lived step-up session tokens.
- Role assignment endpoint currently writes to in-memory service state and is intended to be swapped with database persistence in a later step.
- API key/secret handling strategy is documented in `docs/security/api-key-secret-storage-strategy.md`.
