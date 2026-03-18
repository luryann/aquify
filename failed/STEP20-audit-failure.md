# STEP 20(0.1) Post-Implementation Audit Failure

**Step:** STEP 20(0.1) — Admin UI Screens (v1 Core)
**Date:** 2026-03-17
**Verdict:** FAIL — Condition 1 (no bugs/potential issues) is not well-met.

---

## Conditions Evaluated

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | ❌ FAIL |
| 2 | No regressions in code quality | ⚠️ MARGINAL |
| 3 | No clear code security issues | ✅ PASS |
| 4 | Directions followed and completed fully | ✅ PASS |

---

## Condition 1 — Bugs / Issues (FAIL)

### Bug 1 — Sorting Silently Broken on All Detail-Page Tables (HIGH)

**What went wrong:**
`Table.tsx` has two levels of sorting opt-in: (a) a table-level `sortable` prop (defaults to `false`), and (b) a per-column `sortable: true` in `ColumnDef`. The render guard is:

```ts
// Table.tsx ~line 411
sortable && col.sortable !== false
```

Because the table-level `sortable` prop defaults to `false`, **column-level `sortable: true` entries are silently ignored** unless the consumer also passes `sortable` on the `<Table>` element. No detail-page consumer passes this prop. As a result, sorting is completely non-functional on every table in the following pages despite columns being designed as sortable:

- `admin/app/(admin)/members/households/[id]/page.tsx` — `SWIMMER_COLS`, `INVOICE_COLS`
- `admin/app/(admin)/members/swimmers/[id]/page.tsx` — `BT_COLS`
- `admin/app/(admin)/members/groups/[id]/page.tsx` — `SWIMMER_COLS`
- `admin/app/(admin)/meets/[id]/page.tsx` — `COMMITMENT_COLS`, `ENTRY_COLS`

**How to fix:**
Option A (preferred — no consumer changes): Change the `sortable` prop default in `Table.tsx` from `false` to `true`:

```ts
// Table.tsx — in the TableProps destructure
{
  sortable = true,    // was: sortable = false
  ...
}
```

Option B: Add `sortable` to every `<Table>` usage in all affected pages.

**What counts as passing:** Clicking column headers marked `sortable: true` should visually sort the table data, showing a direction indicator icon, on every table in households, swimmers, groups, and meets detail pages.

---

### Bug 2 — `useParams()` Return Value Discarded; URL `id` Never Read (MEDIUM)

**What went wrong:**
In `households/[id]/page.tsx`, `useParams()` is called but the return value is thrown away:

```ts
// households/[id]/page.tsx line 163
useParams() // ensure params context is loaded  ← WRONG
```

The actual URL parameter (`id`) is never extracted. The page always renders mock data for a hardcoded ID regardless of navigation. The same pattern appears in `meets/[id]/page.tsx` and `swimmers/[id]/page.tsx` (which do not call `useParams()` at all).

**How to fix:**
Change the call to capture the ID so it is ready when API wiring begins:

```ts
const params = useParams()
const id = params?.id as string | undefined
```

This is non-breaking now (mock data is used regardless) but prevents a refactoring trap during STEP 20A API integration.

**What counts as passing:** Each `[id]` page extracts its ID from `useParams()` and stores it in a variable, even if mock data is still used.

---

### Bug 3 — Default Auth Redirect Target `/login` Does Not Exist (LOW)

**What went wrong:**
`AuthGuard.tsx` defaults `redirectTo` to `'/login'`:

```ts
// AuthGuard.tsx
export function AuthGuard({ redirectTo = '/login', ... })
```

There is no `/login` route in the admin application. When an unauthenticated user hits a protected route, the `router.replace('/login')` call navigates to a 404. In development this is masked by the `stub-owner` auto-seed. In a real deployment it would produce an infinite redirect or blank page.

**How to fix:**
Either create a minimal `/login` page (can be a stub that redirects to Keycloak OIDC) or change the default to a route that exists — e.g., `redirectTo = '/'`. Since Keycloak is the auth provider, the login entry point is an OIDC redirect, not an internal page. A simple redirect page at `/login` that triggers the OIDC flow is the correct solution.

