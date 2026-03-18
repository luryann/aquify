# STEP 22 Security Hardening Plan

Date: 2026-03-18
Owner: Codex
Status: In progress

## Goal

Finish the remaining items in `TODO.md` section `22) Security hardening` with code, tests, CI updates, and documentation.

## Inputs reviewed

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`
- `docs/planning/STEP5-stub-retirement-hardening-plan.md`

## Findings before implementation

- Step 22 is the active unfinished task block in `TODO.md`.
- Step 5 completed the first four hardening items already:
  - dev stub token acceptance restricted to development
  - real OIDC authorization-code exchange
  - signed `state` plus nonce validation
  - factor-based MFA verification
- Session endpoints currently return tokens in JSON only; no secure cookie controls are enforced yet.
- No CSRF middleware exists for session-establishing endpoints.
- No general rate-limiting middleware is wired into the router.
- Import upload flow validates extension/type hints, but not filename safety, size caps, MIME allowlists, malware-scan hook state, or finalize-time upload metadata.
- No baseline security headers middleware is registered.
- CI currently runs OpenAPI lint, Go tests/vet, and admin lint/build, but not secret or dependency vulnerability scans.
- Privileged MFA enforcement exists for role assignment through the permission helper, but not for all sensitive billing mutations.
- There is no admin impersonation feature in the current router or service layer; safeguard work should make “disabled by default” explicit and block accidental enablement.
- There is no threat model document yet.
- `ABOUT.txt` was requested as a fallback source, but no `ABOUT.txt` file exists in the workspace.

## Implementation checklist

- [ ] Add security configuration for secure cookies, CSRF, rate limits, upload caps, malware-scan hook mode, and impersonation toggle.
- [ ] Add HTTPS-only cookie issuance/clearing on auth session endpoints.
- [ ] Add CSRF token issuance and validation for session mutation endpoints.
- [ ] Add rate-limiting middleware and wire it into the router.
- [ ] Add baseline security headers middleware and wire it into the router.
- [ ] Harden import upload requests with filename checks, size caps, MIME allowlist, and malware-scan status hook.
- [ ] Extend import tests to cover new upload/finalize security behavior.
- [ ] Add privileged action step-up MFA checks for billing-sensitive mutations.
- [ ] Make impersonation explicitly disabled unless a config flag is enabled, with guardrails if enabled later.
- [ ] Add CI secret scanning.
- [ ] Add CI dependency vulnerability scanning for Go and npm.
- [ ] Create threat model v1 doc.
- [ ] Update `CLAUDE.md` with newly relevant project security/runtime notes.
- [ ] Update `TODO.md` once verified.

## Working assumptions

- Existing auth continues to support bearer-token API access for tests and admin/dev flows, while secure HttpOnly cookies are added for browser session compatibility and hardening.
- CSRF protection should apply to cookie-backed session endpoints, while bearer-token authenticated API requests continue to work without a CSRF token.
- Import uploads remain presigned-storage based; size/MIME validation will be enforced through upload metadata recorded at request/finalize time rather than by streaming file bytes through the API.
- Because no impersonation routes exist today, the safest completion is to make impersonation an explicit disabled-by-default capability with rejection semantics if someone attempts to enable it without the required safeguards.

## Verification target

- `go test ./...` in `server/`
- targeted router tests for auth/imports/billing security flows
- CI workflow YAML validates structurally after edits

