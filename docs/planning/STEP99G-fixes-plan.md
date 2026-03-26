# STEP 99G — Bug Fixes Plan

## Issues

### 1. MFA enrollment not surviving server reboots
- **Root cause**: `mfaFactorsByUser map[string][]mfaFactorRecord` in `authn.Service` is purely in-memory. Server restart wipes all enrolled TOTP/backup-code factors.
- **Fix**: Add a `mfa_factors` DB table and persist factors on enroll; load on startup.
- **Files**:
  - `db/migrations/20260324000300_create_mfa_factors.sql` (new)
  - `server/internal/app/authn/service.go` — add `pool`, `SetPool`, `loadMFAFactorsFromDB`, `saveMFAFactorToDB`, `updateMFAFactorInDB`; wire into `EnrollMFA` / `ChallengeMFA`
  - `server/internal/app/server.go` — call `authService.SetPool(dbPool)` after pool is available

### 2. `keycloak_user_id` column missing on members load
- **Root cause**: Migration `20260324000100_add_keycloak_user_id_to_households.sql` has not been applied yet.
- **Fix**: Run `make db-up` (migration files already exist). Document in planning.

### 3 & 4. WARN postmark/keycloak init failed in dev
- **Root cause**: Missing credentials → `logger.Warn(...)`. Expected in dev but loud.
- **Fix**: Downgrade from `Warn` to `Info` in `allowsProviderStubFallback` paths in `server.go`.

### 5. Session not cleared on logout (hard refresh / new sign-in returns to previous dashboard)
- **Root cause**: `defer appmiddleware.ClearSessionCookies(w)` in `revokeSession` fires AFTER `httpx.WriteError` / `w.WriteHeader()` commits response headers. Go's `net/http` ignores `w.Header().Add(...)` after `WriteHeader` is called — the Set-Cookie headers are silently dropped on every code path (error AND success).
- **Fix**: Remove the defer. Call `ClearSessionCookies(w)` as the very first statement in `revokeSession`, before any `WriteHeader` call. Also: if no refresh token is found at all, skip the revoke error and return 204 (cookies are cleared; that's the observable effect the client needs).

## Implementation order
1. Fix revokeSession (high impact, 3 lines)
2. Downgrade log level for stub fallbacks (1 line each)
3. Create mfa_factors migration
4. Wire MFA DB persistence in authn service
5. Update server.go to call SetPool on authService
6. Run `make db-up` (user action)
