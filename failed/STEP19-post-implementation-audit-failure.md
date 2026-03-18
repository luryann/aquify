# STEP 19 Post-Implementation Audit Failure Report

**Date:** 2026-03-16
**Step:** STEP 19 — Admin UI Foundation
**Overall Verdict:** PARTIAL FAIL — Conditions 1, 2, 4 pass after remediation. Condition 3 (Security) fails on two residual concerns.

---

## Condition Results

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **PASS** (after remediation) |
| 2 | No regressions in code quality | **PASS** (after remediation) |
| 3 | No clear code security issues | **FAIL** (two residual concerns documented below) |
| 4 | Directions followed and completed fully | **PASS** (after remediation) |

---

## Bugs Fixed During Audit (Condition 1)

The following bugs were found and fixed during this audit session:

- **`app/(admin)/layout.tsx`** — File had `'use client'` directive, making it a Client Component. This violates Next.js App Router conventions (layouts should be Server Components). `AdminShell` was extracted to `components/layout/AdminShell.tsx` as a proper `'use client'` boundary; the layout is now a Server Component.
- **`lib/roles.ts:138` — RBAC `canAccess` bug** — `canAccess` used numeric hierarchy comparison (`hasRole`), allowing the `billing` role to access Competition, Files, and Communications nav sections via the `read-only` rank shortcut. Fixed: `canAccess` now uses `allowedRoles.includes(userRole)` (exact membership check).
- **`components/auth/AuthGuard.tsx:171`** — On unauthenticated render cycle before redirect fired, `UnauthorizedScreen` flashed briefly. Fixed: now shows `LoadingSplash` while the `router.replace()` fires.
- **`lib/api.ts:106`** — `useApiClient` returned a new `ApiClient` object reference on every render, causing infinite loops in `useEffect` dependencies. Fixed: result is now wrapped in `useMemo` keyed on `token`.
- **`components/ui/Toast.tsx:72`** — `ToastContextValue` was recreated on every render because `shorthand(...)` returns new function references each call. Fixed: context `value` is now wrapped in `useMemo`.
- **`components/forms/FormWrapper.tsx:141`** — `registerOptions?: RegisterOptions<T>` prop was declared on `TextFieldProps` but never consumed in the implementation. Fixed: removed the dead prop and its `RegisterOptions` import.
- **`app/(admin)/dashboard/page.tsx:163`** — Date was hardcoded as `"Mon, Mar 16 2026"`. Fixed: now uses `new Date().toLocaleDateString(...)` which renders at request time (Server Component).

---

## Security Failures (Condition 3)

### Issue 1 — Auth tokens stored in `localStorage`

**File:** `admin/lib/session.tsx` (lines 65–83, 88–93)
**What went wrong:** The `Session` object, including the `token` (bearer token) and `refreshToken`, is serialized to `localStorage` under the key `dare:session`. `localStorage` is accessible to any JavaScript on the same origin. Any XSS attack—including those introduced by future third-party npm packages—can exfiltrate the token without any additional privilege escalation. This is the most common vector for session token theft in SPAs.

**Why it was done this way:** The STEP 19 plan explicitly specified "localStorage-backed SessionProvider." This was intentional for the development/scaffolding phase, paired with `AUTH_ENABLE_DEV_STUB_TOKENS`. However, the storage mechanism does not distinguish between stub tokens and real tokens — when real Keycloak OIDC tokens are wired in (a future step), they will be stored in the same insecure location unless the architecture is explicitly changed.

**How to fix it:**
Replace `localStorage` with `HttpOnly` cookie storage for the session token. The cookie must be set by the backend (`Set-Cookie: session=...; HttpOnly; Secure; SameSite=Strict`) and the frontend must never have direct access to the token value. The frontend can use a `GET /api/v1/auth/sessions/me` endpoint to hydrate session user info on load.

Alternatively for a pure-frontend approach during development: store only non-sensitive metadata (user ID, role, display name) in `localStorage`, and use a short-lived session cookie for the bearer token that is scoped to the API subdomain.

**What counts as passing:**
- Real session tokens (from Keycloak/OIDC) must not be stored in `localStorage`
- `HttpOnly; Secure; SameSite=Strict` cookies must be used for auth token persistence in production
- The frontend session provider may retain `localStorage` for non-sensitive display metadata only (name, role, avatar)
- `AUTH_ENABLE_DEV_STUB_TOKENS=false` must be enforced in non-development environments (already in CLAUDE.md)

