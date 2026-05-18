# STEP29E Admin Live-Data Conversion Plan

## Goal

Finish the first active Step 29E conversion slice by replacing remaining admin
membership, meets, imports, exports, invoices, and ledger mock datasets with
live API-backed queries and removing production-facing mock-only notices where
real persistence now exists.

## Source context reviewed

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`
- `docs/planning/STEP29D-frontend-auth-session-hardening-plan.md`

## Current repo reality

- Admin list screens under these domains still render hard-coded arrays:
  - membership: households, swimmers, groups, registrations
  - meets
  - imports
  - exports
  - invoices
  - ledger
- Some backend surfaces already exist for these pages:
  - swimmers, groups, meets, imports, exports, invoices, ledger
- Some membership list surfaces are missing and must be added before the admin
  UI can stop using mocks:
  - household listing
  - registration listing
- The broader Step 29E scope still contains reports, audit, jobs, billing
  overview/settings, performance, compliance, comms, and suppression screens.
  Those are not part of this first execution slice.

## Execution strategy

1. Add a lightweight plan artifact for Step 29E tracking.
2. Add missing backend admin membership read endpoints.
3. Add a small frontend live-data helper layer for authenticated admin queries.
4. Replace mock datasets on the targeted admin screens with live API fetches,
   loading states, error states, and derived stats based on returned data.
5. Remove mock-only "created locally" persistence assumptions from converted
   screens.
6. Verify with targeted backend tests plus `npm run build` for the admin app.
7. Update `CLAUDE.md` and `TODO.md` to reflect the newly completed slice.

## Targeted screens in this pass

- `admin/app/(admin)/members/households/page.tsx`
- `admin/app/(admin)/members/swimmers/page.tsx`
- `admin/app/(admin)/members/groups/page.tsx`
- `admin/app/(admin)/members/registrations/page.tsx`
- `admin/app/(admin)/meets/page.tsx`
- `admin/app/(admin)/imports/page.tsx`
- `admin/app/(admin)/exports/page.tsx`
- `admin/app/(admin)/invoices/page.tsx`
- `admin/app/(admin)/ledger/page.tsx`

## Backend gaps to close

- Add `GET /api/v1/members/households`
- Add `GET /api/v1/registrations`

## Verification checklist

- [ ] Backend membership list endpoints return scoped live data
- [ ] Converted admin pages compile and render from API data
- [ ] No targeted page depends on a hard-coded mock dataset for primary content
- [ ] `CLAUDE.md` updated with any new runtime/API notes
- [ ] `TODO.md` updated to reflect completed conversion slice
