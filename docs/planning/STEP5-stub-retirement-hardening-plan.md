# STEP 5 Stub Retirement Hardening Plan

## Objective
Retire high-risk authentication and CI stubs so they cannot ship to non-development environments.

## Scope
- Disable `stub-*` bearer token acceptance outside local development.
- Replace OIDC callback role-derivation stub with real provider token exchange.
- Enforce OIDC callback `state` + `nonce` validation.
- Replace fixed MFA challenge code behavior with factor-based verification.
- Replace placeholder `make test` / `make lint` targets with executable quality gates.
- Update tracking docs (`TODO.md`, `CLAUDE.md`, and step notes) for explicit accountability.

## Implementation Checklist
- [x] Add environment-aware auth configuration flags and OIDC provider settings.
- [x] Remove middleware fallback that accepts hardcoded `stub-*` bearer tokens.
- [x] Gate any remaining static auth principals to development-only configuration.
- [x] Implement OIDC authorization code exchange against provider token endpoint.
- [x] Validate signed `state` payload and compare with ID token `nonce`.
- [x] Validate ID token issuer/audience/expiry claims needed for callback acceptance.
- [x] Extract principal identity/roles from verified callback claims and issue session.
- [x] Replace static MFA code check with factor secret verification flow.
- [x] Update auth handler messaging for non-static MFA challenge behavior.
- [x] Replace `make test` and `make lint` placeholders with real commands.
- [x] Update/extend tests for new auth behavior and environment stub gating.
- [x] Update `.env.example` and `.env.local.example` with required auth settings.
- [x] Update `README.md` developer command descriptions.
- [x] Update `TODO.md` with explicit stub-retirement task items and status.
- [x] Update `CLAUDE.md` with new implementation and operating notes.

## Risks and Mitigations
- Risk: Breaking local developer convenience.
  - Mitigation: Keep local stub tokens gated by explicit dev env config.
- Risk: Callback verification drift vs chosen IdP claims.
  - Mitigation: Keep issuer/audience/state/nonce checks explicit and configurable.
- Risk: Existing tests depending on stub tokens fail.
  - Mitigation: Convert tests to issue real session tokens via auth endpoints.

## Verification Plan
- Run Go tests for API packages.
- Run admin lint pipeline from `make lint`.
- Run combined test pipeline from `make test`.
- Confirm no code path accepts `stub-*` tokens in non-development mode.