---

### Issue 2 — `dangerouslySetInnerHTML` on SVG icon content

**File:** `admin/components/ui/icons.tsx` (line 123)
**What went wrong:** The `Icon` component uses `dangerouslySetInnerHTML={{ __html: paths }}` to inject SVG child content. The `paths` value comes from the static compile-time `ICONS` map. In the current implementation, no user-supplied data reaches this call, and the TypeScript `IconName` type prevents arbitrary strings from being passed as `name`. However, `dangerouslySetInnerHTML` on an SVG element bypasses React's XSS sanitization entirely. If any future developer adds a dynamic icon path derived from API data or user input (e.g., theme configuration, custom icon uploads), this becomes an unguarded stored XSS vector.

**How to fix it:**
Replace the `dangerouslySetInnerHTML` approach with JSX-based SVG rendering. The `ICONS` map should store `ReactNode` values (pre-defined `<path>`, `<polyline>`, `<rect>` elements) rather than raw HTML strings, and the `Icon` component should render them as `<svg>{paths}</svg>` children. This removes the `dangerouslySetInnerHTML` call entirely and brings SVG content under React's XSS protection.

Example refactor:
```tsx
import type { ReactNode } from 'react'

const ICONS: Record<IconName, ReactNode> = {
  'layout-grid': (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1"/>
      <rect x="14" y="3" width="7" height="7" rx="1"/>
      {/* ... */}
    </>
  ),
  // ...
}

export function Icon({ name, size = 16, color, ...props }: IconProps) {
  const content = ICONS[name]
  if (!content) return null
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none"
         stroke={color ?? 'currentColor'} strokeWidth={1.75}
         strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>
      {content}
    </svg>
  )
}
```

**What counts as passing:**
- No `dangerouslySetInnerHTML` on any SVG element that could receive non-static content
- Icon paths rendered as JSX children, not raw HTML strings
- OR: if the string-map approach is retained, a lint rule (`no-danger` ESLint rule) must be enforced that prevents any call to `dangerouslySetInnerHTML` outside of an explicit allowlist

---

## Files Edited by Claude in STEP 19 (Master List)

The following files were created or modified during STEP 19 (including remediation fixes applied during this audit):

### Created (new files)
- `admin/app/(admin)/dashboard/page.tsx`
- `admin/app/(admin)/layout.tsx`
- `admin/app/providers.tsx`
- `admin/components/auth/AuthGuard.tsx`
- `admin/components/forms/FormWrapper.tsx`
- `admin/components/layout/AdminShell.tsx` *(created during remediation)*
- `admin/components/layout/Sidebar.tsx`
- `admin/components/layout/Topbar.tsx`
- `admin/components/ui/EmptyState.tsx`
- `admin/components/ui/Skeleton.tsx`
- `admin/components/ui/Table.tsx`
- `admin/components/ui/Toast.tsx`
- `admin/components/ui/icons.tsx`
- `admin/lib/api.ts`
- `admin/lib/roles.ts`
- `admin/lib/session.tsx`

### Modified (existing files)
- `admin/app/globals.css` — design token system added
- `admin/app/layout.tsx` — providers, font variables, dark theme
- `admin/app/page.tsx` — preserved as API dev console (no changes to content)
- `admin/package.json` — react-hook-form, @hookform/resolvers, zod dependencies added

---

## Notes on Passing Conditions After Remediation

**Condition 1 (Bugs) — PASS:** All high-confidence bugs listed above were fixed. Build and lint pass with zero new errors. The only ESLint warning (`react-hooks/exhaustive-deps` in `app/page.tsx` line 175) is pre-existing in the root API dev console page, not in STEP 19 code.

**Condition 2 (Code Quality) — PASS:** `app/(admin)/layout.tsx` is now a proper Server Component. `AdminShell` is correctly placed as the `'use client'` boundary. All STEP 19 components follow Next.js 15 App Router conventions. TypeScript compiles without errors.

**Condition 4 (Completeness) — PASS:** All deliverables from the STEP 19 plan are present and verified by successful `npm run build`. The RBAC `canAccess` bug was fixed, ensuring role-based nav filtering is now correct.
