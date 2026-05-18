# STEP33B CSRF and CORS Contract Alignment Plan

## Goal

Finish `TODO.md` step `33B)` by ensuring browser clients using cookie-backed
auth can send the configured CSRF header through CORS preflight checks.

## Scope

- Add the configured CSRF header name to API CORS `AllowedHeaders`.
- Add an integration-style server test that covers:
  - Cross-origin `OPTIONS` preflight for a cookie-auth mutation route.
  - Actual cookie-auth mutation request with matching CSRF cookie/header.

## Verification Checklist

- [x] Focused security tests pass.
- [x] Full server package tests pass or any unrelated blocker is documented.
- [x] `TODO.md` Step 33B is marked complete only after verification.
- [x] `CLAUDE.md` notes the CORS/CSRF browser contract if the implementation adds new project guidance.
- [x] Final diff review confirms no accidental changes beyond the task scope.
