# STEP 20E Gap Closure Plan

## Goal
Build all pages present in SuggestedPageStructure.md that haven't yet been implemented. These span
auth routes, admin portal sections, and parent portal wizard flows.

## Missing Pages (15 total)

### Auth / Shared Routes
- [ ] `/app/auth/callback/page.tsx` — OIDC callback processing screen
- [ ] `/app/mfa/page.tsx` — TOTP / backup-code MFA challenge
- [ ] `/app/logout/page.tsx` — session revoke + redirect
- [ ] `/app/unauthorized/page.tsx` — 403 error page

### Admin Portal — Performance section (new nav section)
- [ ] `/app/(admin)/performance/time-standards/page.tsx`
- [ ] `/app/(admin)/performance/best-times/page.tsx`

### Admin Portal — Billing additions
- [ ] `/app/(admin)/billing/page.tsx` — Billing Overview dashboard
- [ ] `/app/(admin)/billing/stripe/page.tsx` — Stripe reconciliation / webhook log

### Admin Portal — Communications addition
- [ ] `/app/(admin)/comms/suppression/page.tsx` — Suppression List

### Admin Portal — Compliance section
- [ ] `/app/(admin)/compliance/data-exports/page.tsx`
- [ ] `/app/(admin)/compliance/data-deletions/page.tsx`

### Admin Portal — Settings section
- [ ] `/app/(admin)/settings/page.tsx` — System Settings
- [ ] `/app/(admin)/settings/roles/page.tsx` — Role Management

### Admin Portal — Meets
- [ ] `/app/(admin)/meets/new/page.tsx` — Create Meet wizard

### Parent Portal
- [ ] `/app/(portal)/portal/registrations/new/page.tsx` — New Registration wizard

## Nav Updates Required (roles.ts)
- Add 'Performance' to NAV_SECTIONS
- Add time-standards + best-times nav items
- Add billing overview + stripe nav items
- Add suppression nav item to Communications
- Add compliance + settings nav items to System
