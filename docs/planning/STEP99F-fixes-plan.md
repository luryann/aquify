# STEP 99F — Auth + UI Bug Fixes

## Issues

1. **Logout + refresh → back to logged-in screens** — AdminShell calls `signOut()` first (clears React state) then navigates to /logout. If user refreshes before the /logout page's API revoke completes, the session cookie is still valid and `fetchCurrentSession` returns a live session.
   - **Fix**: In AdminShell's `handleSignOut`, await `POST /api/v1/auth/sessions/revoke` before clearing state and navigating, so the cookie is gone before any navigation occurs.

2. **`make dev-full` resets passwords** — The Makefile always copies the realm JSON to `data/import/` before starting Keycloak with `--import-realm`, causing Keycloak to overwrite the realm (and reset all user passwords) on every start.
   - **Fix**: Only copy the realm file (and thus trigger import) if the Keycloak H2 database doesn't already exist (`keycloak-26.5.6/data/h2/keycloakdb.mv.db`).

3. **Add user accounts (not just invite)** — Users page only has "Invite User" (send welcome email). Need a "Create User" flow that sets a password directly.
   - **Fix**: Add `SetUserPassword` to Keycloak client; add `POST /users` endpoint; add `createUser` to adminData.ts; add "Create User" modal to users/page.tsx.

4. **Remember me centered + 2 lines, Password label too high** — Keycloak login CSS issues.
   - **Fix**: Update `login.css` to add `white-space: nowrap` and better flex control to the form row; add top spacing for password step.

5. **MFA cancel → redirects back to MFA** — Cancel button does `router.push('/login')` but session still exists with `mfaAuthenticated: false`, causing AuthGuard to redirect back to `/mfa`.
   - **Fix**: `cancelLink` must call `signOut()` first (which revokes session) then navigate.

6. **No MFA enrolled → still asks for a code** — When factors API fails or TOTP enrollment fails, the catch block defaults to `setStage('verify')` with no factorId, showing a broken code input.
   - **Fix**: Show a proper error/retry state when enrollment fails instead of defaulting to verify stage.

## Files to Modify
- `admin/components/layout/AdminShell.tsx` — Fix #1
- `Makefile` — Fix #2
- `server/internal/keycloak/client.go` — Fix #3 (SetUserPassword)
- `server/internal/app/handlers/users.go` — Fix #3 (POST /users)
- `admin/lib/adminData.ts` — Fix #3 (createUser)
- `admin/app/(admin)/users/page.tsx` — Fix #3 (Create User modal)
- `keycloak-26.5.6/themes/aquify/login/resources/css/login.css` — Fix #4
- `admin/app/mfa/page.tsx` — Fix #5, Fix #6
