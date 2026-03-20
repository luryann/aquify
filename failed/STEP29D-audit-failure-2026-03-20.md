# STEP29D Audit Failure - 2026-03-20

## Result

STEP 29D does **not** pass the required audit bar.

The work is close, and the main runtime checks I ran succeeded:

- `npm run build` in `/Users/ryan/Documents/aquify/admin` passed
- Targeted backend auth/session tests passed:
  - `TestIssueSessionSetsSecureCookies`
  - `TestIssueSessionDisabledOutsideDevMode`
  - `TestRefreshSessionWithCookieRequiresCSRF`
  - `TestRefreshSessionWithCookieAndCSRFSucceeds`
  - `TestCurrentSessionEndpointAcceptsCookieAuthWithoutOrgHeader`
  - `TestMFAChallengeSetsReplacementSessionCookies`

However, the implementation still has a launch-relevant auth gap, so the audit conditions are **not** all well-met.

## 1) What went wrong

### Coach MFA redirect gap

The frontend OIDC callback flow does not route `coach` users into the MFA screen when the resolved session is not MFA-authenticated.

Evidence:

- [`/Users/ryan/Documents/aquify/admin/app/auth/callback/page.tsx:71`](/Users/ryan/Documents/aquify/admin/app/auth/callback/page.tsx:71) only checks:
  - `admin`
  - `owner`
  - `billing`

It does **not** include `coach`.

That means a coach session returned from `GET /api/v1/auth/session` with `mfa_authenticated: false` is sent straight to the normal destination instead of `/mfa`.

Why this fails the audit:

- The repo docs explicitly describe MFA enforcement expectations for coach/admin privileged users.
- STEP 29D specifically required verifying real login, MFA, logout, refresh, and unauthorized-state handling across surfaces.
- A coach role missing from the frontend MFA routing logic is a real auth behavior bug, not just a documentation mismatch.

### Why this matters

This is not a cosmetic issue. It creates a clear mismatch between the intended auth policy and the browser login flow:

- backend session state can correctly report `mfa_authenticated: false`
- but the frontend callback still grants coaches a normal post-login route
- so the browser experience does not enforce the expected MFA step for that role

## 2) How to fix it

1. Update the callback gating logic in [`/Users/ryan/Documents/aquify/admin/app/auth/callback/page.tsx`](/Users/ryan/Documents/aquify/admin/app/auth/callback/page.tsx) so `coach` is treated as MFA-required in the same way as the privileged roles already listed.
2. Prefer centralizing this as a small helper, for example `roleRequiresMFA(role)`, so the rule is not re-implemented ad hoc in multiple screens.
3. Add targeted coverage for the browser/session decision:
   - coach with `mfa_authenticated=false` must redirect to `/mfa`
   - coach with `mfa_authenticated=true` must proceed normally
   - parent/swimmer should continue to bypass MFA unless policy changes
4. Re-run the frontend build and the targeted auth/session tests after the change.

## 3) What counts as passing

STEP 29D should count as passing only when all of the following are true:

- A `coach` session with `mfa_authenticated=false` is redirected to `/mfa` from the callback flow.
- `admin`, `owner`, and any other intentionally MFA-gated roles still redirect correctly.
- `parent` and `swimmer` users still route to their normal destinations without an unnecessary MFA block.
- The frontend build passes.
- Targeted auth/session tests pass, including explicit coverage for the coach MFA path.
- The result aligns with the repo’s documented MFA expectations, not just the currently hardcoded callback condition.

## 4) Master list of files edited by me in STEP 29D

- `/Users/ryan/Documents/aquify/.env.example`
- `/Users/ryan/Documents/aquify/README.md`
- `/Users/ryan/Documents/aquify/docs/api/openapi.yaml`
- `/Users/ryan/Documents/aquify/docs/planning/STEP29D-frontend-auth-session-hardening-plan.md`
- `/Users/ryan/Documents/aquify/CLAUDE.md`
- `/Users/ryan/Documents/aquify/TODO.md`
- `/Users/ryan/Documents/aquify/admin/lib/session.tsx`
- `/Users/ryan/Documents/aquify/admin/app/login/page.tsx`
- `/Users/ryan/Documents/aquify/admin/app/logout/page.tsx`
- `/Users/ryan/Documents/aquify/admin/app/mfa/page.tsx`
- `/Users/ryan/Documents/aquify/admin/app/auth/callback/page.tsx`
- `/Users/ryan/Documents/aquify/admin/components/dev/DevRoleSwitcher.tsx`
- `/Users/ryan/Documents/aquify/admin/components/layout/AdminShell.tsx`
- `/Users/ryan/Documents/aquify/admin/components/portal/PortalShell.tsx`
- `/Users/ryan/Documents/aquify/admin/components/swimmer/SwimmerShell.tsx`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/auth.go`
- `/Users/ryan/Documents/aquify/server/internal/app/middleware/auth.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server.go`
- `/Users/ryan/Documents/aquify/server/internal/app/config.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server_security_test.go`

## Audit verdict against the 4 required conditions

1. No bugs, UI issues, potential issues, or edge cases:
   - **Not met**
2. No regressions in code quality:
   - **Not confidently met** because the role-based MFA rule is incomplete in the callback logic
3. No clear code security issues:
   - **Not met** because the coach MFA enforcement path is incomplete
4. Earlier directions were followed fully and completed fully:
   - **Not met** because the implementation does not fully satisfy the documented MFA expectation for coach users
