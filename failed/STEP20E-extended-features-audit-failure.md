# STEP 20E Extended Features — Post-Implementation Audit Failure

**Date:** 2026-03-18
**Audited by:** Claude Code (automated code review agent)
**Result:** FAIL — Conditions 1, 2, and 4 not met

---

## Conditions Not Met

### Condition 1 — Bugs, potential issues, or edge cases: FAIL
Multiple critical and high-severity bugs found. Details below.

### Condition 2 — No regressions in code quality: FAIL
Multiple regressions in rendering correctness, re-render efficiency, and date handling introduced across new files.

### Condition 3 — No clear code security issues: PASS (with minor caveat)
No SQL injection, XSS, or direct secrets exposure found. One auth ordering concern noted (H-4).

### Condition 4 — All directions followed and completed fully: FAIL
Task #6 (Autopay) has zero implementation. Task #14 (sidebar nav for commitments/entries) is functionally broken. Tasks #1, #2, #4, #5, and #9 are incomplete stubs.

---

## What Went Wrong

### CRITICAL Issues

**C-1: Hardcoded "today" date breaks practices page**
- File: `admin/app/(admin)/practices/page.tsx`, ~line 257
- `const today = '2026-03-17'` is a string literal. All calendar highlighting, `totalThisWeek`, and `totalThisMonth` stats are computed against this frozen date. The page silently shows wrong data every day after March 17, 2026.
- Fix: `const today = new Date().toISOString().split('T')[0]` and derive week/month boundaries dynamically.

**C-2: Add Session modal silently discards all form data**
- File: `admin/app/(admin)/meets/[id]/page.tsx` — `handleSave` inside the Add Session modal
- The handler closes the modal (`setShowAddSession(false)`) with no state update. User fills out the form, clicks Save — modal closes, nothing is added, no error is shown.
- Fix: Collect controlled form state and call `setSessions(prev => [...prev, newSession])` before closing.

**C-3: Meet creation wizard discards all data on submit**
- File: `admin/app/(admin)/meets/new/page.tsx`, lines ~63–68
- `handleSave` does `await new Promise(r => setTimeout(r, 1400))` then `router.push('/meets')`. No API call, no state persistence. The entire 4-step wizard form is discarded. User is redirected with false impression of success.
- Fix: Wire the submit handler to the actual `/api/v1/meets` POST endpoint with collected form state.

**C-4: `handleReject` in entries page permanently deletes entries instead of transitioning state**
- File: `admin/app/(admin)/entries/page.tsx`, line ~93
- `setEntries(prev => prev.filter(e => e.id !== entryId))` removes the entry from local state entirely. Per the state machine in CLAUDE.md, the correct transition is `submitted → rejected`. The type `EntryStatus` doesn't even include `'rejected'`. No confirmation dialog is shown.
- Fix: Add `'rejected'` to `EntryStatus`, transition entry to that status, add confirmation dialog before irreversible action.

---

### HIGH Issues

**H-1: "New Meet" button is inert**
- File: `admin/app/(admin)/meets/page.tsx`, line ~172
- The button has no `onClick` handler and no `Link` wrapper. The `/meets/new` creation flow exists but is completely unreachable from this button.
- Fix: `<Link href="/meets/new">` or `onClick={() => router.push('/meets/new')}`.

**H-2: Task #14 broken — commitments and entries have no sidebar nav items**
- File: `admin/lib/roles.ts` — `NAV_ITEMS` array
- No entry with `path: '/commitments'` or `path: '/entries'` exists. Both pages are complete and functional but completely unreachable from the sidebar. Task #14 ("Add back commitment/entries screen to admin sidebar") is stated complete but is NOT implemented.
- Fix: Add nav items for Commitments and Entries to `NAV_ITEMS` with correct role visibility.

**H-3: `SECTION_ORDER` in Sidebar excludes `'Performance'` and `'Compliance'`**
- File: `admin/components/layout/Sidebar.tsx`
- `SECTION_ORDER` controls which sections render. Neither `'Performance'` nor `'Compliance'` is in the array. Nav items in those sections are silently dropped and never appear in the sidebar.
- Fix: Add `'Performance'` and `'Compliance'` to `SECTION_ORDER` in the correct position.

**H-4: `SwimmerShell` performs feature-flag check before auth check**
- File: `admin/components/swimmer/SwimmerShell.tsx`
- When `SWIMMER_PORTAL_ENABLED` is false, unauthenticated users see the "coming soon" UI instead of being redirected to login. This bypasses the auth gate and leaks portal existence.
- Fix: Move session/auth check before feature flag check.

