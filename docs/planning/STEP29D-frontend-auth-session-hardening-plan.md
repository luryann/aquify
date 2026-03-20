# STEP29D Frontend Auth and Session Hardening Plan

## Goal

Finish `TODO.md` Step `29D) Frontend auth and session hardening` by replacing
dev-bootstrap assumptions with a real frontend session bootstrap flow,
removing production-visible dev auth affordances, and validating the browser
auth lifecycle against the existing backend session/cookie model.

## Current repo reality

- The frontend session provider still auto-seeds a dev stub owner session and
  relies on localStorage metadata instead of a real backend "current session"
  bootstrap.
- The login page points to `/api/v1/auth/oidc/authorize`, but that route does
  not exist in the backend today.
- The auth callback, MFA, and logout pages still simulate auth state changes
  instead of calling the real backend routes.
- Dev-only role switching is hidden behind `NODE_ENV === 'development'`, but
  Step 29D still requires removing production dependence on those flows and
  tightening deployment-facing config.
- A few admin screens still use localStorage-only draft persistence to fake
  state changes that never reach the backend. That broader live-data conversion
  belongs to Step 29E/29F, but any auth/session-specific persistence shortcuts
  should be removed in this pass.

## Execution strategy

1. Add a real backend current-session bootstrap/profile endpoint that works
   with the existing cookie-backed auth middleware and returns enough identity
   metadata for admin/parent/swimmer routing.
2. Update backend auth handlers so browser login/logout/MFA flows can complete
   fully with cookies instead of simulated client-only state.
3. Replace frontend session bootstrap logic with a real `/api/v1/auth/session`
   fetch, including refresh-on-401 and clean unauthorized-state handling.
4. Hide dev-only stub login and role switching behind explicit frontend feature
   flags so production builds and configs do not depend on them.
5. Update auth callback, MFA, and logout screens to call the real backend
   routes and route users to the correct surface (`/dashboard`, `/portal`, or
   `/swim`) based on the resolved session.

## Scope for this pass

- Backend auth/session bootstrap endpoint
- Cookie-backed frontend session hydration
- Real callback, refresh, logout, and MFA browser flows
- Production-safe gating for stub login / role switching
- Targeted tests for the new backend auth/session behavior

## Explicit non-goals

- Full admin live-data conversion away from mock datasets
- Parent/swimmer portal data conversion
- Real staging login verification against an external Keycloak deployment

Those remain Step 29E/29F work or environment verification tasks.

## Checkpoints

- [ ] Add Step 29D planning artifact
- [ ] Add backend current-session bootstrap/profile endpoint
- [ ] Set cookie-backed session state during MFA completion where needed
- [ ] Replace frontend simulated auth flows with real API calls
- [ ] Gate dev auth affordances behind explicit non-production flags
- [ ] Add/update targeted auth tests
- [ ] Update `CLAUDE.md` and `TODO.md`
