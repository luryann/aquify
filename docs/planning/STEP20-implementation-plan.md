# STEP 20 — Admin UI Screens (v1 Core) — Implementation Plan

## Status: IN PROGRESS

## Scope: STEP 20(0.1) — Core Admin Screens

### Screens to Build
1. **Members > Households** — list + detail (tabs: Overview, Swimmers, Contacts, Invoices)
2. **Members > Swimmers** — list + detail (tabs: Profile, Performance, Groups, History)
3. **Members > Groups** — list + detail (tabs: Members, Eligibility Rules, Settings)
4. **Members > Registrations** — list only
5. **Competition > Meets** — list + detail (tabs: Overview, Sessions, Events, Commitments, Entries)

### Aesthetic Direction
Continuing STEP 19's "Industrial Precision" — Bloomberg Terminal × Vercel Dashboard.
- Dense information tables
- Monospace data values (JetBrains Mono)
- Status badges with semantic color coding
- Card-based section layout on detail pages
- Tab navigation on detail pages

### Architecture Decisions
- All screens use client components (search/filter/tabs require interactivity)
- Mock/stub data inline in each page (API wiring deferred)
- Shared `Tabs` component for detail page tab navigation
- Use existing `Table`, `EmptyState`, `Icon`, `Toast` primitives
- CSS variables throughout, no Tailwind, no new external packages

### File Plan
```
admin/
  components/ui/Tabs.tsx                              [NEW]
  app/(admin)/
    members/
      households/
        page.tsx                                      [NEW] list
        [id]/page.tsx                                 [NEW] detail
      swimmers/
        page.tsx                                      [NEW] list
        [id]/page.tsx                                 [NEW] detail
      groups/
        page.tsx                                      [NEW] list
        [id]/page.tsx                                 [NEW] detail
      registrations/
        page.tsx                                      [NEW] list
    meets/
      page.tsx                                        [NEW] list
      [id]/page.tsx                                   [NEW] detail
```

### Progress
- [x] Planning doc
- [x] Tabs component
- [x] Households list
- [x] Household detail
- [x] Swimmers list
- [x] Swimmer detail
- [x] Groups list
- [x] Group detail
- [x] Registrations list
- [x] Meets list
- [x] Meet detail

## STEP 20A — Status: COMPLETE

### Screens Built
- [x] Imports list (`/imports`) — file type + status filter, conflict count callouts, stats strip
- [x] Import detail (`/imports/[id]`) — tabs: Overview, Rows, Conflicts, Report; interactive conflict resolution cards with candidate matching + accept/reject actions; progress bar
- [x] Exports list (`/exports`) — status + search filter, expiry warnings, download/retry actions, stats strip
- [x] Invoices list (`/invoices`) — status + search filter, open balance / collected / overdue stats, overdue badges
- [x] Invoice detail (`/invoices/[id]`) — tabs: Overview, Line Items, Payments, Refunds, Ledger; payment progress bar; lifecycle actions based on status

### Notes
- STEP 20B will cover: Payments/refunds, announcements, email campaigns, audit logs
- STEP 20C-E: Parent and Swimmer portal UX