**H-5: Meet list mock dates are all in 2025 (12+ months in the past)**
- File: `admin/app/(admin)/meets/page.tsx` — `MOCK_MEETS`
- Dates set in Jan–Apr 2025. Today is March 2026. Every "upcoming" meet is already in the past, breaking tab filtering during review or demo.
- Fix: Update mock dates to 2026 values, or compute relative to `new Date()`.

**H-6: `COLUMNS` array not memoized in entries page**
- File: `admin/app/(admin)/entries/page.tsx`
- `COLUMNS` is reconstructed inside the component on every render because it closes over `handleApprove`/`handleReject`. Triggers unnecessary downstream re-renders in the Table component.
- Fix: Wrap in `useMemo(() => [...], [handleApprove, handleReject])` and stabilize callbacks with `useCallback`.

---

### MEDIUM Issues

**M-1: Course filter in performance visualization does nothing**
- File: `admin/app/(admin)/performance/visualization/page.tsx`, line ~130
- `const [course, setCourse] = useState<'SCY'|'SCM'|'LCM'>('SCY')` is updated by buttons but `course` is never referenced in filter logic. Interactive control silently does nothing.
- Fix: Apply `course` filter when deriving swimmer/event stats.

**M-2: `ThemeToggle` flashes wrong icon on initial render**
- File: `admin/components/layout/Topbar.tsx`, line ~51
- `useState(true)` assumes dark mode. `useEffect` corrects after first paint. In light mode, sun icon briefly shows then snaps to moon on every page load.
- Fix: Use lazy initializer reading `document.documentElement.getAttribute('data-theme')`, or suppress icon until after mount with a `mounted` state.

**M-3: Swimmer schedule section grouping uses hardcoded date strings**
- File: `admin/app/(swimmer)/swim/schedule/page.tsx`
- Section keys like `['Mon, Mar 17', 'Tue, Mar 18', ...]` are hardcoded. Wrong groupings for any real event data with different dates.

**M-4: `daysUntilDeadline` is a hardcoded static field**
- File: `admin/app/(portal)/portal/meets/[id]/page.tsx`
- `daysUntilDeadline: 2` on the mock meet. Should be computed: `Math.ceil((new Date(deadline) - new Date()) / 86400000)`.

**M-5: Portal calendar event colors hardcoded to light-mode hex values**
- File: `admin/app/(portal)/portal/calendar/page.tsx`
- `EVENT_STYLE` map uses light-mode hex colors. Dark mode users will get unreadable light backgrounds on event chips.

---

### LOW Issues

**L-1: Goal time input in swimmer performance has no validation**
- File: `admin/app/(swimmer)/swim/performance/page.tsx`
- `parseFloat` on a non-numeric string silently produces `NaN`, shown in UI as "NaN" with no error.
- Fix: Validate format (e.g., `MM:SS.ss`) before committing; show inline error if invalid.

**L-2: Mock meet event dates have wrong weekday labels**
- File: `admin/app/(portal)/portal/meets/[id]/page.tsx`
- `ev-008` and `ev-009` both claim March 25 but one shows "Tue" and the other "Wed".

---

## Task Completion Assessment

| # | Task | Status |
|---|------|--------|
| 1 | Split meet sessions into sections | PARTIAL — section grouping UI exists in detail view; no section config in creation wizard |
| 2 | Swimmer self-signup flag | PARTIAL — `swimmerSelfSignup: true` in mock data only; no Settings toggle; no distinct flow |
| 3 | Swimmers view entries | COMPLETE |
| 4 | Upload meet descriptions/PDFs/images | PARTIAL — `AttachmentsTab` UI shell exists; upload button is a stub with no file input or API |
| 5 | Practice scheduler | PARTIAL — calendar and list views exist; create/edit modals discard data (same pattern as C-2/C-3) |
| 6 | Autopay tab/feature | **MISSING** — zero implementation; no page, tab, component, or stub exists anywhere |
| 7 | Parent calendar bigger | COMPLETE |
| 8 | Performance visualization | COMPLETE |
| 9 | Admin/coach override entries post-commitment | PARTIAL — edit modal and OVERRIDE badge exist; add entry modal is a stub; no audit logging |
| 10 | Consistent admin UI | COMPLETE |
| 11 | Light/dark mode | COMPLETE (minor flash bug, M-2) |
| 12 | Actual login page | COMPLETE |
| 13 | Advanced progression/customization | COMPLETE |
| 14 | Commitments/entries in sidebar | **BROKEN** — pages exist but `NAV_ITEMS` has no entries; pages are unreachable |
| 15 | Signup/register screen | COMPLETE |
| 16 | 20E fully | COMPLETE — all 20E tasks implemented in swimmer portal |

