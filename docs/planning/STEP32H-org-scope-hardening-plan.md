# STEP 32H Org Scope Middleware Hardening Plan

## Goal

Finish `TODO.md` step `32H)` by replacing the current `OrgScopeStub` middleware
with hardened production org-scope enforcement, reducing duplicated
principal-vs-header checks in handlers, and adding contract coverage for the
shared invariant across protected routes.

## Working Notes

- Replace `OrgScopeStub` naming/behavior with a production middleware that:
  requires `X-Org-Id`,
  normalizes the header value,
  and rejects requests whose authenticated principal belongs to a different org.
- Keep the normalized org id in request context so handlers can trust a single
  source of truth.
- Refactor existing helper functions to lean on centralized middleware and only
  perform domain-specific scope checks beyond org matching.
- Add contract tests for protected routes to prove:
  missing org header returns `400`,
  mismatched org header returns `403`,
  matching org header reaches the handler path.
- Update `CLAUDE.md` if this changes project conventions for scoped handlers.

## Verification Checklist

- [x] `OrgScopeStub` removed from production routing/middleware naming
- [x] Authenticated protected routes enforce org header presence centrally
- [x] Principal/header org mismatch is rejected before handler logic
- [x] Contract tests cover org-scope invariants on protected routes
- [x] `CLAUDE.md` updated if conventions changed
- [x] Final diff review confirms `TODO.md` step `32H` coverage
