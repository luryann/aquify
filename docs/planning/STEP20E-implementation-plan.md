# STEP 20E Implementation Plan

## Status: IN PROGRESS

## Overview
STEP 20E completes the parent portal communications + calendar screens, introduces the optional swimmer portal behind a feature flag, and adds non-frontend stubs for e2e tests and mobile QA checklists.

---

## Frontend Deliverables

### Parent Portal (continues warm ivory aesthetic)
1. **`/portal/communications`** — Combined communications inbox + notification preferences
   - Tab 1: Announcements feed (read/unread state, pinned, category filter)
   - Tab 2: Notification Preferences (toggles: email/push for each event type)
   - Update `PortalNav` to include "Communications" link (replaces "Announcements" direct link)

2. **`/portal/calendar`** — Calendar/schedule screen
   - Month view (default) + Week view toggle
   - Event types: meets (orange accent), practices (blue), deadlines (red/amber)
   - Mini sidebar with upcoming events list
   - Color-coded day cells with event dots

### Swimmer Portal (new, distinct aesthetic)
- **Aesthetic**: Deep navy performance feel — contrast with light parent portal
- Background: #0D1B2A (deep navy), accent: #00D4AA (electric teal)
- Font: Fraunces (display), DM Sans (body), JetBrains Mono (mono)
- Feels like a training dashboard / sports app

3. **Route group**: `admin/app/(swimmer)/` with layout + AuthGuard (swimmer role)
4. **Shell components**: `SwimmerShell.tsx`, `SwimmerNav.tsx`
5. **`/swim`** — Swimmer dashboard (schedule, announcements, meet status)
6. **`/swim/performance`** — Best times by course/event, progression chart, cut qualification
7. **`/swim/profile`** — Read-only profile (limited edits per policy)
8. **`/swim/schedule`** — Practices + upcoming meets calendar-style

### Feature Flag
- `NEXT_PUBLIC_SWIMMER_PORTAL_ENABLED` env var
- AuthGuard on swimmer routes checks flag + swimmer role
- Disabled state page: "Swimmer accounts not yet enabled — please log in as a parent"

---

## Non-Frontend Deliverables
- `docs/testing/e2e-parent-flows.md` — e2e test stubs (registration, commitments, payment without swimmer login)
- `docs/testing/mobile-qa-checklist.md` — mobile-first QA checklist for parent critical flows
- `docs/testing/policy-acceptance-tests.md` — policy checks (no launch-critical flow requires swimmer account)
- `admin/lib/featureFlags.ts` — feature flag utility

---

## Progress Tracker
- [x] STEP20E planning doc created
- [x] PortalNav updated (Communications + Calendar links, removed standalone Announcements)
- [x] /portal/communications page (Inbox + Notifications tabs)
- [x] /portal/calendar page (month/week view, type filters, sidebar)
- [x] Feature flag utility (admin/lib/featureFlags.ts)
- [x] Swimmer portal route group + layout (admin/app/(swimmer)/)
- [x] SwimmerShell + SwimmerNav components
- [x] /swim dashboard page
- [x] /swim/performance page (Best Times / Cut Qualification / Progression tabs)
- [x] /swim/profile page (read-only fields + editable bio)
- [x] /swim/schedule page (chronological sections, type filter)
- [x] e2e test stubs doc (docs/testing/e2e-parent-flows.md)
- [x] Mobile QA checklist doc (docs/testing/mobile-qa-checklist.md)
- [x] Policy acceptance tests doc (included in e2e-parent-flows.md)
- [x] CLAUDE.md update

## Status: COMPLETE
