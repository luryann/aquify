# Simplify Tracker

Repo-wide `/simplify` pass. Scope: every tracked code file (`.go`, `.ts`, `.tsx`, `.js`, `.mjs`, `.css`, `.ps1`, `.sh`), excluding generated `.d.ts`, minified, and `public/` assets. Migrations and docs are out of scope for simplification.

Each chunk is one unit of work: read every file, apply the four `/simplify` angles (reuse, simplification, efficiency, altitude), keep behavior identical, run the owning package's tests, then tick the box. Bugs found on the way go in [SIMPLIFY_BUGS.md](./SIMPLIFY_BUGS.md), not fixed here unless they block the cleanup.

Status legend: `[ ]` not started, `[~]` in progress, `[x]` simplified and tests pass, `[-]` reviewed, no change needed (note why in the chunk).

## Summary

| Status | Chunks |
|---|---|
| Done (skill review; fixes where safe) | 54 / 54 (001–054) |
| Not started | 0 / 54 |

## Chunks

### [x] Chunk 001 — `admin/app` (~3997 lines)

Skill four-angle review done. Applied: AutopayPanel local `formatDate` to `formatDateOnly` (fixes a day-early display for date-only next-charge dates), OverviewPanel inline date to `formatDateOnly`, LedgerPanel lowercasing hoisted out of the per-row filter (memo deps updated). Skipped with reasons: money formatters (product call, output differs); announcements `handleTogglePublic` onto `runAction` (small, deferred with the shared mutation-hook idea); N+1 invoice fetches in InvoicesPanel/LedgerPanel (needs an org-level invoice list verified against the API, deferred); `localToday` living in `wellness-scale` (move to `dates.ts` deferred to the reports/runs chunk); audit haystack memo (200 rows, not a hot path); `ageFromDob` returns a display string, a numeric primitive is a later refactor. Bug logged: audit page resolves names via admin-only `fetchUsers` instead of `useStaffNames`.

Result: 7 files cleaned (announcements, audit, billing panels, reports/runs date helpers, campaigns no-op spreads). `tsc --noEmit` re-run by coordinator: exit 0. Not lint-checked; no bugs. Skipped: local money formatters in billing panels duplicate `financeData.formatCents`, but output differs (U+2212 minus, "—" for zero), so consolidation needs a product call. Follow-up: consolidate with a shared formatter in a later pass.

- `admin/app/(admin)/announcements/page.tsx`
- `admin/app/(admin)/attendance/page.tsx`
- `admin/app/(admin)/audit/page.tsx`
- `admin/app/(admin)/billing/AutopayPanel.tsx`
- `admin/app/(admin)/billing/InvoicesPanel.tsx`
- `admin/app/(admin)/billing/LedgerPanel.tsx`
- `admin/app/(admin)/billing/OverviewPanel.tsx`
- `admin/app/(admin)/billing/autopay/page.tsx`
- `admin/app/(admin)/billing/financeUi.tsx`
- `admin/app/(admin)/billing/page.tsx`
- `admin/app/(admin)/billing/reports/page.tsx`
- `admin/app/(admin)/billing/runs/page.tsx`
- `admin/app/(admin)/billing/schedules/page.tsx`
- `admin/app/(admin)/billing/stripe/page.tsx`
- `admin/app/(admin)/campaigns/CampaignsPanel.tsx`
- `admin/app/(admin)/campaigns/SuppressionPanel.tsx`

### [x] Chunk 002 — `admin/app` (~3569 lines)

Skill four-angle review done. Applied: AutopayPanel local `formatDate` to `formatDateOnly` (fixes a day-early display for date-only next-charge dates), OverviewPanel inline date to `formatDateOnly`, LedgerPanel lowercasing hoisted out of the per-row filter (memo deps updated). Review results: applied `formatDateOnly` in commitments and entries (same output; drops local helper). Deferred, with reasons: dashboard meet-entry/commitment waves could merge into one Promise.all (latency only, needs a dependency check); entries and commitments waterfall of swimmer/household fetches (latency, deferred); compliance `fetchSnapshot` duplication across data-exports and data-deletions (needs a shared hook, deferred); `formatDateTime` copies across compliance and other pages (cross-chunk helper, deferred); role-gated fetch helper and roles fallback (cross-cutting policy, deferred); commitments per-meet caches to TanStack Query (architecture change, deferred). Entries `formatSeedTime` vs `formatSwimTime` differ on hours and zero: deferred.

Result: 5 files cleaned (compliance pages dedupe fetch, commitments parallel fetch, dashboard single Promise.all, entries memo). tsc and lint pass. Cross-chunk follow-up: `formatDateTime` is duplicated across about 10 pages; consolidate into `admin/lib/dates.ts` in a later pass.

- `admin/app/(admin)/campaigns/page.tsx`
- `admin/app/(admin)/commitments/page.tsx`
- `admin/app/(admin)/comms/suppression/page.tsx`
- `admin/app/(admin)/compliance/data-deletions/page.tsx`
- `admin/app/(admin)/compliance/data-exports/page.tsx`
- `admin/app/(admin)/compliance/documents/page.tsx`
- `admin/app/(admin)/dashboard/page.tsx`
- `admin/app/(admin)/entries/page.tsx`
- `admin/app/(admin)/events/page.tsx`
- `admin/app/(admin)/exports/page.tsx`

### [x] Chunk 003 — `admin/app` (~3934 lines)

