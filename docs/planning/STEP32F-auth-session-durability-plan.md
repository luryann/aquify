# STEP 32F Auth Session Durability And Scale Safety Plan

## Goal

Finish `TODO.md` step `32F)` by replacing any remaining process-local auth
session state with persistent/shared storage, then prove session refresh,
rotation, revocation, and expiry behavior remains correct across restarts and
cross-instance routing.

## Working Notes

- Verify whether the current auth/session implementation still uses in-memory
  refresh-token or session state in production paths.
- Confirm the canonical persistence layer for session lifecycle data and the
  exact schema/queries already available.
- Close any gaps in refresh rotation, revocation, TTL validation, and
  cross-instance visibility.
- Add regression coverage for restart/cross-instance refresh + revoke flows.
- Update `CLAUDE.md` if this work changes project-specific implementation
  conventions or operator expectations.

## Verification Checklist

- [x] Production auth/session lifecycle no longer depends on process-local state
- [x] Refresh token rotation/invalidation is persisted and enforced cross-instance
- [x] Restart and multi-router tests cover refresh and revoke behavior
- [x] TTL/expiry checks are enforced against persistent session records
- [x] `CLAUDE.md` updated if conventions changed
- [x] Final diff review confirms `TODO.md` step `32F` coverage