**What counts as passing:** Unauthenticated users navigating to a protected admin route are redirected to a page that exists (or triggers the Keycloak OIDC flow).

---

## Condition 2 — Code Quality (MARGINAL)

The following issues are code quality concerns that do not rise to bugs but represent technical debt:

- **`api.ts`:** `Content-Type: application/json` is set unconditionally on all requests including `GET` and `DELETE` (which have no body). Some strict server middleware may reject this. The header should only be set when a body is present.

- **`session.tsx`:** `DEV_SESSIONS` uses `Date.now()` at module load time for `expiresAt`. Functionally harmless (30-day window) but semantically wrong — it should use `Date.now()` at session creation time.

- **`swimmers/[id]/page.tsx`, `meets/[id]/page.tsx`:** `key={i}` used for dynamic-looking lists (history timeline, event selection items). Using array index as key produces incorrect diff behavior on insertion/deletion. Should use stable IDs when data is real.

These are documented but do not rise to a FAIL on their own.

---

## Condition 3 — Security (PASS)

No high-severity security issues found. The localStorage token strategy is appropriately scoped to development (gated by `APP_ENV === 'development'` and `AUTH_ENABLE_DEV_STUB_TOKENS`). No XSS vectors, no SQL injection surface, no unvalidated external redirects, no secrets in source.

---

## Condition 4 — Completeness (PASS)

All screens listed in `STEP20-implementation-plan.md` were built:
- ✅ Households list + detail (4 tabs)
- ✅ Swimmers list + detail (4 tabs)
- ✅ Groups list + detail (3 tabs)
- ✅ Registrations list
- ✅ Meets list + detail (5 tabs)
- ✅ Tabs component
- ✅ Design system and CSS token compliance

The TODO.md entry "Build swimmer create/edit screen" references form creation, which the STEP20 implementation plan explicitly defers to a future phase. The STEP20 plan itself (the authoritative per-step document) marks all planned items complete. This is not a failure of this step.

---

## Master File List — All Files Created or Modified in STEP 20(0.1)

### New Files Created
```
admin/components/ui/Tabs.tsx
admin/app/(admin)/members/households/page.tsx
admin/app/(admin)/members/households/[id]/page.tsx
admin/app/(admin)/members/swimmers/page.tsx
admin/app/(admin)/members/swimmers/[id]/page.tsx
admin/app/(admin)/members/groups/page.tsx
admin/app/(admin)/members/groups/[id]/page.tsx
admin/app/(admin)/members/registrations/page.tsx
admin/app/(admin)/meets/page.tsx
admin/app/(admin)/meets/[id]/page.tsx
```

### Modified Files
```
admin/components/ui/Table.tsx        (ColumnDef.render signature updated, row-first)
admin/lib/roles.ts                   (nav paths updated, members prefix, meets path)
```

### Pre-existing Files (unchanged by STEP 20 but part of foundation)
```
admin/lib/api.ts
admin/lib/session.tsx
admin/components/layout/Sidebar.tsx
admin/components/layout/Topbar.tsx
admin/components/auth/AuthGuard.tsx
admin/app/(admin)/layout.tsx
admin/app/(admin)/dashboard/page.tsx
admin/app/providers.tsx
admin/app/globals.css
admin/components/ui/icons.tsx
admin/components/ui/Toast.tsx
admin/components/ui/Skeleton.tsx
admin/components/ui/EmptyState.tsx
admin/components/forms/FormWrapper.tsx
admin/app/page.tsx
admin/package.json
admin/tsconfig.json
admin/next.config.ts
admin/app/layout.tsx
```

---

## Required Fixes to Pass

1. **[HIGH — required]** Fix sorting: Change `sortable` default in `Table.tsx` from `false` to `true`, or add `sortable` prop to all affected `<Table>` usages.
2. **[MEDIUM — required before API wiring]** Fix `useParams()` discard in `households/[id]/page.tsx`; add `useParams()` with capture to `meets/[id]/page.tsx` and `swimmers/[id]/page.tsx`.
3. **[LOW — recommended]** Create a `/login` stub page or change `AuthGuard` default redirect to an existing route.
