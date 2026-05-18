# STEP33A Auth Token Exposure Hardening Plan

## Goal

Finish `TODO.md` step `33A)` by ensuring cookie-auth session endpoints do not
return access or refresh token values in JSON response bodies. Session secrets
must be issued and rotated only through secure HttpOnly cookies, while response
bodies expose only non-sensitive session metadata needed by the browser UI.

## Scope

- `POST /api/v1/auth/oidc/callback`
- `POST /api/v1/auth/sessions/issue`
- `POST /api/v1/auth/sessions/refresh`
- Related session replacement flows that share the same response shape, such as
  MFA challenge, to avoid reintroducing token fields through a parallel path.

## Implementation Notes

- `server/internal/app/handlers/auth.go` now serializes session responses with
  `authSessionMetadata(...)`, which includes `expires_at` but omits token
  fields.
- Cookie issuance still uses the full `authn.SessionTokens` values internally
  via `appmiddleware.SetSessionCookies(...)`.
- Regression tests live in
  `server/internal/app/auth_response_contract_test.go` and recursively assert
  that response JSON omits `access_token` and `refresh_token`.
- Shared test helpers now retrieve issued bearer tokens from the HttpOnly access
  token cookie instead of expecting token values in JSON bodies.

## Verification Checklist

- [x] Focused auth/security tests pass.
- [x] Full server package tests pass or any unrelated blocker is documented.
- [x] `TODO.md` Step 33A is marked complete only after verification.
- [x] `CLAUDE.md` notes the cookie-only auth response contract.
- [x] Final diff review confirms no accidental changes beyond the task scope.
