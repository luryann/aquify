# ADR: Auth Provider for v1

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires locking Auth0 vs Keycloak before implementation proceeds.

## Decision
Use `Keycloak` as the v1 auth provider.

## Alternatives Considered
- `Auth0`: lower ops overhead, managed service.
- `Keycloak`: self-hosted OIDC with tighter deployment control.

## Consequences
- Team owns auth operations, upgrades, and backup/restore.
- OIDC integration and MFA enforcement are still the required interface and behavior.
- Environment setup must include Keycloak in non-local environments before auth work begins.

## References
- `DECISIONS.md` section "Authentication Provider"
- `docs/planning/Developer Handoff Master Document.txt` section 3
- `TODO.md` STEP 0 item 3
