# STEP 19 — Admin UI Foundation: Implementation Plan

## Goal
Build the Next.js admin portal foundation: layout shell, auth guard, session state, API client,
and core UI primitives (toast, skeleton, empty state, table, form wrapper).

## Aesthetic Direction: "DARE Precision"

Industrial-precision dark console. Think Bloomberg Terminal × Vercel Dashboard.
- Dark-first (#0A0A0B base, #111113 sidebar, #1A1A1C surface)
- Instrument Serif for brand mark + page headings → editorial-luxury contrast in data context
- JetBrains Mono for values, IDs, metrics
- Plus Jakarta Sans for UI chrome
- #eb5d1e accent used sparingly: active nav, focus rings, CTA buttons
- Hairline borders (rgba(255,255,255,0.07)) everywhere
- Tight 48px topbar, 224px sidebar
- Micro-animations: 120ms hover, staggered sidebar nav reveal on mount

## File Plan

### Core Infrastructure
- `app/globals.css` — Full design token system (rewrite)
- `app/layout.tsx` — Root layout: fonts, dark theme, providers
- `app/providers.tsx` — Client-side SessionProvider + ToastProvider
- `lib/roles.ts` — Role types, nav config, permission helpers
- `lib/session.tsx` — Session context, localStorage persistence
- `lib/api.ts` — Typed API client factory

### UI Primitives
- `components/ui/icons.tsx` — Inline SVG icon set
- `components/ui/Toast.tsx` — Toast context + container (bottom-right)
- `components/ui/Skeleton.tsx` — Shimmer skeleton shapes
- `components/ui/EmptyState.tsx` — Empty state with icon/action
- `components/ui/Table.tsx` — Sortable, paginated data table
- `components/forms/FormWrapper.tsx` — React Hook Form + Zod

### Layout
- `components/layout/Sidebar.tsx` — Role-filtered sidebar navigation
- `components/layout/Topbar.tsx` — Breadcrumb + user/role display
- `components/auth/AuthGuard.tsx` — Session-based route protection

### App Shell
- `app/(admin)/layout.tsx` — Admin shell (Sidebar + Topbar + AuthGuard)
- `app/(admin)/dashboard/page.tsx` — Dashboard overview placeholder

## Dependencies to Install
- `react-hook-form@^7.54` — Form state management
- `@hookform/resolvers@^3.9` — Zod integration
- `zod@^3.24` — Schema validation

## Role → Nav Visibility Matrix

| Section | owner | admin | coach | billing | read-only |
|---------|-------|-------|-------|---------|-----------|
| Dashboard | ✓ | ✓ | ✓ | ✓ | ✓ |
| Members | ✓ | ✓ | ✓ | - | ✓ |
| Competition | ✓ | ✓ | ✓ | - | ✓ |
| Files | ✓ | ✓ | ✓ | - | ✓ |
| Billing | ✓ | ✓ | - | ✓ | ✓ |
| Comms | ✓ | ✓ | ✓ | - | ✓ |
| System | ✓ | ✓ | - | - | - |

## Status — COMPLETE ✓
- [x] Install dependencies (react-hook-form, @hookform/resolvers, zod)
- [x] globals.css — full design token system, dark-first, animations, utilities
- [x] lib/roles.ts — Role type, NAV_ITEMS, filterNavItems, canAccess, ROLE_HIERARCHY
- [x] lib/session.tsx — SessionProvider, useSession, localStorage persistence, dev stubs
- [x] lib/api.ts — createApiClient factory, ApiError, buildQuery
- [x] components/ui/icons.tsx — 50+ inline SVG icon set
- [x] components/ui/Toast.tsx — ToastProvider, useToast, 4 variants, progress bar
- [x] components/ui/Skeleton.tsx — Shimmer shapes (text, avatar, badge, table, card, stat-grid)
- [x] components/ui/EmptyState.tsx — EmptyState, LoadingState, ErrorState
- [x] components/ui/Table.tsx — Sortable, paginated, selectable data table
- [x] components/forms/FormWrapper.tsx — Full RHF+Zod integration with all field types
- [x] components/layout/Sidebar.tsx — Collapsible, role-filtered, staggered reveal
- [x] components/layout/Topbar.tsx — Breadcrumb + status indicator
- [x] components/auth/AuthGuard.tsx — Session guard with loading splash + forbidden screen
- [x] app/providers.tsx — Client-side SessionProvider + ToastProvider wrapper
- [x] app/layout.tsx — Updated with dark theme, providers, font variables
- [x] app/(admin)/layout.tsx — Admin shell route group
- [x] app/(admin)/dashboard/page.tsx — Dashboard with stats, activity, system status
- [x] Build passes clean (`next build` ✓)