---

## How to Fix (Priority Order)

1. **Fix Task #14 first** — Add to `NAV_ITEMS` in `admin/lib/roles.ts`:
   ```ts
   { label: 'Commitments', path: '/commitments', icon: 'ClipboardList', section: 'Meets', roles: ['admin','coach'] },
   { label: 'Entries', path: '/entries', icon: 'ListChecks', section: 'Meets', roles: ['admin','coach'] },
   ```
   Then add `'Performance'` and `'Compliance'` to `SECTION_ORDER` in `Sidebar.tsx`.

2. **Fix C-1** — Replace hardcoded today date in `practices/page.tsx`.

3. **Fix C-4** — Correct `handleReject` to transition state instead of delete; add `'rejected'` to `EntryStatus`.

4. **Fix H-1** — Wire the "New Meet" button to `/meets/new`.

5. **Fix C-2 and C-3** — Modal save handlers must actually collect and persist form data. For now, at minimum update local state; eventually wire to API.

6. **Implement Task #6 (Autopay)** — Create `admin/app/(admin)/billing/autopay/page.tsx` or add an Autopay tab in `admin/app/(admin)/billing/page.tsx`. Minimum: household autopay enrollment toggle, saved payment method display, next scheduled charge date, and opt-out flow.

7. **Fix M-1** — Apply `course` filter in `performance/visualization/page.tsx`.

8. **Fix H-4** — Move auth check before feature flag check in `SwimmerShell.tsx`.

9. **Fix M-2** — Eliminate ThemeToggle flash.

10. **Fix H-5** — Update mock meet dates to 2026.

---

## What Counts as Passing

- All 4 conditions must be **well-met**, not barely met.
- **Condition 1:** Zero CRITICAL bugs. No modal that silently discards data. No state machine violations (delete vs. transition). No hardcoded dates relative to "today."
- **Condition 2:** No unused state controls. No un-memoized column arrays rebuilt on every render. No hardcoded light-mode hex in dark-mode-capable components.
- **Condition 3:** Auth gate must execute before feature flag check. No bypasses.
- **Condition 4:** Every one of the 16 listed tasks must have a working, reachable implementation. Task #6 must exist. Task #14 must be navigable. All "partial" tasks must be either fully implemented or formally deferred with documentation.

---

## Master File List — All Files Edited in This Step

### Admin App Pages
- `admin/app/(admin)/meets/[id]/page.tsx`
- `admin/app/(admin)/meets/page.tsx`
- `admin/app/(admin)/meets/new/page.tsx`
- `admin/app/(admin)/commitments/page.tsx`
- `admin/app/(admin)/entries/page.tsx`
- `admin/app/(admin)/practices/page.tsx`
- `admin/app/(admin)/performance/visualization/page.tsx`
- `admin/app/(admin)/billing/page.tsx`
- `admin/app/(admin)/layout.tsx`

### Auth / Onboarding Pages
- `admin/app/login/page.tsx`
- `admin/app/register/page.tsx`
- `admin/app/logout/page.tsx`
- `admin/app/mfa/page.tsx`
- `admin/app/unauthorized/page.tsx`
- `admin/app/auth/callback/page.tsx`

### Portal Pages
- `admin/app/(portal)/portal/calendar/page.tsx`
- `admin/app/(portal)/portal/meets/[id]/page.tsx`
- `admin/app/(portal)/portal/meets/page.tsx`
- `admin/app/(portal)/portal/dashboard/page.tsx`
- `admin/app/(portal)/layout.tsx`

### Swimmer Portal Pages
- `admin/app/(swimmer)/swim/page.tsx`
- `admin/app/(swimmer)/swim/performance/page.tsx`
- `admin/app/(swimmer)/swim/schedule/page.tsx`
- `admin/app/(swimmer)/swim/profile/page.tsx`
- `admin/app/(swimmer)/layout.tsx`

### Components
- `admin/components/layout/Sidebar.tsx`
- `admin/components/layout/Topbar.tsx`
- `admin/components/layout/AdminShell.tsx`
- `admin/components/swimmer/SwimmerNav.tsx`
- `admin/components/swimmer/SwimmerShell.tsx`
- `admin/components/portal/PortalNav.tsx`
- `admin/components/portal/PortalShell.tsx`

### Lib / Config
- `admin/lib/roles.ts`
- `admin/lib/theme.ts` (if created)