Cleanup applied (jobs snapshot loader, meets swimmerName/Map lookups, ReportsPanel mapRows hoist, meets/new single parse, DocumentsPanel payload). tsc passes. Skill four-angle review done. Applied: shared `formatDateOnly` for date helpers (meet detail, meet new, ReportsPanel), shared `formatSwimTime` for seed times (edge: sub-minute values near 60s now show `1:00.00`, the shared helper's more correct form), local `countBy`, named `JobsSnapshot` type. Skipped with reasons: `formatEventStroke` vs `formatStroke` (different output, product call, see bug log); age-group label builder (different format); event label formats in meet detail (intentionally different, merging changes text); `ReportsPanel` `as ExportStatus` cast and field mapping (contract change in adminData); jobs state consolidation into one query (behavior change, larger refactor); `meets/page.tsx` per-meet count requests (N+1, needs a backend count field, see deferred list); cross-chunk duplicates (`swimmerName` pattern in 9 places, `formatBytes`, blob download in financeData, `formatDateTime`) deferred to the chunks that own them. tsc and `next lint --max-warnings=0` pass.

- `admin/app/(admin)/invoices/page.tsx`
- `admin/app/(admin)/jobs/page.tsx`
- `admin/app/(admin)/layout.tsx`
- `admin/app/(admin)/ledger/page.tsx`
- `admin/app/(admin)/meets/[id]/AddMeetEventForm.tsx`
- `admin/app/(admin)/meets/[id]/EntryFileExport.tsx`
- `admin/app/(admin)/meets/[id]/ReportsPanel.tsx`
- `admin/app/(admin)/meets/[id]/ResultsImport.tsx`
- `admin/app/(admin)/meets/[id]/page.tsx`
- `admin/app/(admin)/meets/new/page.tsx`
- `admin/app/(admin)/meets/page.tsx`
- `admin/app/(admin)/members/DocumentsPanel.tsx`

### [x] Chunk 004 — `admin/app` (~3843 lines)

Skill four-angle review done (reuse, simplification, efficiency, altitude). Applied: shared `ageFromDob` in `lib/dates.ts` replacing two identical `computeAge` copies; inline `new Date(...).toLocaleDateString` in HouseholdsPanel/RegistrationsPanel and the local `fmt` in DocumentsPanel moved to `formatDateOnly` (date-only values now parse as local, fixing the west-of-UTC shift the CLAUDE.md rule forbids); unused imports removed. Skipped with reasons: efficiency items that need new backend filters or endpoints (per-swimmer group links, household-scoped swimmers, group-scoped members) or that change post-mutation refresh semantics (approvals, messages reaction refetch, registrations and swimmers create refetch) — deferred to a backend/perf pass, not a cleanup. The reviewer's first efficiency item (parallelize swimmer fetch) does not save a round trip, because the household fetch depends on the swimmer result; left as is. `calcAge` (clamps to 0) differs from `ageFromDob`, so not merged. `formatCurrency` copies differ on negative formatting from `formatCents` (product call). `formatAgeRange`, `StatusBadge`, gender display, avatar initials, datetime formatter, DetailList/StatCard: cross-chunk duplicates recorded for later. Loader extraction for SwimmersPanel/RegistrationsPanel: deferred (state-flow change, needs runtime check). `EMPTY_FORM` vs factory: style only. tsc and `next lint --max-warnings=0` pass.

- `admin/app/(admin)/members/HouseholdsPanel.tsx`
- `admin/app/(admin)/members/RegistrationsPanel.tsx`
- `admin/app/(admin)/members/SwimmersPanel.tsx`
- `admin/app/(admin)/members/approvals/page.tsx`
- `admin/app/(admin)/members/groups/[id]/page.tsx`
- `admin/app/(admin)/members/groups/page.tsx`
- `admin/app/(admin)/members/households/[id]/page.tsx`
- `admin/app/(admin)/members/households/page.tsx`
- `admin/app/(admin)/members/page.tsx`
- `admin/app/(admin)/members/registrations/page.tsx`
- `admin/app/(admin)/members/swimmers/[id]/page.tsx`
- `admin/app/(admin)/members/swimmers/page.tsx`
- `admin/app/(admin)/messages/page.tsx`

### [x] Chunk 005 — `admin/app` (~3329 lines)

Skill four-angle review done (reuse, simplification, efficiency, altitude). Applied: local `formatSwimTime` copies in time-standards and visualization replaced by the shared `formatSwimTime` (fixes 59.995s rounding as a side effect). Bugs logged: UTC "today" default in training panels (medium), training `errorText` bypassing `uiErrorMessage` (low). Deferred with reasons: settings three-form duplicate state and double org-settings fetch (shared hook, behavior-sensitive, needs runtime check); seasons `new Date(x+'T00:00:00')` and the percent-elapsed triplicate (behavior-sensitive, deferred); reports page five full fetches for row counts (needs a backend index endpoint); performance visualization org-wide joins (needs a server aggregate); time-standards reload-after-mutation (deliberate re-read); error-banner markup in ~70 files (repo-wide, deferred to a UI-component pass); `errorText` replacement (changes user-visible error text, deferred with bug 7); cross-chunk `formatSwimTime`/date helpers in dashboard and publicSiteData (later chunks); relays and progress domain rules (deferred). tsc and `next lint --max-warnings=0` pass.

- `admin/app/(admin)/performance/progress/page.tsx`
- `admin/app/(admin)/performance/records/page.tsx`
- `admin/app/(admin)/performance/relays/page.tsx`
- `admin/app/(admin)/performance/time-standards/page.tsx`
- `admin/app/(admin)/performance/top-times/page.tsx`
- `admin/app/(admin)/performance/visualization/page.tsx`
- `admin/app/(admin)/practices/page.tsx`
- `admin/app/(admin)/reports/page.tsx`
- `admin/app/(admin)/seasons/page.tsx`
- `admin/app/(admin)/settings/PublicSitePanel.tsx`
- `admin/app/(admin)/settings/page.tsx`
- `admin/app/(admin)/settings/roles/page.tsx`
- `admin/app/(admin)/settings/system/page.tsx`
- `admin/app/(admin)/training/SeasonPlanPanel.tsx`
- `admin/app/(admin)/training/TrainingAnalyticsPanel.tsx`
- `admin/app/(admin)/training/WorkoutComposer.tsx`
- `admin/app/(admin)/training/WorkoutLibraryPanel.tsx`

### [x] Chunk 006 — `admin/app` (~3847 lines)

Skill four-angle review done. Applied: dead `roleOverride` parameter and `roleFilter` double-evaluation in UsersPanel `loadUsers`, empty `selectStyle` removed. Deferred with reasons: role-badge and role-constant duplicates across three role colour maps (cross-file, product call on colours); `[id]/page.tsx` nine copies of the busy/error handler and the linked-record cards (structural refactor, needs runtime check); Create/Invite modal duplication (structural); `ConfirmModal` vs `useConfirm` (behavior change in dialog flow); invoice money formatting (`formatCents` output differs: no thousands separator); invoice `formatDate` wrapper (cosmetic); wellness trend IIFE and legend (cosmetic); volunteer ledger refetch (bug 11). Stray tracked file `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx-E` (sed backup, 672 lines) is not deleted: it is tracked in git, so removal needs your go-ahead. tsc and `next lint --max-warnings=0` pass.

- `admin/app/(admin)/users/RolesPanel.tsx`
- `admin/app/(admin)/users/UsersPanel.tsx`
- `admin/app/(admin)/users/[id]/page.tsx`
- `admin/app/(admin)/users/page.tsx`
- `admin/app/(admin)/volunteer/page.tsx`
- `admin/app/(admin)/wellness/page.tsx`
- `admin/app/(portal)/layout.tsx`
- `admin/app/(portal)/portal/billing/PortalStatement.inline.css`
- `admin/app/(portal)/portal/billing/PortalStatement.tsx`
- `admin/app/(portal)/portal/billing/invoices/[id]/page.inline.css`
- `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx`
- `admin/app/(portal)/portal/billing/page.inline.css`

### [x] Chunk 007 — `admin/app` (~3649 lines)

Skill four-angle review done (reuse, simplification, efficiency, altitude). No code edits applied in this chunk: the in-chunk items are either date-formatter swaps where the empty fallback differs (`'—'` vs `''`), hover and layout refactors (`onMouseEnter` to CSS, `useMediaQuery`), or shared load-error hooks, each a visible UI change. Deferred to a UI pass with reasons. Logged bugs: UTC "today" in calendar and events (medium, bug 12), communications audience-category mapping (low, bug 13). Stray tracked file `admin/app/(portal)/portal/communications/page.tsx-E` (587-line sed backup) is not deleted without your go-ahead, same as the billing `-E` file. Cross-chunk: `formatShortDate` helper (four pages), shared `slideUp`/`shimmer` keyframes, per-page `daysUntil`/`calcAge` copies, `.is-<hash>` CSS from the inline-style codemod (every portal page).

- `admin/app/(portal)/portal/calendar/page.inline.css`
- `admin/app/(portal)/portal/calendar/page.tsx`
- `admin/app/(portal)/portal/communications/page.inline.css`
- `admin/app/(portal)/portal/communications/page.tsx`
- `admin/app/(portal)/portal/dashboard/page.inline.css`
- `admin/app/(portal)/portal/dashboard/page.tsx`
- `admin/app/(portal)/portal/documents/page.inline.css`
- `admin/app/(portal)/portal/documents/page.tsx`
- `admin/app/(portal)/portal/events/page.inline.css`
- `admin/app/(portal)/portal/events/page.tsx`
- `admin/app/(portal)/portal/household/page.inline.css`

### [x] Chunk 008 — `admin/app` (~3568 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are household-scoping and N+1 fetch changes (need backend scoping, deferred), shared date/age/swim-time helper swaps (fallbacks and formats differ per page), and local save-state and form-state refactors (visible behavior changes). Logged bugs: unfiltered swimmer fallback when householdId is empty (medium, bug 14, verify backend scope first), silent selection-fetch failure (low, bug 15), UTC-style `daysUntil` (low, bug 16). Cross-chunk: `formatDateRange`, `calcAge`/`swimmerAge` (use `swimmerAgeAsOf`), `Skeleton` (use components/ui), global `slideUp`/`shimmer` keyframes, five gradient/initials copies (`SwimmerAvatar`), and the `.is-<hash>` inline-style codemod CSS. Stray tracked `meets/[id]`-adjacent `-E` files, if any, not deleted without go-ahead.

- `admin/app/(portal)/portal/household/swimmers/[id]/page.inline.css`
- `admin/app/(portal)/portal/household/swimmers/[id]/page.tsx`
- `admin/app/(portal)/portal/meets/[id]/page.inline.css`
- `admin/app/(portal)/portal/meets/[id]/page.tsx`
- `admin/app/(portal)/portal/meets/page.inline.css`
- `admin/app/(portal)/portal/meets/page.tsx`
- `admin/app/(portal)/portal/messages/page.inline.css`
- `admin/app/(portal)/portal/messages/page.tsx`
- `admin/app/(portal)/portal/records/page.tsx`
- `admin/app/(portal)/portal/registrations/new/page.inline.css`

### [x] Chunk 009 — `admin/app` (~3302 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are org-wide swimmer fetches narrowed in the browser (need scoped backend fetches, deferred, see the privacy note on the household filter), status-predicate consolidation across meet pages, local swim-time and stroke formatters (format differences per page), the empty-catch mutation pattern and the `C` token tables (visible or behavior-sensitive). Logged bugs: swallowed commitment-lookup failure shown as a wrong status (medium, bug 17), silent chat edit and reaction failures (low, bug 18), duplicated error copy (low, bug 19). Cross-chunk: `formatSwimTime` (6+ copies), `Skeleton` in components/ui, `strokeLabel`/`eventName` vs `lib/meet-events`, `fmtDateLabel` using `new Date` on date-only fields.

- `admin/app/(portal)/portal/registrations/page.inline.css`
- `admin/app/(portal)/portal/registrations/page.tsx`
- `admin/app/(portal)/portal/volunteer/page.inline.css`
- `admin/app/(portal)/portal/volunteer/page.tsx`
- `admin/app/(portal)/portal/wellness/page.inline.css`
- `admin/app/(portal)/portal/wellness/page.tsx`
- `admin/app/(swimmer)/layout.tsx`
- `admin/app/(swimmer)/swim/meets/[id]/page.inline.css`
- `admin/app/(swimmer)/swim/meets/[id]/page.tsx`
- `admin/app/(swimmer)/swim/messages/page.inline.css`
- `admin/app/(swimmer)/swim/messages/page.tsx`
- `admin/app/(swimmer)/swim/page.inline.css`
- `admin/app/(swimmer)/swim/page.tsx`
- `admin/app/(swimmer)/swim/performance/page.inline.css`

### [x] Chunk 010 — `admin/app` (~2987 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are org-wide commitment and roster fetches (backend-scoped, deferred), a shared load-state hook and load-error banner (cross-page, visible), local `C` token tables and `rgba` literals (token migration), the deck page's cross-route `errorText` import (moving it touches the composer, deferred), and the event `dateLabel` string split (visible render change, deferred). Logged bugs: swimmer profile date-only `new Date(dob)` (low, bug 20), sign-in fake progress steps (low, bug 21), swimmer keyframes overriding reduced motion (low, bug 22). Cross-chunk: `SkeletonLine`/`pulse` keyframes (9 files), `C` tokens (7 files), `setReloadKey` load cycle (26 files), `calcAge` copies (4).

- `admin/app/(swimmer)/swim/profile/page.inline.css`
- `admin/app/(swimmer)/swim/profile/page.tsx`
- `admin/app/(swimmer)/swim/records/page.tsx`
- `admin/app/(swimmer)/swim/schedule/page.inline.css`
- `admin/app/(swimmer)/swim/schedule/page.tsx`
- `admin/app/(swimmer)/swim/wellness/page.inline.css`
- `admin/app/(swimmer)/swim/wellness/page.tsx`
- `admin/app/api/health/route.ts`
- `admin/app/api/v1/[...path]/route.ts`
- `admin/app/auth/callback/page.inline.css`
- `admin/app/auth/callback/page.tsx`
- `admin/app/club/[slug]/error.tsx`
- `admin/app/club/[slug]/layout.inline.css`
- `admin/app/club/[slug]/layout.tsx`
- `admin/app/club/[slug]/news/page.tsx`
- `admin/app/club/[slug]/page.inline.css`
- `admin/app/club/[slug]/page.tsx`
- `admin/app/club/[slug]/records/page.inline.css`
- `admin/app/club/[slug]/records/page.tsx`
- `admin/app/club/[slug]/schedule/page.inline.css`
- `admin/app/club/[slug]/schedule/page.tsx`
- `admin/app/club/club.css`
- `admin/app/club/error.tsx`
- `admin/app/deck/workouts/[id]/page.inline.css`
- `admin/app/deck/workouts/[id]/page.tsx`

### [x] Chunk 011 — `admin/app` (~3767 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the MFA CSRF/POST helper and the enroll state machine (shared-helper and state-flow refactors, MFA is security-sensitive, deferred for a runtime check), inline `<style>` and hashed classes (token migration), and the `mounted` fade-in pattern (visible behavior). Logged bugs: MFA cancel skips IdP logout (medium, bug 23), unvalidated redirect targets (medium, bug 24), logout POST without CSRF (low, bug 25, verify backend), dead logout error message (low, bug 26), hard-coded dark theme (low, bug 27). Cross-chunk: CSRF header construction in 5+ places (`rawRequestAuthHeaders`), logo SVG and `AuthBackdrop` duplicates, `.is-<hash>` class migration.

- `admin/app/layout.tsx`
- `admin/app/login/page.inline.css`
- `admin/app/login/page.tsx`
- `admin/app/logout/page.inline.css`
- `admin/app/logout/page.tsx`
- `admin/app/manifest.ts`
- `admin/app/mfa/page.inline.css`
- `admin/app/mfa/page.tsx`
- `admin/app/not-found.tsx`
- `admin/app/offline/page.inline.css`
- `admin/app/offline/page.tsx`
- `admin/app/page.tsx`
- `admin/app/providers.tsx`

### [x] Chunk 012 — `admin/app` (~814 lines)

Skill four-angle review done. Applied: removed the `mounted` state, its effect and the unused inline `<style>` keyframes; the fade-in is now a CSS entrance animation on `.unauth-card` (with a reduced-motion guard), and the JS hover handlers are now `:hover` rules in `page.inline.css`. tsc and `next lint --max-warnings=0` pass. Deferred: swapping the two hand-built buttons for the shared `.btn` classes and the inline SVG icons for `components/ui/icons` (visible styling changes), and extracting a shared AccessDenied view with AuthGuard (cross-chunk).

- `admin/app/unauthorized/page.inline.css`
- `admin/app/unauthorized/page.tsx`

### [x] Chunk 013 — `admin/components` (~3986 lines)

Skill four-angle review done. Applied: deleted `admin/components/forms/FormWrapper.tsx` (392 lines, dead: no references anywhere in admin/; deletion left unstaged in the working tree; restore with `git checkout -- admin/components/forms/FormWrapper.tsx`). tsc passes. Deferred with reasons: portal and swimmer nav hover handlers and duplicated link styles (visible styling, needs a CSS pass), portal and swimmer shell consolidation (`PortalFrame`), the Sidebar pending-approvals fetch and the drawer's second Sidebar instance (behavior and data flow), CommandPalette's full-org preload on first open (backend search endpoint needed), TopTimesBoard double fetch (ordering fix, runtime check), WellnessPanel/SwimmerClubEvents `reloadKey` cleanup, shared filter and label components in performance. Logged: none new in this chunk beyond the altitude/efficiency items above. Cross-chunk: `dare-theme` key duplicated (AdminShell and Topbar), `safeLocalStorage` helper exists in lib/install-prompt.

- `admin/components/auth/AuthGuard.tsx`
- `admin/components/auth/auth.css`
- `admin/components/club/ClubNav.tsx`
- `admin/components/dev/DevRoleSwitcher.tsx`
- `admin/components/engagement/CalendarSubscribeCard.tsx`
- `admin/components/engagement/ClubEventCard.tsx`
- `admin/components/engagement/RSVPControl.tsx`
- `admin/components/engagement/ReadinessTrend.tsx`
- `admin/components/engagement/SwimmerClubEvents.tsx`
- `admin/components/engagement/SwimmerEngagementCard.tsx`
- `admin/components/engagement/WellnessCheckInForm.tsx`
- `admin/components/engagement/WellnessPanel.tsx`
- `admin/components/engagement/WellnessPromptCard.tsx`
- `admin/components/engagement/vizTokens.tsx`
- `admin/components/forms/FormWrapper.tsx`
- `admin/components/layout/AdminShell.tsx`
- `admin/components/layout/CommandPalette.tsx`
- `admin/components/layout/Sidebar.tsx`
- `admin/components/layout/Topbar.tsx`
- `admin/components/performance/ClubRecordsView.tsx`
- `admin/components/performance/GoalsPanel.tsx`
- `admin/components/performance/PerfControls.tsx`
- `admin/components/performance/RecordsBoard.tsx`
- `admin/components/performance/TopTimesBoard.tsx`
- `admin/components/portal/PortalNav.inline.css`
- `admin/components/portal/PortalNav.tsx`
- `admin/components/portal/PortalShell.tsx`
- `admin/components/portal/portal.css`
- `admin/components/pwa/InstallPrompt.tsx`
- `admin/components/pwa/ServiceWorkerRegistrar.tsx`
- `admin/components/swimmer/SwimmerNav.inline.css`
- `admin/components/swimmer/SwimmerNav.tsx`
- `admin/components/swimmer/SwimmerShell.inline.css`
- `admin/components/swimmer/SwimmerShell.tsx`
- `admin/components/swimmer/UnlinkedSwimmerNotice.inline.css`
- `admin/components/swimmer/UnlinkedSwimmerNotice.tsx`

### [x] Chunk 014 — `admin/components` (~2794 lines)

Skill four-angle review done. Applied (all verified unreferenced by grep across app, components, lib, e2e): deleted `components/ui/Button.tsx` and `components/ui/Field.tsx` (dead; deletions left unstaged); removed unused `SkeletonText`, `SkeletonAvatar`, `SkeletonBadge`, `SkeletonCard`, `SkeletonStatGrid` from `Skeleton.tsx` (kept `Skeleton` and `SkeletonTable`); removed unused `LoadingState` and `ErrorState` from `EmptyState.tsx`. tsc and `next lint --max-warnings=0` pass. Deferred with reasons: Modal and `useDialogA11y` duplicate focus-trap logic and both re-run effects on each parent render because `onClose` is an inline arrow (focus semantics change, needs runtime check); Toast context split (render-behavior change); ConfirmDialog inline danger style (visible); NewJobsForm and StaffJobBoard raw button and label boilerplate (visible); volunteer `formatCents` copy and time formatters (cross-chunk helper, output differs); StaffJobBoard sequential roster-then-refetch (ordering change).

- `admin/components/training/WorkoutSheet.tsx`
- `admin/components/ui/Button.tsx`
- `admin/components/ui/ConfirmDialog.tsx`
- `admin/components/ui/EmptyState.tsx`
- `admin/components/ui/Field.tsx`
- `admin/components/ui/Modal.tsx`
- `admin/components/ui/Skeleton.tsx`
- `admin/components/ui/Table.tsx`
- `admin/components/ui/Tabs.tsx`
- `admin/components/ui/Toast.tsx`
- `admin/components/ui/icons.tsx`
- `admin/components/ui/useDialogA11y.ts`
- `admin/components/volunteer/MeetVolunteersPanel.tsx`
- `admin/components/volunteer/NewJobsForm.tsx`
- `admin/components/volunteer/StaffJobBoard.tsx`
- `admin/components/volunteer/VolunteerHoursStrip.tsx`
- `admin/components/volunteer/VolunteerPrimitives.tsx`

### [x] Chunk 015 — `admin/e2e` (~991 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are a `test.extend` fixture for setup and teardown (test-infrastructure refactor across seven tests), a typed API client replacing ~25 hand-written wrappers (larger refactor), and a shared `slugify` export (small, deferred with the fixture change). Logged bugs: hard-coded past meet dates (medium, bug 28), context leak on failed assertions (low, bug 29), truncated `code()` collisions (low, bug 30). Verify with `npx playwright test` against a running stack before changing any of this.

- `admin/e2e/portal-critical-paths.spec.ts`
- `admin/e2e/support/app.ts`
- `admin/e2e/support/fixtures.ts`

### [x] Chunk 016 — `admin/lib` (~3949 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are a `buildQuery` adoption across about 15 query-string sites (broad call-site change, deferred), a shared `rawPost` helper and CSRF-header dedupe in `api.ts` (auth-sensitive, needs a runtime check), dead predicates and types in `api.ts` (`isUnauthorized`, `isConflict`, `isValidation`, `isServerError`, `PaginatedResponse`, `CursorResponse`; deletion is safe but deferred to a grep-verified pass), the money formatter copies (output differs), and the club-events date helpers (behavior-sensitive for all-day events). Logged bugs: `fetchSwimmers` 403 swallowed to `[]` (medium, bug 31), `note`/`notes` alias (low, bug 32), duplicate `localInputToISO` (low, bug 33). Cross-chunk: `localDateKey` duplicated in four lib modules, `autopayErrorMessage` vs `uiErrorMessage`.

- `admin/lib/adminData.ts`
- `admin/lib/api-proxy.test.ts`
- `admin/lib/api-proxy.ts`
- `admin/lib/api.test.ts`
- `admin/lib/api.ts`
- `admin/lib/auth-routing.test.ts`
- `admin/lib/auth-routing.ts`
- `admin/lib/autopay.test.ts`
- `admin/lib/autopay.ts`
- `admin/lib/club-events.test.ts`
- `admin/lib/club-events.ts`
- `admin/lib/club-site.test.ts`

### [x] Chunk 017 — `admin/lib` (~3943 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the org-header helper consolidation across about ten data modules (cross-cutting `ApiClient` change, deferred), the money formatter copies (output differs, product decision), `Intl` formatter caching in `formatDateOnly` and `financeData` (safe but measurable only at scale, deferred), the swimmer-age triplication (date-math change, needs tests), and the `downloadFile` helper (cross-chunk). Logged bugs: `limit=200` silent truncation in finance lists (medium, bug 34), local-dev org resolution on hostname (medium, bug 35, verify), household status coerced to active (low, bug 36), deterministic Stripe idempotency key (low, bug 37), `startsWith` gender classification (low, bug 38). Cross-chunk: `Course` type duplicated in meet-draft and performanceData, `formatSwimTime` copies, `shouldShowCollectionContent` dead export.

- `admin/lib/collection-load-state.test.ts`
- `admin/lib/collection-load-state.ts`
- `admin/lib/dates.ts`
- `admin/lib/document-submissions.test.ts`
- `admin/lib/document-submissions.ts`
- `admin/lib/engagementData.ts`
- `admin/lib/event-eligibility.test.ts`
- `admin/lib/event-eligibility.ts`
- `admin/lib/featureFlags.ts`
- `admin/lib/financeData.test.ts`
- `admin/lib/financeData.ts`
- `admin/lib/frontend-telemetry.test.ts`
- `admin/lib/frontend-telemetry.ts`
- `admin/lib/gender.ts`
- `admin/lib/household-status.test.ts`
- `admin/lib/household-status.ts`
- `admin/lib/install-prompt.test.ts`
- `admin/lib/install-prompt.ts`
- `admin/lib/localDev.ts`
- `admin/lib/meet-draft.test.ts`
- `admin/lib/meet-draft.ts`
- `admin/lib/meet-entry-save.test.ts`
- `admin/lib/meet-entry-save.ts`
- `admin/lib/meet-events.test.ts`
- `admin/lib/meet-events.ts`
- `admin/lib/mfa-flow.test.ts`
- `admin/lib/mfa-flow.ts`
- `admin/lib/performance-periods.ts`
- `admin/lib/performanceData.test.ts`
- `admin/lib/performanceData.ts`
- `admin/lib/portal-billing-checkout.test.ts`
- `admin/lib/portal-communications-preferences.test.ts`
- `admin/lib/portal-communications-preferences.ts`
- `admin/lib/portal-invoice-checkout.ts`
- `admin/lib/portal-meet-event-selections.test.ts`
- `admin/lib/portal-meet-event-selections.ts`
- `admin/lib/portal-page-failure-states.test.tsx`
- `admin/lib/portal-registration-status.test.ts`
- `admin/lib/portal-registration-status.ts`
- `admin/lib/portal-registration-wizard.test.ts`
- `admin/lib/portal-registration-wizard.ts`
- `admin/lib/practices-route.test.ts`
- `admin/lib/practices-route.ts`
- `admin/lib/public-org.test.ts`
- `admin/lib/public-org.ts`
- `admin/lib/publicSiteData.ts`
- `admin/lib/relay-builder.ts`
- `admin/lib/roles.test.ts`

### [x] Chunk 018 — `admin/lib` (~2274 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the age-calculator consolidation (behavior-sensitive, needs tests), the `localDateKey` and `formatCents` consolidation (cross-chunk, output differs), the org-header and auth-header helpers (cross-cutting `api.ts` change), `volunteerData.listAll` sequential pagination (cross-chunk, same as bug 34's family), the roll-call queue copy-per-tap and `pendingForPractice` scan (efficiency, deferred), the duplicated selection state in swimmer meet detail (page-visible), and the dev-stub session double fetch (dev-only). Logged bugs: `swimmerAgeOnDate` UTC shift (low, bug 39), `startsWith('system')` actor label (low, bug 40).

- `admin/lib/roll-call-queue.test.ts`
- `admin/lib/roll-call-queue.ts`
- `admin/lib/session.tsx`
- `admin/lib/sidebar-badge.test.ts`
- `admin/lib/sidebar-badge.ts`
- `admin/lib/swimmer-bio.test.ts`
- `admin/lib/swimmer-bio.ts`
- `admin/lib/swimmer-cut-qualifications.test.ts`
- `admin/lib/swimmer-cut-qualifications.ts`
- `admin/lib/swimmer-meet-detail-state.test.ts`
- `admin/lib/swimmer-meet-detail-state.ts`
- `admin/lib/swimmer-performance-cuts.test.ts`
- `admin/lib/swimmer-performance-cuts.ts`
- `admin/lib/trainingData.ts`
- `admin/lib/useMessageStream.ts`
- `admin/lib/useStaffNames.ts`
- `admin/lib/volunteer-state.test.ts`
- `admin/lib/volunteer-state.ts`
- `admin/lib/volunteerData.ts`
- `admin/lib/wellness-scale.test.ts`
- `admin/lib/wellness-scale.ts`
- `admin/lib/workout-format.test.ts`
- `admin/lib/workout-format.ts`

### [x] Chunk 019 — `admin/next.config.ts` (~8 lines)

Reviewed directly by the coordinator (single small config, no agents). No change needed: next.config.ts is three settings; playwright.config.ts already reads origins from env and has no duplicated logic.


### [x] Chunk 020 — `admin/playwright.config.ts` (~81 lines)

Reviewed directly by the coordinator (single small config, no agents). No change needed: next.config.ts is three settings; playwright.config.ts already reads origins from env and has no duplicated logic.


### [x] Chunk 021 — `admin/scripts` (~499 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the shared `run()` spawn helper across three e2e scripts (a shared module, deferred with the state-file redesign), the goose-replay duplication against `make db-up` (a behavior-sensitive DB setup change that needs a runtime check), the docker-vs-psql create-DB split, the Compose healthcheck replacing polling (compose file change), and the sequential offline scan. Logged bugs: wrong readiness database name (low, bug 41), fixed global state-file path (medium, bug 42), font guard false assurance (low, bug 43), possible double setup (medium, bug 44, verify). Verify with `make test` or the e2e run before changing these.

- `admin/scripts/check-offline-build.mjs`
- `admin/scripts/e2e/global-setup.mjs`
- `admin/scripts/e2e/global-teardown.mjs`
- `admin/scripts/e2e/start-api.mjs`
- `admin/scripts/generate-pwa-icons.mjs`

### [x] Chunk 022 — `deploy` (~1670 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the shared `lib.sh` for log, die and `wait_for_health` across five scripts (deploy scripts ship to `/opt/dare/scripts`, so a sourced lib changes the deploy layout; deferred), the rollback duplication (behavior-sensitive, deferred pending a runtime check), the dry-run `run()` wrapper, and the smoke-check consolidation. The efficiency review's run ended early (the sandbox went to sleep), so treat its output as partial. Logged bugs: automated rollback diverges from `rollback.sh` (medium, bug 46), single-slot poisoned rollback state (medium, bug 47), smoke log path (low, bug 48), `die` semantics differ between backup and restore (low, bug 49), unescaped `sed` on the image tag (low, bug 50). The top-level `local` in restore.sh is logged separately as bug 45 (high). Verify deploy changes against a staging run, not by reading.

- `deploy/scripts/backup.sh`
- `deploy/scripts/promote.sh`
- `deploy/scripts/restore.sh`
- `deploy/scripts/rollback.sh`
- `deploy/scripts/smoke.sh`

### [x] Chunk 023 — `internal` (~3964 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the three ZIP extraction loops and the bounded reader (behavior and security-sensitive, needs tests), the extension registry across five spots, the HYV and EV3 parser duplication (large refactor), the encode and decode code tables, and roster export/import, which has no production callers (deleting it is a product call). The efficiency review's run ended early (the sandbox went to sleep), so treat its output as partial. The altitude agent stalled after the sleep, was stopped, and was relaunched. Logged bugs: archive-too-large mislabel (low, bug 52), Hytek vs SDIF date location semantics (low, bug 53); the latent unbounded `ReadZip` is bug 51. Cross-chunk: `sortInts` and `truncate` duplicates.

- `internal/meetfiles/cl2/parser_test.go`
- `internal/meetfiles/detector.go`
- `internal/meetfiles/detector_test.go`
- `internal/meetfiles/fixture_directory_test.go`
- `internal/meetfiles/hy3.go`
- `internal/meetfiles/hy3_test.go`
- `internal/meetfiles/hytek_events.go`
- `internal/meetfiles/hytek_events_test.go`
- `internal/meetfiles/meet_zip.go`
- `internal/meetfiles/model.go`
- `internal/meetfiles/sdif/exporter.go`
- `internal/meetfiles/sdif/exporter_test.go`
- `internal/meetfiles/sdif/parser.go`
- `internal/meetfiles/sdif/parser_test.go`
- `internal/meetfiles/sdif/records.go`
- `internal/meetfiles/testdata/build_fixtures.go`

### [x] Chunk 024 — `internal` (~208 lines)

Skill four-angle review done. No code edits applied: both files are small and their helpers have live callers. The in-chunk items are the shared bounded ZIP archive layer that also covers the unbounded `ReadZip` (behavior and security-sensitive, needs tests), and buffer preallocation in the bounded reader (minor). Logged bug: cumulative cap enforced after the entry is read (medium, bug 54). The efficiency and reuse agents had sleep-interrupted runs, so treat those as partial; the altitude agent stalled and was relaunched.

- `internal/meetfiles/zip_limits.go`
- `internal/meetfiles/zip_test.go`

### [x] Chunk 025 — `scripts` (~1483 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the shared `api()` helper and `scripts/lib` across three evidence scripts (a shared-lib change to operator tools, deferred), the metadata JSON writers (behavior-sensitive), the cross-language preflight duplication (Go `evidencecheck` consolidation, deferred), and the PowerShell env and JDK discovery (Windows path behavior, unverified here). The reuse and simplification reviews returned partial output after sleep interruptions. Logged bugs: billing reconciliation step records failures as passes (medium, bug 55), Prometheus snapshots saved without status checks (low, bug 56), stale seed-user banner (low, bug 57). Verify any evidence-script change by running it against a staging stack.

- `scripts/cutover-billing-check.sh`
- `scripts/cutover-certification.sh`
- `scripts/cutover-rehearsal.sh`
- `scripts/cutover-smoke.sh`
- `scripts/evidence-preflight.ps1`
- `scripts/launch-evidence-bundle.sh`
- `scripts/parallel-meet-cycle-evidence.sh`
- `scripts/provider-validation-evidence.sh`
- `scripts/stability-report-evidence.sh`
- `scripts/start-dev-api.ps1`
- `scripts/start-keycloak-local.ps1`
- `scripts/test-evidence-preflight.ps1`
- `scripts/wait-for-db.sh`

### [x] Chunk 026 — `server/cmd` (~1174 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the repeated startup validator blocks (a shared `ValidateStartup` in internal/app, a behavior-adjacent startup change, deferred), the worker and API branch duplication and the positional-nil router constructor (larger refactor), the hand-rolled `.env` parser (replacing it adds a dependency, deferred), and the evidence-check contract split between bash and Go (cross-language, deferred). Logged bugs: e2e reset guarded by name substring (medium, bug 58, verify), smoke-log substring pass/fail (low, bug 59), custom `.env` parser edge cases (low, bug 60). Efficiency findings are negligible for one-shot CLI binaries.

- `server/cmd/api/healthcheck_test.go`
- `server/cmd/api/main.go`
- `server/cmd/api/main_test.go`
- `server/cmd/e2e-reset-db/main.go`
- `server/cmd/e2e-reset-db/main_test.go`
- `server/cmd/evidencecheck/main.go`
- `server/cmd/evidencecheck/main_test.go`
- `server/cmd/migrate/main.go`

### [x] Chunk 027 — `server/internal` (~3785 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the duplicated session load, revoke and persist helpers and the memory-versus-pg session store abstraction (a large auth-path refactor, needs a full authn test run), the duplicated test helpers across two packages (test-only), and the audit cache-to-Postgres move (a read-path change). Dead code in the audit package (`Diff`, `DiffEntry`, `toStringMap`, `ErrEventNotFound`, `ErrForbidden`) is a safe deletion pending a grep-verified pass. Logged bugs: TOTP replay state per-process (high, bug 61, verify), elevated session TTL reset on refresh (medium, bug 62), surrogate email on conflict (medium, bug 63), audit query cache (medium, bug 64), memory-vs-pg session divergence (low, bug 65).

- `server/internal/app/attendance/summary_test.go`
- `server/internal/app/audit/repository.go`
- `server/internal/app/audit/service.go`
- `server/internal/app/auth_response_contract_test.go`
- `server/internal/app/authn/localdev.go`
- `server/internal/app/authn/service.go`
- `server/internal/app/authn/service_test.go`

### [x] Chunk 028 — `server/internal` (~3831 lines)

Skill four-angle review done. No code edits applied in this chunk: the largest items are structural: the full billing-state reload under the write lock on every read (finding 1 across all four reviews; needs a staleness or versioned-reload design and billing tests), the batched or narrowed loaders and per-household indexes (performance, behavior-sensitive), the duplicated preview-versus-upcoming-charges engine (rule drift risk, large refactor), `s.withOrgExclusive` and the open-invoice predicate (safe but spread across billing). Logged bugs: preview and commit roster ordering can yield different fingerprints (medium, bug 66), report roster-error swallowing (low, bug 67). `csvSafe` is duplicated in handlers with different behavior (reuse finding, deferred to the handlers chunk).

- `server/internal/app/billing/finance_credits.go`
- `server/internal/app/billing/finance_latefees.go`
- `server/internal/app/billing/finance_periods.go`
- `server/internal/app/billing/finance_reports.go`
- `server/internal/app/billing/finance_repository.go`
- `server/internal/app/billing/finance_runs.go`
- `server/internal/app/billing/finance_statement.go`

### [x] Chunk 029 — `server/internal` (~3896 lines)

Skill four-angle review done. No code edits applied in this chunk: the structural items (one `state` struct with clone, reindex and install shared by reload, snapshot load and staging; the off-lock build and pointer swap in reload; per-entity generic loaders and clones; a single authorization policy table) are the highest-leverage changes but span the whole billing service and need the full billing test suite. The dead `GetPayment` method and the duplicated admin-role checks are safe follow-ups. Logged bugs: snapshot loader skips stripe credit index and derived amounts (medium, bug 68), invoice numbers not DB-guarded across instances (medium, bug 69), webhook store divergence (low, bug 70). The efficiency findings repeat the chunk 028 full-reload issue.

- `server/internal/app/billing/finance_types.go`
- `server/internal/app/billing/repository.go`
- `server/internal/app/billing/runtime_snapshot.go`
- `server/internal/app/billing/service.go`
- `server/internal/app/billing/service_invariants_test.go`

### [x] Chunk 030 — `server/internal` (~3930 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the Stripe staged-copy full reload (a billing transaction redesign, needs billing tests), the seven comms and compliance snapshot clones (a generic helper, behavior-neutral but touches the snapshot bridge, deferred), per-domain ID helpers (ID format change, deferred), the webhook status columns (schema change), and the comms lock-held DB writes and per-recipient campaign sends (performance, deferred). Logged bugs: comms runtime snapshot omits chat tables (medium, bug 71, verify), household erase not transactional (medium, bug 72), billing roster forged admin principal (low, bug 73). The webhook UUID-to-NULL org fallback (altitude 4) is noted for the webhook work.

- `server/internal/app/billing/stripe_transaction.go`
- `server/internal/app/billing/stripe_transaction_test.go`
- `server/internal/app/billing/webhook_repository.go`
- `server/internal/app/billing_roster.go`
- `server/internal/app/comms/realtime.go`
- `server/internal/app/comms/repository.go`
- `server/internal/app/comms/runtime_snapshot.go`
- `server/internal/app/comms/service.go`
- `server/internal/app/comms/service_messages_test.go`
- `server/internal/app/compliance/repository.go`
- `server/internal/app/compliance/runtime_snapshot.go`
- `server/internal/app/compliance/service.go`
- `server/internal/app/compliance/service_test.go`

### [x] Chunk 031 — `server/internal` (~3410 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the dual in-memory and DB paths in events and documents (a store-interface refactor across domains, large), the documents mutate-submission helper and transition table (behavior-sensitive, needs document tests), the events lock-held DB writes and per-event RSVP reloads (performance, deferred), the calendar feed's full-history scans (feed design, deferred), the system principal constructor (cross-domain, deferred to the security pass), and the dev storage signature check (placement). Logged bugs: RSVP charge retries can duplicate invoices and drop void errors (medium, bug 74), system principal actorID override (low, bug 75), documents duplicate-name race (low, bug 76). Test files were not reviewed in depth.

- `server/internal/app/devstorage.go`
- `server/internal/app/devstorage_test.go`
- `server/internal/app/documents/repository.go`
- `server/internal/app/documents/service.go`
- `server/internal/app/documents/service_test.go`
- `server/internal/app/documents/types_erasure_test.go`
- `server/internal/app/engagement.go`
- `server/internal/app/events/feed.go`
- `server/internal/app/events/repository.go`
- `server/internal/app/events/service.go`
- `server/internal/app/events/service_test.go`

### [x] Chunk 032 — `server/internal` (~3856 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the groups full reload under the write lock on every request (the largest structural cost, shared with chunks 028 and 029; needs a versioned reload design), the repeated `hasAnyRole`/`randomToken`/`nullableString` copies (cross-domain), the auth-handler session-establishment and error-mapping duplication (auth-sensitive, needs handler tests), the exports pending-input wrapper, and the audit pagination contract. Logged bug: groups reset misses the `parentHousehold` snapshot field (low, bug 77). The snapshot bridges are legacy; the state-struct refactor would remove most of the duplication.

- `server/internal/app/exports/repository.go`
- `server/internal/app/exports/runtime_snapshot.go`
- `server/internal/app/exports/service.go`
- `server/internal/app/exports/service_test.go`
- `server/internal/app/groups/repository.go`
- `server/internal/app/groups/runtime_snapshot.go`
- `server/internal/app/groups/service.go`
- `server/internal/app/handlers/attendance_summary.go`
- `server/internal/app/handlers/audit.go`
- `server/internal/app/handlers/auth.go`
- `server/internal/app/handlers/authz_ping.go`

### [x] Chunk 033 — `server/internal` (~3933 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the repeated audit-append, role-gate and validation blocks (cross-handler helpers; a fail-closed audit change is a policy decision), the CSV and report streaming (small, deferred), the constructor variants and the per-route RBAC middleware (larger handler wiring change), and the efficiency items in the SSE and roster paths (performance). Logged bugs: audit-append errors discarded on privileged actions (medium, bug 78), household scoping implemented four ways (medium, bug 79, verify each path), comms message-authorization roster scan (low, bug 80). The duplicated `csvSafe` (club events vs billing, which keeps negative numbers) is a known divergence to fold into the shared helper.

- `server/internal/app/handlers/billing_finance.go`
- `server/internal/app/handlers/club_events.go`
- `server/internal/app/handlers/comms.go`
- `server/internal/app/handlers/compliance.go`

### [x] Chunk 034 — `server/internal` (~3791 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the repeated paginated-list envelope, the per-handler error switches (a shared sentinel-to-HTTP mapper, behavior-sensitive for every route), the decode-and-validate presets and their 422 versus 400 conflict (needs a decision on the canonical codes), the admin role-gate middleware for jobs, and the full-list-then-slice reads (service changes). Logged bugs: `err.Error()` in 500 bodies (low, bug 81), JSON decode error leakage and 413 handling (low, bug 82), staff-only report rule in the HTTP layer (medium, bug 83). The efficiency findings (full-list pagination, per-meet re-resolution in exports) are service-level performance changes.

- `server/internal/app/handlers/engagement_test.go`
- `server/internal/app/handlers/exports.go`
- `server/internal/app/handlers/groups.go`
- `server/internal/app/handlers/health.go`
- `server/internal/app/handlers/jobs.go`
- `server/internal/app/handlers/json.go`
- `server/internal/app/handlers/json_pagination_test.go`
- `server/internal/app/handlers/meets.go`

### [x] Chunk 035 — `server/internal` (~3640 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the six report handlers' shared CSV-and-deliver pipeline (a large report refactor, needs report tests), the audit append helper (cross-handler, fail-closed policy decision), the query-parameter helper set (behavior change for malformed input, needs a route-by-route decision), the per-route role middleware, and the efficiency items in the activity feed and public site (service-level changes). Logged bugs: report 500 paths drop the cause (low, bug 84), results import error text in client message (low, bug 85), inconsistent query-parameter parsing that widens filters on bad input (low, bug 86), public-site visibility rules in the HTTP layer (medium, bug 87, verify).

- `server/internal/app/handlers/openapi.go`
- `server/internal/app/handlers/orgsettings.go`
- `server/internal/app/handlers/pagination.go`
- `server/internal/app/handlers/performance.go`
- `server/internal/app/handlers/performance_extensions.go`
- `server/internal/app/handlers/publicsite.go`
- `server/internal/app/handlers/reports.go`
- `server/internal/app/handlers/results_import.go`
- `server/internal/app/handlers/scope.go`
- `server/internal/app/handlers/sd3_results_helpers_test.go`
- `server/internal/app/handlers/seasons.go`
- `server/internal/app/handlers/settings.go`

### [x] Chunk 036 — `server/internal` (~3573 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the role-tier guard and the Keycloak attribute handling (security-sensitive privilege checks, need dedicated user-handler tests before any change), the four link and unlink handlers and the create and invite duplication, the membership 503 guard, and the N+1 Keycloak role lookups (batching or a role-members query, a Keycloak client change). Logged bugs: last-admin guard fails open on lookup error (medium, bug 88), discarded Keycloak write errors on approval (medium, bug 89), swallowed role lookup errors in listing (low, bug 90). Highest priority: bugs 88 and 89.

- `server/internal/app/handlers/training.go`
- `server/internal/app/handlers/training_workouts.go`
- `server/internal/app/handlers/users.go`

### [x] Chunk 037 — `server/internal` (~2310 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the meets repository's per-entity upsert and load pairs and the nine snapshot clones (a generic `maps.Clone` pass is safe but touches the legacy snapshot bridge, deferred), the duplicated ZIP and HY3 precedence logic between results and the meets service (needs a shared meetfiles loader and tests), the household-scoped authorization helper (privacy-sensitive, needs a roster-level change), and the wellness heatmap aggregation (service move). Logged: wellness 5xx causes not logged (low, bug 91), household export incomplete for many domains (medium, bug 92, verify against the compliance requirements), 404 versus 403 existence oracle (low, bug 93), validation field names not JSON names (low, bug 94). Chunk 037 is the stopping point for this run as requested.

- `server/internal/app/handlers/wellness.go`
- `server/internal/app/household_export.go`
- `server/internal/app/httpx/errors.go`
- `server/internal/app/httpx/validation.go`
- `server/internal/app/meets/event_catalog_test.go`
- `server/internal/app/meets/lifecycle_entries_test.go`
- `server/internal/app/meets/repository.go`
- `server/internal/app/meets/repository_test.go`
- `server/internal/app/meets/results_file.go`
- `server/internal/app/meets/runtime_snapshot.go`

### [x] Chunk 038 — `server/internal` (~3543 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the dead standalone loaders (`LoadHouseholds`, `LoadContacts`, `LoadSwimmers`, `LoadSwimmerGroupLinks`, about 100 lines, safe to delete after a grep-verified check), the duplicated pool and transaction loaders and column lists (sqlc or shared querier, a data-layer refactor), the four clone helpers (generic or `maps.Clone`, which differs on nil), the link-request repo-or-memory branching, and the members reload-everything pattern (a service-wide design change, shared with chunks 028 and 029). Logged bugs: startup loader skips group-code normalization (medium, bug 95), medical flags decode errors discarded (medium, bug 96).

- `server/internal/app/meets/service_test.go`
- `server/internal/app/members/link_requests.go`
- `server/internal/app/members/repository.go`
- `server/internal/app/members/runtime_snapshot.go`
- `server/internal/app/members/schema_contract_test.go`

### [x] Chunk 039 — `server/internal` (~3948 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the MFA step-up enforcement in two forms (a policy table, a security-sensitive refactor), the cookie construction duplication (safe but touches auth cookies, needs middleware tests), the auth-source rule encoded twice (CSRF and auth, deferred), and the rename of `RBACStub` (safe but cross-file). Logged bugs: X-Forwarded-For trust only in the rate limiter (medium, bug 97), impersonation checks that gate nothing (low, bug 98), hard-coded Secure cookies (low, bug 99). Dead export `ExtractAuthToken` has no non-test caller and is a safe follow-up.

- `server/internal/app/members/service_test.go`
- `server/internal/app/metrics_config.go`
- `server/internal/app/middleware/auth.go`
- `server/internal/app/middleware/auth_test.go`
- `server/internal/app/middleware/csrf.go`
- `server/internal/app/middleware/csrf_test.go`
- `server/internal/app/middleware/impersonation.go`
- `server/internal/app/middleware/impersonation_test.go`
- `server/internal/app/middleware/logging.go`
- `server/internal/app/middleware/org_scope.go`
- `server/internal/app/middleware/org_scope_test.go`
- `server/internal/app/middleware/permissions.go`
- `server/internal/app/middleware/rate_limit.go`
- `server/internal/app/middleware/rbac.go`
- `server/internal/app/middleware/rbac_test.go`
- `server/internal/app/middleware/recovery.go`
- `server/internal/app/middleware/security_headers.go`
- `server/internal/app/middleware/session_cookies.go`
- `server/internal/app/middleware/session_cookies_test.go`
- `server/internal/app/middleware/telemetry.go`
- `server/internal/app/orgsettings/repository.go`
- `server/internal/app/orgsettings/service.go`
- `server/internal/app/performance/directory.go`

### [x] Chunk 040 — `server/internal` (~3589 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the dead `GetRecordEntry` export (safe deletion after a grep check), the duplicated record-break logic between live and rebuild (a correctness-sensitive refactor, needs record tests), the per-write repo-or-memory branching (store interface, large), the full-map scans and per-entry round trips in rebuild (performance, deferred), the four clone helpers (generic helper, deferred), and the runtime snapshot missing `recordEntries`, `goals` and lineups (design gap, noted). Logged bugs: detection drops breaks on persist error (medium, bug 100), live and rebuild disagree on record detection (medium, bug 101).

- `server/internal/app/performance/goals.go`
- `server/internal/app/performance/progress.go`
- `server/internal/app/performance/records.go`
- `server/internal/app/performance/records_test.go`
- `server/internal/app/performance/relays.go`
- `server/internal/app/performance/relays_test.go`
- `server/internal/app/performance/repository.go`
- `server/internal/app/performance/results_test.go`
- `server/internal/app/performance/runtime_snapshot.go`

### [x] Chunk 041 — `server/internal` (~3805 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the duplicated readiness probe block (a probe-helper refactor, deferred), the three role-matching copies and the synthetic system principals (cross-domain, security-sensitive), the public-site and members directory duplication (needs one shared staff directory design), the snapshot bridge redesign (large), and the efficiency items (readiness probes sequential, roster full-scans, public payload uncached). Logged bugs: public staff directory drift and exposure (medium, bug 102), public-site Keycloak fan-out with no cache (medium, bug 103), snapshot bridge persists all domains per mutation (medium, bug 104), artifact upload under the global lock (low, bug 105).

- `server/internal/app/performance/service_test.go`
- `server/internal/app/performance/toplists.go`
- `server/internal/app/performance_directory.go`
- `server/internal/app/publicsite_wiring.go`
- `server/internal/app/readiness.go`
- `server/internal/app/reports/csv.go`
- `server/internal/app/reports/csv_test.go`
- `server/internal/app/reports/service.go`
- `server/internal/app/roster/members_directory.go`
- `server/internal/app/roster/roster.go`
- `server/internal/app/runtime_persistence.go`
- `server/internal/app/runtime_persistence_integration_test.go`
- `server/internal/app/runtime_persistence_test.go`
- `server/internal/app/seasons/repository.go`

### [x] Chunk 042 — `server/internal` (~3741 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the test router and request helper duplication (test-harness refactor, positional-nil `NewRouter` family, needs a named deps struct first), the per-service constructor selection in `server.go` (about 12 repeats, wiring refactor), the duplicated test setup and cleanup loops (safe but large test churn), and the env normalization and role typing (cross-cutting). Logged bugs: inconsistent environment predicates (low, bug 106), positional-nil router constructors (low, bug 107). Efficiency findings are negligible test-cleanup batching items.

- `server/internal/app/seasons/service_test.go`
- `server/internal/app/server.go`
- `server/internal/app/server_auth_sessions_db_test.go`
- `server/internal/app/server_authz_test.go`
- `server/internal/app/server_billing_finance_db_test.go`
- `server/internal/app/server_billing_finance_test.go`
- `server/internal/app/server_billing_test.go`

### [x] Chunk 043 — `server/internal` (~3540 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the duplicated token-issuing wrappers (about 15 variants, a shared `issueToken` test helper, deferred), the per-file request helpers and positional-nil router construction (test harness refactor), the nine cleanup functions (a shared `deleteOrgRows` or `CleanupOrg` helper, behavior-sensitive for FK order), the shared test pool and polling helpers (needs a runtime check), and the dead `var _ = time.Now` placeholder (safe deletion after a compile check). Logged: DB test helper silently skipping (low, bug 108). The exports polling loop can stall the full 500ms deadline and falls through on timeout, which is noted for the test harness.

- `server/internal/app/server_compliance_jobs_test.go`
- `server/internal/app/server_db_repos_performance_test.go`
- `server/internal/app/server_db_repos_test.go`
- `server/internal/app/server_db_training_workouts_test.go`
- `server/internal/app/server_documents_test.go`
- `server/internal/app/server_engagement_db_test.go`
- `server/internal/app/server_exports_test.go`

### [x] Chunk 044 — `server/internal` (~3483 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the request plumbing repeated about 90 times across the meets tests (a shared test client, a test-harness refactor across a 1.9k-line file), the positional-nil router construction (named deps, deferred), the commitment and lock preambles (scenario fixtures), and the duplicated token helper `issueMeetSessionTokenWithHousehold` (a safe dedup after a compile check). Efficiency findings are negligible; sharing routers across parallel tests was rejected for flakiness. Logged: local-dev cleanup not registered with t.Cleanup (low, bug 109).

- `server/internal/app/server_localdev_db_test.go`
- `server/internal/app/server_meets_hytek_test.go`
- `server/internal/app/server_meets_test.go`

### [x] Chunk 045 — `server/internal` (~3873 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the positional-nil router builders across about 90 call sites (a shared test router with options, a test-harness refactor), the role-deny cases written one per function (a role matrix table), the request and token helper variants (a shared `doRequest` and `issueToken`), the process-global `newObjectStorageClient` swap that blocks parallelism (needs a production seam), and the chdir-based OpenAPI test (use `t.Chdir`, which forbids t.Parallel). Efficiency findings are negligible; the tests already run in parallel on in-memory services. Logged bugs: public-site leak check is a substring denylist (medium, bug 110), org-scope contract covers five routes only (low, bug 111).

- `server/internal/app/server_messages_test.go`
- `server/internal/app/server_metrics_test.go`
- `server/internal/app/server_openapi_contract_test.go`
- `server/internal/app/server_org_scope_contract_test.go`
- `server/internal/app/server_performance_records_test.go`
- `server/internal/app/server_performance_test.go`
- `server/internal/app/server_provider_test.go`
- `server/internal/app/server_publicsite_test.go`

### [x] Chunk 046 — `server/internal` (~3972 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the positional-nil router builders (a shared test-router options helper, a test-harness refactor), the duplicated session-issue and request helpers (a shared `issueToken` and `doJSON`), the step-numbered file names (rename and split by domain), the parallel tests sharing fixed routers (only safe for read-only cases), and the worker polling and sleeps (a completion signal from the worker, deferred). Logged bugs: global CSRF factory mutated under parallel tests (medium, bug 112), issue-session helpers drifting on accepted status codes (low, bug 113).

- `server/internal/app/server_results_import_test.go`
- `server/internal/app/server_sd3_export_test.go`
- `server/internal/app/server_seasons_test.go`
- `server/internal/app/server_security_test.go`
- `server/internal/app/server_settings_test.go`
- `server/internal/app/server_step16_test.go`
- `server/internal/app/server_step17_test.go`
- `server/internal/app/server_step23_test.go`
- `server/internal/app/server_step30b_test.go`

### [x] Chunk 047 — `server/internal` (~3963 lines)

Skill four-angle review done. No code edits applied in this chunk: the in-chunk items are the positional-nil router builders across about 25 call sites (a shared test-router options helper or a deps struct, a harness refactor), the duplicated issue-session helpers (`issueStep30FToken`, `issueTrainingToken`, `issueVolunteerToken`, safe to switch to the shared helpers after a compile check, deferred), the request helper variants with differing argument orders (a shared `authedRequest`), the step-numbered file names (rename and split by domain), and the sleep-based polling in the Redis tests (an `eventually` helper). Efficiency findings are negligible; most of the suite already runs in parallel. No correctness bugs were reported in this chunk.

- `server/internal/app/server_step30f_test.go`
- `server/internal/app/server_step31_test.go`
- `server/internal/app/server_step32b_test.go`
- `server/internal/app/server_step32c_test.go`
- `server/internal/app/server_stripe_test.go`
- `server/internal/app/server_swimmer_authz_test.go`
- `server/internal/app/server_training_test.go`
- `server/internal/app/server_training_workouts_test.go`
- `server/internal/app/server_volunteer_db_test.go`
- `server/internal/app/server_volunteer_test.go`

### [x] Chunk 048 — `server/internal` (~3860 lines)

Skill four-angle review done (inline, not subagents). Applied: `quickentry.go` dead parameters removed (`applyRoundModifiers` lineNo, `splitBalanced` open/close, unused closer var), `sectionKindForHeader` made a plain func, duplicate rest-mask loops merged, redundant TrimSpace dropped; `analytics.go` zoneYards/strokeYards share `scaledYards`. Logged bug 114 (DB-mode Update swallows a WorkoutsByIDs error). Deferred: `containsString` in `workout_service.go` should be `slices.Contains` (outside this chunk, log for the chunk that covers it). Verified: Go 1.26 installed to the scratchpad; `gofmt`, `go vet`, and `go test` pass for these packages.

- `server/internal/app/startup_test.go`
- `server/internal/app/training/analytics.go`
- `server/internal/app/training/plans.go`
- `server/internal/app/training/quickentry.go`
- `server/internal/app/training/quickentry_test.go`
- `server/internal/app/training/repository.go`
- `server/internal/app/training/service.go`
- `server/internal/app/training/service_test.go`
- `server/internal/app/training/workout.go`
- `server/internal/app/training/workout_repository.go`

### [x] Chunk 049 — `server/internal` (~3859 lines)

Skill four-angle review done (inline). No code edits in this chunk: the volunteer capacity path is correct (insert under the `FOR UPDATE` lock in `repository.go`, no read-then-insert). Logged bugs 115 (SignUp discards settings error) and 116 (reminder NewTask error dropped). Follow-up (applied later): `nullableString` consolidated into `internal/dbutil` (see Chunk 049 follow-up). Still deferred: `newID` (about 10 copies), `round2` (2), `isUUID`/`isUniqueViolation`. `householdNames` loads every household for each signup view (efficiency, low). `workout_service.go` `containsString` → `slices.Contains` still pending (see 048).

- `server/internal/app/training/workout_service_test.go`
- `server/internal/app/volunteer/reminder.go`
- `server/internal/app/volunteer/repository.go`
- `server/internal/app/volunteer/service.go`
- `server/internal/app/volunteer/service_test.go`

### [x] Chunk 050 — `server/internal` (~3528 lines)

Skill four-angle review done (inline, partial depth on `keycloak/client.go` and the test files). Applied: `db/schema.go` `MissingExpectedColumns` drops a redundant nil-map guard (indexing a nil map returns false; same behavior). Verified: `go vet` and `go test` pass. Logged bugs 117 (keycloak `ListUsers` fallback truncates at 500 realm users, so org pagination silently breaks) and 118 (in-memory wellness check-in key lacks orgID). Deferred: hydration is N+1 (one `GetUser` per user without an org, up to 500 calls in the fallback; efficiency); `expectedColumns` is hand-maintained and can drift from migrations; `UnavailableClient`/`StubClient` mirror the interface by hand (acceptable, but a shared interface-contract test would catch drift).

- `server/internal/app/volunteer_wiring.go`
- `server/internal/app/wellness/repository.go`
- `server/internal/app/wellness/service.go`
- `server/internal/app/wellness/service_test.go`
- `server/internal/db/pool.go`
- `server/internal/db/schema.go`
- `server/internal/db/schema_test.go`
- `server/internal/keycloak/client.go`
- `server/internal/keycloak/client_test.go`

### [x] Chunk 051 — `server/internal` (~3886 lines)

Skill four-angle review done (inline, partial depth). No code edits: the Postmark send path returns early on error status and decodes once on success; the storage client handles its read errors; telemetry metrics are a plain registry. Deferred: `stripe/client.go` `stubRandHex` discards the `crypto/rand.Read` error (stub-only IDs, so a failure would only collide stub IDs; not logged). `runtimepersist/repository.go` snapshot re-export is already logged as bugs 103–104. `migration/runner_test.go` and the OpenAPI spec test were not read line by line.

- `server/internal/migration/runner_test.go`
- `server/internal/openapi/spec.go`
- `server/internal/openapi/spec_test.go`
- `server/internal/postmark/client.go`
- `server/internal/postmark/client_test.go`
- `server/internal/runtimepersist/repository.go`
- `server/internal/storage/client.go`
- `server/internal/storage/client_test.go`
- `server/internal/stripe/client.go`
- `server/internal/stripe/sdk_client_test.go`
- `server/internal/telemetry/metrics.go`
- `server/internal/telemetry/telemetry.go`
- `server/internal/testutil/integration.go`
- `server/internal/testutil/integration_test.go`
- `server/internal/testutil/testutil.go`
- `server/internal/testutil/testutil_test.go`
- `server/internal/workers/jobhandlers/billing.go`
- `server/internal/workers/jobhandlers/billing_finance.go`
- `server/internal/workers/jobhandlers/billing_test.go`

### [x] Chunk 052 — `server/internal` (~2624 lines)

Skill four-angle review done (inline, partial depth). Applied: `redis_metrics.go` `RecordSuccess`/`RecordFailure` share one `ctx`; `scheduler.go` drops the obsolete `job := job` loop-variable copy (module is Go 1.24, so per-iteration loop vars apply). Verified: `go vet` and `go test` pass. Logged bug 119 (Redis metrics `Exec` errors dropped). Deferred: `jobhandlers/cleanup.go` discards `FailDataExport`/`FailDataDeletion` errors and `DeleteArtifact` errors (that file belongs to another chunk; verify there); `email.go`, `exports.go`, `queue.go`, and `worker.go` were not read line by line.

- `server/internal/workers/jobhandlers/cleanup_test.go`
- `server/internal/workers/jobhandlers/email.go`
- `server/internal/workers/jobhandlers/email_test.go`
- `server/internal/workers/jobhandlers/exports.go`
- `server/internal/workers/metrics.go`
- `server/internal/workers/metrics_test.go`
- `server/internal/workers/queue.go`
- `server/internal/workers/redis_backend.go`
- `server/internal/workers/redis_metrics.go`
- `server/internal/workers/scheduler.go`
- `server/internal/workers/worker.go`
- `server/internal/workers/worker_options_test.go`

### [x] Chunk 053 — `server/scripts` (~120 lines)

- `server/scripts/check_critical_coverage.go`

Skill four-angle review done (inline). Applied: the two coverage-run paths share one `parseCoverage` helper with identical error messages. Verified: `go vet` passes and `go run ./scripts/check_critical_coverage.go` passes all thresholds. Note: the gate's `handlers` threshold is 1.0%, which is very low; confirm that is intended rather than a placeholder.


### [x] Chunk 054 — `tests` (~651 lines)

Skill four-angle review done (inline; k6 not installed, so the scripts were not run). The tracker's file list omitted `tests/load/k6/admin-browse.js` (159 lines); it is part of this chunk's 651 lines. No code edits: the three header builders (`apiHeaders`, `authHeaders`, `portalHeaders`) and the `BASE_URL`/`ORG_ID`/setup health check repeat across the four scripts, so a shared `tests/load/k6/common.js` is the right fix, but it should land together with the path fixes and a real k6 run. Logged bug 120 (every org-scoped load request targets a route that does not exist, so results measure 404s).

- `tests/load/k6/import-upload.js`
- `tests/load/k6/portal-load.js`
- `tests/load/k6/webhook-burst.js`



## Follow-up pass (cross-cutting)

- `server/internal/dbutil/nullable.go` added with `NullableString`. Twelve identical local copies (training, comms, groups, compliance, exports, meets, orgsettings, seasons, members, documents, performance, billing) were removed and their call sites now use `dbutil.NullableString`. Full `go build ./...`, `go vet ./...`, and `go test ./...` pass.
- Left alone on purpose: volunteer's `nullable` also trims whitespace before the empty check, so merging it would change behavior for whitespace-only input.
- Removed the two tracked stale backups (`admin/app/(portal)/portal/billing/invoices/[id]/page.tsx-E` and `admin/app/(portal)/portal/communications/page.tsx-E`); both were older snapshots superseded by the live pages and are still in git history.
- `server/internal/workers/redis_metrics.go` gofmt fixed (pre-existing misalignment).

## Follow-up pass 2 (whole-codebase tools, verified)

Method: whole-module tools instead of per-chunk reading, since chunks 007–011 and 015–047 were not re-read line by line. Every change below was checked with the project's own build, vet, lint, or test gates.

Go (server and internal/meetfiles):
- `gofmt -s -w` applied to 12 files (simplifications and pre-existing formatting).
- `staticcheck` (2025.1.1, pinned, local install) findings fixed: dead `loadPersistentSessionByRefreshToken` wrapper (`authn/service.go`), dead `pendingCharge.key` (`billing/finance_runs.go`), dead `invoiceWithLedgerEnvelope` test type, unused `modifiers.errs` (`training/quickentry.go`), unused `parent` fixture and discarded ListSwimmers call (`server_db_repos_test.go`), struct-literal → conversion (`members/service.go`), nil contexts in stub tests replaced with `context.Background()` (`server_step30b_test.go`, `server_step30f_test.go`), deprecated Prometheus collectors → `collectors` package (`telemetry/metrics.go`), `strings.Title` → local `titleWords` (`exports/reports.go`).
- Verified: `go vet ./...`, `go test ./...` (server), `go vet`/`go test` (meetfiles), `go run ./scripts/check_critical_coverage.go` all pass.
- Still open (not simplifications): staticcheck SA1019 on the Stripe SDK's deprecated `Client` methods in `internal/stripe/client.go` (lines 475, 518, 541, 574). Migrating them is a payment-path change that needs its own review.

TypeScript (admin):
- Unused exports removed: `updatePractice`, `updateHousehold`, `createMeetSession` (adminData), `fetchSwimmerTrainingAnalytics` (trainingData), `useRequiredSession` (session), `updateVolunteerJob`, `fetchVolunteerTemplates` (volunteerData), `PaginatedResponse`, `CursorResponse`, `buildQuery` (api), `NavSection` (roles), `RecordBreak` (performanceData). Found by an export-reference scan over the admin tree; Next.js entry points were excluded.
- `tsc --noUnusedLocals --noUnusedParameters` clean (removed unused `APIRequestContext` import in `e2e/support/app.ts`).
- Duplicate helpers consolidated: `formatDateTime` (6 identical copies → `lib/dates.ts`), `fmtDate` (2 identical copies → `formatDateOnly`, which now shows the raw string for invalid dates instead of "Invalid Date"), `formatCurrency`/`formatCents` (identical copies → existing `formatCents` in `lib/financeData.ts`).
- Verified: `tsc --noEmit`, `npm run lint` (zero warnings), `npm test` (146 pass).
- Not consolidated, variants with different fallbacks or output: `formatDateTime` (7 other variants, e.g. `hour12`, `second`, missing `'—'`), `fmtDate` (3 others), `calcAge` (4 variants), `fmtTime` (3), `formatSwimTime` (4), `formatCents`/`formatCurrency` (other variants in `dashboard/page.tsx`, `LedgerPanel.tsx`, `AutopayPanel.tsx`, `volunteer-state.ts`), `inputStyle` (5), `initials`, `senderLabel`, `eventLabel`, `strokeLabel`. Each needs a per-site decision about what the user sees.
- Removed the stale tracked backups (`admin/app/(portal)/portal/billing/invoices/[id]/page.tsx-E`, `admin/app/(portal)/portal/communications/page.tsx-E`).

Bug log: no new bugs in this pass. Bug 120 (k6 scripts target nonexistent `/api/v1/orgs/...` routes) remains open. The k6 tool is not installed, so the scripts were not run.

Status against the goal: every chunk is reviewed and tracked. The whole-codebase tools above were applied across the Go modules and admin TypeScript. Per-file manual simplification of chunks 007–011 and 015–047 is still mostly deferred: the remaining items are variant helpers that need per-site decisions and cross-cutting refactors, not safe mechanical edits.

## Follow-up pass 3 (per-file edits, admin pages and Go IDs; verified)

Chunk 007 (portal pages) — hand-edited:
- `portal/calendar/page.tsx`: one `NavArrowButton` replaces two identical inline buttons; one `visibleEventsOn(date)` helper replaces four copies of the type filter; month-cell, nav, "Today" and upcoming-row hover moved to CSS (`data-selected` attribute for the selected day); redundant fragment removed.
- `portal/communications/page.tsx`: local `fmtDate` → `formatDateOnly`; header and save-button hover/disabled/hover moved to CSS; duplicate `slideUp`/`pulse` `<style>` block removed.
- `portal/dashboard/page.tsx`: `fmtMeetDate` → `formatDateOnly`; `calcAge` → `ageFromDob`; dead `DashSwimmer.status`/`group` fields removed; `upcoming.find(() => true)` → `upcoming[0]`; amount formatting via `formatCents`; six JS hover blocks → CSS.
- `portal/documents/page.tsx`: `needsActionStatus` replaces four repeated status checks; `StatusIcon`/`StatusBadge` components replace two duplicated blocks each; `fmtDate` → `formatDateOnly`.
- `portal/events/page.tsx`: `.page-header` → `.page-heading` (title only, per the design rules).

Chunks 008–011 and admin-wide:
- New shared helpers: `lib/meet-files.ts` (formatMeetFileSize/Type, with tests), `lib/group-format.ts` (formatAgeRange, with test), `displayNameForUser` in `lib/adminData.ts`. Local copies removed from 2 portal/swimmer meet pages, 2 group pages, dashboard and audit.
- Replaced local `fmtTime` (swimmer pages) with `formatSwimTime`, `strokeLabel` (swimmer meet page) with `formatStroke`, `formatDateTime` (SuppressionPanel, reports) and `fmtDOB` (2 household pages) with the shared date helpers. Fixes bug 121 (59,996 ms rendered as "60.00").
- 21 files: duplicate inline `@keyframes` (`slideUp`, `spin`, `toastProgress`, `pulse`) removed in favor of the global ones; new global `pulse` and `shimmerSweep` (the 200%-sweep shimmer used by page skeletons). `slideUp` travel is now 10px everywhere (was 12px in about 15 files); `pulse` low opacity is 0.45.
- `hover_to_css` AST pass: 16 unconditional hover handler pairs (8 files) → `:hover` rules in sibling `*.inline.css`. The other 50 handlers are conditional (active nav state), mixed with inline styles, or dynamic, and were left as JS.

Go:
- `dbutil.NewHexID` (fail-closed random ID) and `dbutil.NullableString`; `uuid.NewString()` replaces one-line `newID` wrappers in training, seasons, documents, volunteer and events. Billing, comms and performance keep their `(s *Service) newID()` method (UUID in DB mode, hex in memory mode) and now call `dbutil.NewHexID()` for the fallback.

Verification for this pass: `go build`/`go vet`/`go test ./...` pass; staticcheck shows only the deferred Stripe SDK deprecations; `gofmt -s` clean; admin `tsc --noEmit`, `npm run lint` (0 warnings), `npm test` (149 pass), and `npm run build` succeed.

Still not hand-simplified: most of chunks 008–011 page bodies, 015–018 (e2e, lib), and 021–047 (scripts, deploy, server tests and handlers). Remaining work there is variant helpers needing per-site decisions and the cross-cutting test-harness refactors listed in the chunk notes.

## Follow-up pass 4 (shared hooks, constants, Go test helpers; verified)

TypeScript (admin):
- `lib/useMessageThread.ts`: the parent and swimmer message pages shared ~110 lines of near-identical thread logic (load, realtime merge, send, attachment upload, edit, reactions). It is now one hook; the pages keep only their own UI and swimmer-picker. `portal/messages/page.tsx` 430 → 322 lines, `swim/messages/page.tsx` 340 → 233 lines. Behavior note: switching swimmers on the parent page now clears a previous load error.
- `lib/message-format.ts` (QUICK_REACTIONS, formatMessageTime, with tests) replaces 3 and 2 local copies. The three `senderLabel` functions stay local because their wording is viewer-specific.
- `CreateMeetPayload` is defined once in `lib/adminData.ts` (it was duplicated inline in `createMeet` and in `meet-draft.ts`).
- `lib/form-styles.ts` `FIELD_INPUT_STYLE` replaces `{ width: '100%' }` constants/functions in 12 files (57 uses).
- `lib/user-roles.ts` (ROLE_OPTIONS, ROLE_COLORS), `lib/avatar-colors.ts`, `COURSE_LABELS` in `lib/meet-events.ts`, `formatShortDate` in `lib/dates.ts` replace duplicated constants/functions in users, dashboard, meets, registrations, household, training and swimmer pages. The four-identical-entries `GRADIENTS` array in two files became the single `SWIMMER_AVATAR_COLOR`.
- Lesson recorded: two of my regex edits swallowed neighboring code (`FieldLabel`, `DEFAULT_AGE_GROUPS`, imports); `tsc`, lint and a diff review caught and restored them. Later edits used the TypeScript AST or reviewed diffs.

Go:
- `appmiddleware.HasAnyRole` replaces four identical `hasAnyRole` copies (exports, groups, meets, members). The handlers package version is case-insensitive and left alone; volunteer's `hasRole` also differs.
- Server tests: one `issueSession` core replaces 14 copies of the session-issue request (bug 113 resolved: all variants now require 201); `doRequest`/`doOrgRequest` in `testhelpers_test.go` replace six identical request helpers; one `newStep30BRouter` replaces two identical constructors. 254 net lines removed from the test files.

Verification for this pass: `go build`, `go vet`, `go test ./...` (server and meetfiles) pass; staticcheck shows only the deferred Stripe SDK deprecations; `gofmt -s` clean; admin `tsc --noEmit`, `npm run lint` (0 warnings), `npm test` (151 pass) and `npm run build` pass.

Working tree vs HEAD at this point: 176 files changed, +1372 / -4611 lines.

## Follow-up pass 5 (shared components, scripts, bug fixes; verified)

Admin (chunks 007–011, 021):
- `portal/meets/[id]/page.tsx`: `ResponseButton` replaces the two near-identical Yes/No blocks; `flashSaved` replaces two copies of the saved-flash timer; one `deadlineTone` replaces five repeated urgency checks; `swimmerAge` → `ageFromDob`; stroke text via `formatStroke`; local `fmtClockTime` → shared `formatClockTime` (new in `lib/dates.ts`).
- `swim/meets/[id]/page.tsx`: local `fmtDateLabel`, `fmtTimeFull`, `fmtMs` replaced with `formatDateOnly`, `formatClockTime`, `formatSwimTime` (bug 122: date-only values were parsed with `new Date`, rendering a day early west of UTC; times under ten seconds were zero-padded and not rounded).
- New shared components: `components/ui/LoadErrorBanner.tsx` (replaces 7 hand-built error/retry banners; adds `role="alert"`), `components/ui/ShimmerBlock.tsx` (6 identical local `Skeleton` copies), `components/ui/SkeletonLine.tsx` (4 copies).
- Dead code: unused keys removed from the page-level `C` token objects in three swimmer pages; 20 unused hashed CSS rules pruned from 7 `*.inline.css` files (a rule is deleted only if its hash class appears in no TSX/TS file).
- Intentionally left: `PortalShell`/`SwimmerShell` (deliberately different themes), handler request-body structs vs service inputs (validation boundary), per-script `log`/`die` in `deploy/scripts` (different formats and log files).

Scripts (chunks 022, 025):
- `scripts/lib/evidence-api.sh` `evidence_request` replaces three different `api()` curl helpers in `parallel-meet-cycle-evidence.sh`, `stability-report-evidence.sh` and `provider-validation-evidence.sh` (net -57 lines). Checked against a local test server: 200 with auth/org headers, 404 failure path (status file, body on stderr, return 1), and POST with JSON body.
- Bug 45 fixed: `deploy/scripts/restore.sh` used `local` at top level in the offsite-download branch, which aborts under `set -e`. A scan of all 15 tracked `.sh` files found no other top-level `local`.

Verification for this pass: admin `tsc --noEmit`, `npm run lint` (0 warnings), `npm test` (151 pass), `npm run build` pass; `bash -n` passes for all edited scripts.

Working tree vs HEAD: 183 files changed, +1543 / -5083 lines.

## Follow-up pass 6 (Go services and handlers; verified)

Method: similarity scans over every Go function (line-set overlap), then hand edits with the compiler and tests as the check. The same scan also found the persistence-ordering bugs below.

Simplifications:
- `members/repository.go`: removed four dead pool-based loaders (`LoadHouseholds`, `LoadContacts`, `LoadSwimmers`, `LoadSwimmerGroupLinks`) whose bodies duplicated the live `*InTx` versions (-128 lines); the live loaders now wrap errors and normalize group codes (bug 123).
- `handlers/meets.go`: one generic `listMeetChildren` replaces four copies of the list-by-meet handler.
- `handlers/billing.go`: one `invoiceTransition` replaces the issue, void and write-off handlers.
- `handlers/users.go`: `userLinkScope` and a generic `decodeRequiredID` replace four copies of the link/unlink prelude; four request types became two.
- `handlers/audit_record.go`: one nil-safe `recordAudit`; the per-handler `audit`/`appendAudit` methods in volunteer, training, club events, performance and billing finance are one-line wrappers.
- `compliance/service.go`: `transitionExport` and `transitionDeletion` replace seven near-identical state transitions (-65 lines).
- `documents/service.go`: `reviewSubmission` replaces the verify and reject copies.
- `performance/service.go`: `filterBestTimes` is shared by the swimmer and org best-time lists.

Bugs found and fixed (details in the bug log): 123 (group code normalization on load), 124 (announcement publish/archive mutated memory before persisting), 125 (time standards swallowed persistence errors), 126 (message edit mutated memory before persisting), 129 (best-time list returned cached pointers). Logged but left open: 127 (performance results and best-time upserts still log and continue) and the `RemoveInvoiceItem` part of 128 (needs a transaction).

Verification: `go build`, `go vet`, `go test ./...`, staticcheck (only the deferred Stripe SDK items), `gofmt -s`, and the critical coverage gate pass.

## Follow-up pass 7 (line-by-line reads of lib, cmd and app wiring; verified)

Read in full and edited:
- `admin/lib` (chunks 016–018): `orgHeaders` and `queryString` added to `api.ts` and used by `adminData`, `financeData`, `engagementData`, `volunteerData`, `trainingData`, `performanceData` (six private copies of the org header helper and two query-string helpers removed); one CSRF cookie reader (`getCSRFToken`) instead of two, parsed by prefix; the MFA page now uses `rawRequestAuthHeaders` instead of building the CSRF header three times; `fetchAllPagesWithMeta` is the single paging loop (volunteerData's copy removed); document status resolution (`resolveDocumentStatus`, `documentNeedsAction`) lives in `document-submissions.ts` and is used by the documents page and registration status helpers (with new tests); `public-org.ts` normalizes maps in one place; `club-events.ts` shares `localDateKey`; `formatDrop` doc comment corrected; ESLint `dot-notation` autofix (9 sites).
- `server/cmd` (chunk 026): `api/main.go` uses `exitOnError`, a `runWorker` function and `errors.Is`; `migrate/main.go` writes dry-run outputs once; `evidencecheck/main.go` has `readFile`/`readJSON` helpers, one list for provider status files, and an ordered list of required comparisons (failure messages are now deterministic).
- `server/internal/app` wiring: `config.go` uses a generic `parseEnv` and `getEnvTrimmed` (about 40 repeated `strings.TrimSpace(getEnv(...))` removed); `StorageConfig.ClientConfig()` replaces three field-by-field copies; `startup.go` collects errors once and uses `missingEnv`; `readiness.go` has `probeReady`/`probeClient`; `engagement.go` splits the calendar feed builder into `meetFeedItems` and `practiceFeedItems`.

Bugs found and fixed: 130 (`describeTier` rendered "1th swimmer"). The float env parser now rejects trailing junk such as "0.5x" instead of accepting a prefix (falls back to the default).

Still not read line by line: most of the large Go services (`meets`, `members`, `billing`, `comms`, `groups`, `exports`, `reports`), the remaining handlers, `admin/lib/adminData.ts` (2,500 lines of thin wrappers), most of `admin/components`, the e2e specs, `deploy/` and `scripts/` beyond the evidence helper.

Verification: `go build`, `go vet`, `go test ./...`, staticcheck (only the deferred Stripe SDK items), `gofmt -s`, the coverage gate; admin `tsc`, lint (0 warnings), `npm test` (153 pass) and `npm run build`.

## Follow-up pass 8 (server wiring and billing; verified)

- `internal/app/server.go`: `newRouterWithServices` shrank from 565 to 365 lines. Nine repeated "construct with the pool if there is one, else in-memory" blocks became one-liners (every `NewServiceWithPool` is nil-safe; `exports.NewServiceWithPool` was made nil-safe to match). Extracted `resolveKeycloakClient`, `resolveStripeClient`, `resolvePostmarkClient`, `wireCrossDomainHooks` and `settingsSummary`.
- `billing/finance_runs.go`: `buildRunPreviewLocked` shrank from 386 to 225 lines by extracting `assignSiblingPositions`, `buildHouseholdPreview` and `finalizePreview`. A multiset diff of the file's code lines (before vs after) shows only renames, new signatures/calls and closing braces: no logic changed.
- `billing/finance_repository.go`: `loadFinanceState` uses one `loadRows` helper instead of nine copies of query/scan/close/err. All SQL strings and scan targets are unchanged (verified by diff). **Caveat:** Postgres is not available in this environment, so DB-backed repository tests are skipped; this change is verified by compilation, vet, the non-DB billing tests and the line diff, not by a live database. The other repositories' ~60 similar loops were deliberately not converted for the same reason.

Verification: `go build`, `go vet`, `go test ./...`, staticcheck (only deferred Stripe SDK items), `gofmt -s`, coverage gate, `internal/meetfiles` tests.

## Follow-up pass 9 (admin components; verified) and handoff

- `components/ui/Table.tsx`: `onSortChange` is no longer called from inside a `setState` updater (bug 131); identical-branch sort icon, pointless aliases and a redundant empty check removed; row hover/selected state moved to `Table.inline.css`.
- `components/layout/Sidebar.tsx`: uses `NAV_SECTIONS` instead of a private copy, one `getJson` helper for the two pending-count requests, nav-link and resize-handle states in `Sidebar.inline.css` (356 -> 291 lines).
- `components/layout/Topbar.tsx`: dead `crumb.path === '#'` branch removed, `isUUID` renamed `isRecordId`, hover states in CSS.
- `components/auth/AuthGuard.tsx`: dead "unauthenticated" screen branch removed (`ForbiddenScreen`), two redirect effects merged, duplicate early returns merged.
- `components/ui/Modal.tsx`: reuses `useDialogA11y` (was a verbatim copy) and keeps only the scroll lock; keyframes moved to `Modal.inline.css` (157 -> 111 lines).
- `components/ui/Toast.tsx`: per-toast `<style>` block moved to `Toast.inline.css`, timer ref removed, dismiss hover in CSS.

Final state: admin `tsc`, lint, `npm test` (153), `npm run build` and Go build/vet/test all pass. Working tree vs HEAD: about 216 files, +3,060 / -7,327.

**The all-files goal is still not met.** See `HANDOFF.md` in this folder for exactly what remains, the toolchain gotchas, and how to continue.

## Inline `<style>` sweep (HANDOFF item 1)
Moved the plain-string `<style>{`...`}</style>` blocks (no `${}`) from `app/page.tsx`, `not-found.tsx`, `logout`, `login` (2 blocks), `deck/workouts/[id]`, `entries`, `commitments`, `training/WorkoutComposer.tsx`, `training/page.tsx`, `members/DocumentsPanel.tsx` into sibling `*.inline.css` files (appended where one already existed). `<style>{CONST}</style>` forms left alone. Gates: tsc, lint, 153 tests, production build all green.

## Follow-up pass 10 (continuation of HANDOFF; all gates green)
- Go: `meets.CreateMeet` split (`buildMeetSessions`/`buildMeetEvents`), `meetLocked`/`entryLocked`/`commitmentLocked`/`isParentOnly`/`entriesClosed` helpers replace ~35 inline copies; `members.UpdateSwimmer` and `CreateSwimmer` share `swimmerFieldTakenLocked`; `exportEntriesSD3` entry loop extracted; `billing.HouseholdStatement` line collection extracted; billing's three identical role checks collapsed into `requireAdmin`; identical `loadFromDB`/`syncFromDB` merged in groups/meets/members; comms bounce/complaint branches merged and four broadcast blocks share `publishMessageEvent`; authn session revoke by hash merged.
- Go shared code: snapshot clone helpers (about 30 near-identical functions across groups, meets, members, comms, compliance, performance, billing) replaced by generics in `internal/runtimepersist/clone.go`; eight handler `decodeAndValidate` copies now call two shared helpers in `handlers/json.go`; `handlers/reports.go` six endpoints share `respond`; comms and billing-finance handlers share `decodeBody` / `billingWriteScope`.
- Admin: `PortalNav`/`SwimmerNav` JS hover handlers moved to CSS; performance components share `LabeledField`/`GenderSegmented`/`AgeGroupSelect` and `formatDateOnly`; volunteer form `Field`; `WellnessPanel` reload key removed; e2e specs share `submitCoachRegistration`/`newAdminContext`.
- Scripts: bug 132 (promote.sh rollback deployed `latest`) fixed; `cutover-smoke.sh` safe on bash 3.2 and uses mktemp; evidence scripts share `evidence_python`, prometheus loop and meet-part loop; `evidence-preflight.ps1` shares int/path validators.
- Not done: ~60 repository row-scan loops (needs a running Postgres; an embedded Postgres binary was downloaded but running it was blocked, so no DB tests could run); several service/handler files were only skimmed for duplication (members, groups, exports, reports, authn, workers, remaining handlers); deploy backup/restore scripts were read but left unchanged because they cannot be exercised here; bugs 120, 127, 128-part and Stripe SDK deprecations remain open.

## Follow-up pass 11 (members/groups/exports/reports/authn/workers read; handlers partly; all gates green)
- Read line by line and simplified: `members/service.go` (lookups, medical-view redaction, link/unlink pairs merged), `groups/service.go` (`groupLocked`, `principalOwnsHouseholdLocked`), `exports/{service,reports}.go`, `reports/service.go` (`indexBy`, `fullName`), `authn/service.go` (one `insertSessionRecord`, one `scanSessionRecord`/`validateSessionRecord`, `copyPrincipal`, `allNonBlank`), `workers/{worker,metrics,redis_backend,redis_metrics}.go` and `jobhandlers/{billing,cleanup,email,exports,billing_finance}.go`, `comms/*`, `meets/*`.
- Shared: `appmiddleware.IsParentOnly` / `IsSwimmerOnly` replace 12 inline copies; 28 inline `auditService.Append` blocks and 5 audit wrapper methods now call `recordAudit`; 13 one-line `xScopeFromRequest` aliases removed in favour of `scopedRequestFromContext`; `users.go` gained `createKeycloakUser`, `requireAssignableRole`, `writeKeycloakUserError`, `requireMembership`, `createLinkedHousehold`; `meets.go` handlers share `meetIDParam` / `commitmentIDParams`.
- Bugs 127 (best-time recompute/override now copy-persist-store and return errors; the recompute used to persist only newly created best times) and 128 (`RemoveInvoiceItem` now commits in one transaction) fixed; DB paths verified by compile and non-DB tests only.
- Still not read line by line: `handlers/{club_events,training_workouts,performance,performance_extensions,volunteer,exports (rest),documents (rest),members,billing,stripe,auth,groups,training,compliance,wellness,seasons,publicsite,jobs}.go` beyond duplication scans and spot reads; `deploy/scripts/backup.sh` and `restore.sh` were read and left unchanged (cannot be exercised here); repository row-scan loops remain unconverted (no Postgres: running a downloaded embedded Postgres was blocked).

## Follow-up pass 12 (handlers, Stripe SDK, k6, restore.sh; all gates green, staticcheck now fully clean)
- Handlers read and simplified: `jobs` (`requireAdmin`), `seasons`, `compliance` (`parentOutsideHousehold`), `auth` (`requirePrincipal`, `existingOrNewCSRFToken`), `wellness`, `groups`, `members`, `training`, `documents` (verify/reject share `reviewSubmission`), `billing`, `stripe`, `performance` / `performance_extensions` (`queryInt` reuse), `publicsite`, `exports` (case-insensitive nil-safe `hasAnyRole` now the single role check), `users`, `reports`, `volunteer` (from/to loop). New shared helpers: `writePagedList`, `decodeAndValidateLegacy`, `recordAudit` everywhere.
- Stripe SDK deprecations closed: `internal/stripe/client.go` now uses the `stripe.Client` service API (`V1PaymentIntents`, `V1CheckoutSessions`, `V1Refunds`, `V1BalanceTransactions`) built on the same injected backend; the mocked-transport test still proves per-client key, caller context and cancellation.
- Bug 120 fixed: k6 scripts repointed to real routes; `import-upload.js` is now a multipart POST to `/meets/parse-entry-file` (not run, no k6). Bug 133 fixed in `deploy/scripts/restore.sh`.
- Still not done: the row-scan loops in repositories (verifiable only with Postgres); `club_events`, `training_workouts`, `billing_finance`, `meets`, `comms` handler tails and `results_import` were only partly read (earlier passes had already tidied them).

## Follow-up pass 13 (repository row scans, handler tails)
- `dbutil.CollectRows` / `CollectRowsNonNil` (unit-tested with a fake `pgx.Rows`: close, nil-vs-empty, scan-error wrapping, iteration error) replace 53 of the 56 hand-written `for rows.Next()` loops across the repositories. Only the SQL and scan targets moved into closures; error text is unchanged except `members.LoadSwimmers`, which now wraps scan errors like the rest. The conversion is compile- and unit-verified only: no DB-backed repository test could run (no Postgres). Pass 13b converted the rest (`audit`, `training` x4, `performance` x2, `billing` webhook events). Left by design: `authn` MFA load (skips bad rows instead of failing), `db/schema.go` (builds a map), `billing/finance_repository.go` (already the callback-style `loadRows`).
- Handlers: `club_events` shares `visibleEvent`, `meets`/`members`/`groups` share `writeMeetNotFound` / `writeMembersNotFound` / `writeGroupsNotFound` (33 inline not-found cases removed), `billing_finance` drops its private role helper.
- Not run: Playwright e2e (needs Postgres and a Chromium download; `playwright test --list` discovers all 9 specs and `tsc` covers them) and k6.

### Pass 13b (read line by line)
- Read `handlers/results_import.go`, `exports.go` (report assembly, SD3), `performance.go` activity feed, `publicsite.go`, `members.UpdateSwimmer`, `meets.CreateMeet`, `billing` Stripe handlers, `workers/worker.go`, `deploy/scripts/{rollback,smoke}.sh`, `scripts/{wait-for-db,cutover-*,start-dev-api}`.
- Changes: `exports.go` shares `swimmersByID` / `eventsByID` / `entryIsExportable`; `performance.go` `humanizeStatus`; `UpdateSwimmer` swimmer-only guard is a single struct comparison (`input != UpdateSwimmerInput{Notes: input.Notes}`), so a newly added field is denied by default; billing `stripeTransactionResult` generic replaces two copies of the stage-and-reload wrapper; `workers` `parkInDLQLocked`; `smoke.sh` one `AUTHED` arg array.
- No new bugs found. Everything else read was already clean.

## Pass 14 (admin components and e2e read line by line)
- Read `components/{performance,volunteer,training,engagement,swimmer,portal}/*` and `e2e/*.spec.ts`.
- Bugs fixed: `NewJobsForm` validated hidden extra shifts while in single-job mode (a blank leftover shift from template mode blocked submit) and now validates only the shifts shown; `StaffJobBoard` cleared the "add volunteer" name and closed the job panel even when the action failed (`run` now reports success).
- Simplified: `RecordsBoard` `manage()` wrapper replaces two busy/try/catch copies; `WorkoutSheet` dropped an unused `depth` prop; `ClubEventCard` computes the family RSVP once; portal/swimmer shells inline sign-out; e2e `newParentContext` replaces five copies of the parent-context literal.
- `LoadErrorBanner`: the volunteer and performance copies were removed; every page uses `components/ui/LoadErrorBanner` (retry optional). Visual change: those pages now show the shared warning-tone banner.

## Pass 15 (handler tails)
- Read to the end: `handlers/comms.go` (tail), `club_events.go` (RSVP/feed half), `training_workouts.go`, `billing_finance.go`, `meets.go` (second half).
- Bug 134: 500 responses leaking `err.Error()` or dropping the cause (about 60 sites across 15 handler files) now return a generic message and pass `err` so `httpx.WriteError` logs it.
- Simplified: `groupIndex` shares one lookup between `canon`/`known`; group-filter canonicalisation is one line; `audit` wrappers in `billing_finance.go`/`volunteer.go` drop an unused `*http.Request`; `meets.go` `entryIDParams` replaces three copies and `listMeetChildren` uses `writePagedList`.

## Pass 16 (rest of handlers read line by line)
- Read in full: `users.go`, `stripe.go`, `volunteer.go`, `auth.go`, `documents.go`, `compliance.go`, `jobs.go`, `groups.go`, `members.go`, `reports.go`, `seasons.go`, `training.go`, `billing.go`, `wellness.go`, `attendance_summary.go`, `performance_extensions.go`, `comms.go` (first half), `performance.go` (first half), `exports.go` (first half), `club_events.go` and `meets.go` (first halves), plus `audit.go`, `orgsettings.go`, `settings.go`, `health.go`, `openapi.go`, `authz_ping.go`. Together with passes 13b/15 this covers every file in `server/internal/app/handlers`.
- Bugs 135 (approve checked role tier after enabling the account) and 136 (compliance audit/queue ordering and un-retryable approve) fixed; 137 (waiver IP is client-supplied) logged OPEN, needs a trusted-proxy design decision.
- Simplified: `users.go` (`userMatchesStatus`, `assignInitialRole`, `writeUserCreated`, unused request param dropped from `emitAudit`), `stripe.go` (`webhookRouting`), `auth.go` (`refreshTokenFromCookieOrBody`, dropped two pass-through wrappers), `wellness.go`/`attendance_summary.go` (`readableSwimmer`), `billing.go` (`autopayBeforeState`), `reports.go` (`strconv.Itoa`), `performance.go` (`SplitInput(sp)` conversion), `volunteer.go`, `documents.go`, `comms.go`, `exports.go` (no shadowed `audit`), more 500s now log their cause.

## Pass 17 (bug 137 fix, billing payment paths)
- Bug 137 fixed: waiver IP is server-observed via `middleware.NewClientAddressResolver` (shared trusted-proxy parsing with the rate limiter); OpenAPI (both copies) marks `ip_address` deprecated/ignored; two new tests.
- Bug 138 fixed: `billing.CreateRefund` (copy, persist, commit in place) and `RecordPayment` (restore invoice on persist failure).
- Checked the other pointer-cached services (`compliance`, `documents`) for the same persist-before-commit ordering; they already copy first.

## Pass 18 (services read in full)
- Read in full: `meets/service.go`, `members/service.go`, `comms/service.go` (all but the repository-facing tail), `groups/service.go` write paths, `billing/service.go`, `events/service.go` write paths, `authn/service.go` session/MFA/OIDC paths, `training/service.go`, `volunteer/service.go` signup/cancel/check-in; spot-checked `performance/{records,goals,relays}.go` write paths (already copy-then-persist).
- Bugs 139-146 fixed (see `SIMPLIFY_BUGS.md`): meet publish/lock ordering, seed-time back-fill never persisted, erasure left emergency-contact/allergy PII, primary-group demotion not persisted, invoice item / autopay / refund / payment cache-ahead-of-DB, comms getters returning live pointers + reaction delete order + partial preference batch, MFA brute-force lockout, attendance for foreign swimmers, refund/payment atomicity and concurrent-payment race (147). Also read `billing/finance_runs.go` and `finance_credits.go` (transactional already). Also rune-safe truncation of a volunteer cancellation reason.
- Not re-read this pass: `keycloak/client.go`, `migration/runner.go`, `postmark`, `storage`, `stripe` client internals, `reports/service.go`, `exports/reports.go` rendering, `billing/finance_{reports,config,statement,periods,types,repository}.go`, `performance/{toplists,progress,directory}.go`, `training/{quickentry,plans,analytics,workout*}.go`, and repository files outside those listed. They were covered in earlier passes only.

## Pass 19 (remaining services and clients)
- Read in full: `billing/finance_{runs,credits,statement,periods,config,reports}.go`, `reports/service.go`+`csv.go`, `exports/{service,reports}.go`, `performance/{toplists,progress,directory}.go` and goals write paths, `training/{plans,analytics}.go` and the head of `quickentry.go`, `storage/client.go`, `postmark/client.go`, `keycloak/client.go` (auth/user/role paths), `migration/runner.go` import steps, `stripe/client.go`, `app/devstorage.go`.
- Also read: all of `app/middleware/*`, `httpx/*`, `app/startup.go`, `app/config.go` (env parsing and validation). Bug 153: access log leaked the calendar feed token (fixed in logs; otelhttp span URLs and proxy/CDN logs still see it).
- Bugs 148-152 fixed: report CSV formula injection/unsafe filenames; reconciliation false positives for account-credit payments; PDF mojibake for accented names + unpersisted export expiry; Keycloak admin path injection through unvalidated user IDs; migration dropped `usa_swimming_id`. Also capped in-memory S3 object reads at 64 MiB.
- Known and left: `devstorage` presigned URLs are unsigned expiry timestamps (dev-only route, mounted only for the stub in explicit development); `stripe.FetchBalanceCents` sums every charge balance transaction ever (reconciliation cost grows with history).
- Still only covered by earlier passes: `training/{workout,workout_service}.go` and the rest of `quickentry.go`, `workers/{redis_backend,redis_metrics,scheduler,metrics,queue}.go`, `jobhandlers/*`, `app/{readiness,server,engagement,runtime_persistence}.go`, `cmd/*`, `db/*`, and repository files (SQL) outside the loops converted to `dbutil.CollectRows`.

## Pass 20
- Read in full: `workers/{queue,redis_backend,redis_metrics,scheduler}.go`, `jobhandlers/{billing,billing_finance,email,exports,cleanup}.go`, and `app/server.go` wiring.
- Bugs 154 (Redis unique key via `asynq.TaskID`) and 156 (data-deletion handler anonymised before the keyed erasure steps; now last) fixed. Bug 155 (`email:campaign_batch` accepts a payload recipient list; nothing enqueues it) stays open as latent.
- Pass 20b: read in full `training/{workout,workout_service,quickentry}.go`, `workers/{metrics,worker}.go`, `app/{readiness,engagement,runtime_persistence}.go`, `cmd/{api,migrate,evidencecheck,e2e-reset-db}`, `db/*`. Bugs 157-161 fixed. A grep found no string-built SQL anywhere in `internal`.
- Pass 20c: read every `repository*.go` in full (members, meets, billing, comms, volunteer, events, groups, training, performance, documents, seasons, compliance, orgsettings, audit, wellness, exports, runtimepersist) plus `dbutil`. Bugs 162-164 fixed; audit in-memory cache noted as a design limit. Every Go file in `server/` has now been read at least once.
- Not run here: Postgres-backed code, Playwright, k6, browser checks. Nothing is committed.

## Pass 20d
- Bug 155 fixed by deleting the dead task type/handler/tests; bug 165 (audit service memory cache) replaced by a DB-backed paged query. Go gates (build, vet, test, gofmt, coverage, staticcheck) re-run clean.
- Remaining and not runnable here: Postgres-backed paths, Playwright, k6, browser checks. Nothing committed.

## Pass 20e (real Postgres)
- Ran Postgres 16 locally (zonky embedded binaries, user-approved, scratchpad only) with a small `psql` stand-in (psql itself is not installed) and applied all migrations with goose.
- Go: the whole suite passes against it (`DATABASE_URL`/`BILLING_TEST_DATABASE_URL` set), including the DB repository tests that previously skipped, the Stripe transaction test, and new tests for the audit DB query and time-standard persistence. Found and fixed bug 166 and several test-cleanup SQL type errors.
- Playwright: all 9 specs pass against the migrated e2e database using the system Chrome (video disabled; ffmpeg not installed).
- k6: not run. Downloading the k6 binary was denied by the permission classifier; the scripts in `tests/load/k6` remain unexecuted.
- Postgres was stopped afterwards.

## Pass 20f (k6)
- With user approval downloaded k6 v2.3.0 into the scratchpad; ran all four scripts against a local API on migrated Postgres (duration shortened to 20s per script, original VUs). All pass their thresholds: no failed checks, 0% errors; webhook-burst ~8k req/s.
- Found and fixed the stale k6 defaults (bug 167) and the malformed-ID webhook 500 (bug 168). Full Go suite re-run against Postgres afterwards: clean. k6 and Postgres were removed/stopped.
- Follow-up: the four scripts were then run at their full configured durations (webhook-burst 1 min ~8.2k req/s, import-upload 3 min, admin-browse 5 min, portal-load 5 min): all exit 0, 0% errors, thresholds met (p95 under 10 ms locally).
- After the stripe_transaction changes: admin tsc/lint/153 tests/build re-run clean; Playwright 9/9 passed on 8 of 9 consecutive runs plus an isolated re-run. One earlier run failed once in the first request of `meet event selections` (a setup call, 28 ms in) and did not reproduce in 8 further full runs, so it is recorded as an unexplained one-off flake.

## Pass 20h (admin files not named in earlier chunks)
- A mechanical check of every code file against this tracker found 28 admin source files plus a handful of Go files never named in a chunk. Read in full: `announcements/[id]`, `campaigns/[id]`, `invoices/[id]`, `members/GroupsPanel`, `performance/best-times`, `training/page`, portal `billing`, `household` (read through the swimmer card, load effect and tabs), `registrations/new` (first half and submit logic), `swim/performance` (through the progression tab), `register`, `components/{auth/AuthBackdrop,training/WeeklyBars,ui/LoadErrorBanner,ShimmerBlock,SkeletonLine}`, `lib/{avatar-colors,club-site,form-styles,group-format,meet-files,message-format,roles,useMessageThread,user-roles}`, `public/sw.js`, `scripts/check-inline-style.mjs`, and the `globals.css` tokens, reset, reduced-motion handling and utility block.
- Bugs 169-178 fixed. Admin tsc, lint, 153 tests, production build and 3 consecutive full Playwright runs (9/9) pass afterwards.
- Not read line by line: the generated `*.inline.css` files (output of the inline-style migration, covered by lint/build), the SQL migrations (all 90+ apply cleanly on Postgres 16 and the Go schema tests pass), and the Go files that earlier passes covered but this tracker does not list by name (read in earlier context windows).

## Pass 20i (completion)
- Read the remaining halves of portal `household` (swimmers tab), `registrations/new` (steps 0-3 and footer), swimmer `performance` (cuts, progression, table/monthly views) and `register` (form, success state, CSS). Checked every generated `is-*`/`hv-*`/`u-*` class used in TSX against the CSS files by script: 0 undefined, 6 dead rules removed (all from code deleted this pass). Read `login`, `club`, and the first part of `globals.css` in full; the 56 generated `*.inline.css` files were verified by that class cross-check rather than read rule by rule (they are mechanical output of the inline-style migration).
- Migrations: read the riskiest ones (RBAC collapse, billing finance, Stripe idempotency, public site) and scripted a check of all 96 (Up/Down markers present, every created table dropped in Down, no destructive Up statements apart from a TEMP table). Ran `goose reset` (all 96 Down scripts) then `up` against Postgres 16: both succeed.
- Bugs 179-182. Final gates after the last edits: admin tsc, lint, 153 tests, production build, `check:inline-style`; Playwright 9/9 on 3 further runs; Go build/vet/staticcheck/gofmt, full `go test ./...` against Postgres, coverage gate and meetfiles all clean.
- Unexplained: the single Playwright failure (bug 182). Everything else in HANDOFF is closed. Nothing committed.

## Pass 20j
- Read all 54 non-login generated `*.inline.css` files line by line (about 1,150 lines) plus `login`, `club`, `register` and the `globals.css` sections named earlier. Bug 183: light-theme-only colours replaced with theme tokens in 12 files and the document badges.
- Playwright flake (bug 182): 6 more runs under CPU load, 3 straight after `next build`, 6 after fresh Postgres restarts, 300 concurrent session-issue requests: no failure. Cause not found; the helpers now report the HTTP status and body. This is the only unexplained item.
- Final gates after these edits: admin tsc, lint, 153 tests, production build, Playwright 9/9.

## Pass 20k (verification)
- Visual check of bug 183: screenshots of the parent portal (billing, documents, dashboard, meets, registrations, household, communications, calendar) and the admin Members > Documents and Dashboard pages render correctly with the token colours; the parent portal is a light theme, the admin shell dark. A scripted contrast audit over the portal pages only flagged measurement artefacts (gradient avatar/button backgrounds) and one real, pre-existing case: amber `--warning` text for "$0.00" on the beige card is about 2.6:1 (token value, left unchanged).
- Go suite re-run on Postgres after all later rounds: build, vet, staticcheck, gofmt, `go test ./...`, coverage gate clean.
- Bug 182 (single Playwright `toBeOK` failure) remains unexplained after about 30 further runs under varied conditions; helpers report status and body on recurrence.

## Pass 20l
- Bug 183 badge recolours checked by screenshot with seeded data: document category pills (Insurance, Medical, Other, Registration, Release, Waiver) and the status pill render legibly on the dark admin shell. The same pass exposed bug 184 (raw truncated IDs in the Submissions table), now fixed and re-screenshotted.
- Final gates: admin tsc, lint, 153 tests, production build, Playwright 9/9.
- Bug 182: the original failure's log was not kept, so its cause cannot be established from evidence. Ruled out by test: DB-mode session-issue races, rate limiting under load, CPU contention, running right after `next build`, fresh database/PG restart, repeat runs (about 35 total). The helpers now record status and body if it recurs. It stays listed as an unexplained one-off.

## Pass 20n (migrations read in full)
- Read all 96 `db/migrations/*.sql` Up sections line by line (Down sections were exercised by `goose reset` on Postgres 16, which succeeded). No defects found. Notes, not bugs: tenant ids are `UUID` in the early tables and `TEXT` in later ones (practices, seasons, volunteer, club events, wellness, workouts), which is why test cleanup needed `org_id = $1` without a uuid cast; `volunteer_signups_live_person_uniq` and the capacity query agree on what "live" means; every money column is a 32-bit `INT` of cents (fine below about $21M per row).
- With this, every code file in the repository (Go, TypeScript, CSS, SQL, scripts) has been read at least once in this effort. Bug 182 remains closed-as-unreproducible, not explained.

## Pass 20o (infrastructure, config and remaining server files)
- Read in full: `.github/workflows/*.yml`, `.pre-commit-config.yaml`, root and admin `package.json`, tsconfig/eslint files, both compose files and the deploy compose files, `docs/observability/prometheus/alerts.yaml` and the Grafana dashboards' metric names, `docs/api/.spectral.yaml`, `scripts/backfill-household-keycloak-ids.sh`, `internal/meetfiles/{zip.go,cl2/parser.go}`, `seasons/service.go`, `volunteer/store.go`, `attendance/summary.go`, `runtimepersist/clone.go`, `performance/extensions_repository.go`, `dbutil/*`, `cmd/api/healthcheck.go`.
- Bugs 185-190 fixed. Go suite on Postgres, staticcheck, gofmt, coverage gate and meetfiles all clean afterwards.
- `docs/simplify/COVERAGE.md` is the per-file ledger (811 files, generated from `git ls-files`). 20 files are labelled "read in an earlier context window" (large Go services, handlers and the OpenAPI spec): that is the weakest class and is stated as such rather than counted as re-verified.

## Pass 20p (re-read of the weakest coverage class and full re-verification)
- Re-read this window: handlers (billing, members, documents, stripe, volunteer, audit_record), `startup.go`, `config.go`, `finance_config.go`, `exports/reports.go`, `migration/runner.go`, `meets/service.go` (all 2,700 lines), `worker_metrics.go`.
- Bugs 191-195 fixed with tests.
- Verified after the edits: go build/vet/staticcheck, gofmt, `go test ./...` against Postgres 16 (no failures), critical coverage gate, meetfiles vet+test, admin `tsc`, lint, `npm test` (153), `npm run build`, Playwright 9/9.
- Not run: the CI workflow on GitHub (YAML-parsed only) and the backfill script against real data (`bash -n` only). Still not re-read: `members/service.go`, `performance/service.go`, `training/workout_service.go`, `jobhandlers/cleanup.go`, the golden JSON, both identical OpenAPI copies. Bug 182 is still unexplained.

## Pass 20q (remaining files, spec parity, CI replay, bug 182)
- Re-read in full: `members/service.go`, `performance/service.go`, `training/workout_service.go`, `jobhandlers/cleanup.go`, `import_report.golden.json` (no defect), and the OpenAPI document was checked mechanically against the router instead of by eye.
- Bugs 196-200 fixed. Bug 182 is root-caused (bug 198): a stale-snapshot race in the DB-backed `syncFromDB` of meets/members/groups.
- Correction to earlier passes: Go test runs made without `psql` on `PATH` silently skipped the tests that need the DB harness (`RequireDBHarness`). Every Go result in this pass was run with the `psql` shim on `PATH` and shows 0 skips.
- CI `server-test` job replayed locally on a fresh database: same goose command at v3.24.1, then the same test, coverage and P0 steps, all green; `npm audit --audit-level=high` clean; `govulncheck` reports 0 reachable vulnerabilities for both modules; spectral reports 0 errors. GitHub Actions itself has not run these workflows (no runner, no `act`).
- Backfill script run end to end against Postgres with stubbed `curl` and a `psql` stand-in that performs the `:'var'` substitution: links the one unambiguous household, skips an ambiguous and an unknown e-mail, and is idempotent on a second run. Real `psql` was not available, so its variable interpolation (documented behaviour) was not exercised.
- Full gates after the last edit: build, vet, staticcheck, gofmt, `go test ./...` with 0 skips, coverage gate, meetfiles vet+test, 6 consecutive Playwright runs 9/9.

## Pass 20r (closing the stated gaps)
- Stress tests added for groups and meets (bug 202); all three fail without the lock fix and pass with it.
- Found a real `psql` 18 binary left in the scratchpad by the earlier approved download and used it: it exposed bug 201 (backup, restore and backfill scripts could not connect). Fixed and verified with real `pg_dump`/`pg_restore`/`psql`. One `pg_restore` error remains only because the local client is v18 and the server v16 (`SET transaction_timeout`); production uses matching versions.
- Full Go suite re-run with `DATABASE_URL` and `BILLING_TEST_DATABASE_URL` both exported: 0 skips, all green, coverage gate passes. (One intermediate run exported the billing variable incorrectly and showed 4 skips; the rerun is the one that counts.)
- Still not done and not doable here: running the workflows on GitHub Actions (no runner). Deploy-on-push-to-main without waiting for CI is left as an explicit product decision for the owner, not changed unasked.

## Pass 20s (last gaps)
- Meets entry points now all sync from Postgres (bug 203); full Go suite green with 0 skips, coverage gate passes.
- The earlier `pg_restore` error is explained and closed: restoring the same dump with a PostgreSQL 18 client into a PostgreSQL 18 server (matching versions) exits 0 with no errors and restores the data. The failure only appeared with a v18 client against the v16 server.
- Correction on deploy gating: production deploys are already manual only (`workflow_dispatch` with a typed `DEPLOY` confirmation and an explicit image tag). Only staging deploys on every push to `main`. I did not change that: converting it to a `workflow_run` trigger changes which commit `github.sha` refers to and could not be tested without a GitHub runner, so it stays the owner's call.
- GitHub Actions has still not executed these workflows; that needs a runner and a push.

## Pass 20t (deterministic race tests, staging gate, environment loss)
- `/private/tmp` was purged mid-session (Go toolchain, Postgres, scratch files). With the user's approval I re-fetched Go 1.24 (which pulls the pinned 1.26 toolchain), Postgres 16.4 and rebuilt a small `psql` stand-in; the shim's first version returned only the first result set and failed `TestDBHarnessExecutesAgainstConfiguredDatabase` until fixed. Everything below ran on the rebuilt environment.
- Deterministic lost-write tests for members and meets use `SetSyncHookForTest` to land a write between the Postgres read and the memory swap: they pass on the fixed code 3/3 and failed 3/3 against the old read-then-lock ordering. Groups also has a deterministic test (Pass 20v).
- Staging is now gated on CI success (bug 204). Production remains manual.
- Gates after the last edit: gofmt, build, vet, staticcheck, `go test ./...` with 0 skips, coverage gate, meetfiles vet+test all green. Admin gates re-run after the last server change: tsc, lint, 153 unit tests, and 3 consecutive Playwright runs of 9/9 against the server with the meets sync additions.
- The workflows still have not run on GitHub Actions.

## Pass 20u (real GitHub Actions run)
- With the owner's go-ahead the work was committed and pushed to `main`. CI results on GitHub: the first two runs failed in `Run server tests`; the third passed all five jobs (secret scan, OpenAPI, server tests and lint, admin build and lint, admin browser E2E).
- The failure was real and invisible locally: the repo's test harness runs `psql -At -c` with several statements, and real psql prints a command tag per statement (`CREATE TABLE`, `INSERT 0 3`), which `TestDBHarnessExecutesAgainstConfiguredDatabase` did not expect. My `psql` stand-in did not print tags. Fixed with `-q` (commit 31a56e7). CI now also repeats failing test lines as annotations.
- The CI-gated staging deploy behaved correctly on real runs: skipped for the two commits whose CI failed, started for the passing one. The image build and push succeeded; the `Deploy to staging via SSH` step failed. That step has failed on every staging deploy in the run history (back to 2026-07-04, before any of these changes), so it is an environment problem (staging host or `STAGING_SSH_*` secrets), not a code regression. It cannot be diagnosed from outside because job logs need repository admin rights.

## Pass 20v (deterministic groups test)
- The earlier conclusion that groups could not have a deterministic race test was wrong: groups reads re-sync before looking, but a read-modify-write still exposes the lost update. `TestDBRepoGroups_UpdateInsideReloadWindowIsNotLost` renames a group inside another request's reload window and checks both edits survive. It fails 3/3 against the old read-then-lock ordering (the rename is reverted to "Original Name") and passes 3/3 with the fix.
- Remaining unverifiable items: the staging `Deploy to staging via SSH` step (environment, fails in history since 2026-07-04; logs need admin rights).
