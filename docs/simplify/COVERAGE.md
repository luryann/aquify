# Coverage ledger

Generated from `git ls-files` plus untracked files (code, config and SQL; lockfiles and `.d.ts` excluded). One row per file with the strongest evidence this effort can point to. Evidence classes:

- **named in a TRACKER.md chunk** — reviewed in the chunked passes 1-19 (the chunk entry lists the file).
- **Pass 20h-20o** — read in full in the final passes (details in TRACKER.md).
- **re-read in full in Pass 20p/20q** — the files that used to be labelled "read in an earlier context window"; all were re-read in the final windows. The two OpenAPI copies (byte-identical) were additionally checked mechanically: every router operation is documented and nothing stale remains (`TestOpenAPIMatchesRegisteredRoutes`), and spectral reports 0 errors. No file is left in the weaker class.

Total files: 811

| File | Evidence |
|---|---|
| `.github/workflows/ci.yml` | Pass 20o: read in full |
| `.github/workflows/deploy-production.yml` | Pass 20o: read in full |
| `.github/workflows/deploy-staging.yml` | Pass 20o: read in full |
| `.pre-commit-config.yaml` | Pass 20o: read in full |
| `admin/.eslintrc.json` | Pass 20o: read in full |
| `admin/app/(admin)/announcements/[id]/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/announcements/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/attendance/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/audit/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/AutopayPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/InvoicesPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/LedgerPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/OverviewPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/autopay/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/financeUi.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/reports/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/runs/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/schedules/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/billing/stripe/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/campaigns/CampaignsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/campaigns/SuppressionPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/campaigns/[id]/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/campaigns/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/commitments/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/commitments/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/comms/suppression/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/compliance/data-deletions/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/compliance/data-exports/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/compliance/documents/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/dashboard/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/dashboard/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/entries/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/entries/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/events/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/exports/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/invoices/[id]/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/invoices/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/jobs/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/layout.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/ledger/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/[id]/AddMeetEventForm.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/[id]/EntryFileExport.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/[id]/ReportsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/[id]/ResultsImport.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/new/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/meets/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/DocumentsPanel.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/members/DocumentsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/GroupsPanel.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/members/HouseholdsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/RegistrationsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/SwimmersPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/approvals/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/groups/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/groups/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/households/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/households/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/registrations/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/swimmers/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/members/swimmers/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/messages/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/best-times/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/performance/progress/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/records/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/relays/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/time-standards/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/top-times/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/performance/visualization/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/practices/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/reports/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/seasons/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/settings/PublicSitePanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/settings/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/settings/roles/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/settings/system/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/training/SeasonPlanPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/training/TrainingAnalyticsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/training/WorkoutComposer.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/training/WorkoutComposer.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/training/WorkoutLibraryPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/training/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(admin)/training/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(admin)/users/RolesPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/users/UsersPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/users/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/users/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/volunteer/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(admin)/wellness/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/layout.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/billing/PortalStatement.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/billing/PortalStatement.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/billing/invoices/[id]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/billing/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/billing/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(portal)/portal/calendar/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/calendar/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/communications/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/communications/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/dashboard/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/dashboard/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/documents/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/documents/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/events/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/events/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/household/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/household/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(portal)/portal/household/swimmers/[id]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/household/swimmers/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/meets/[id]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/meets/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/meets/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/meets/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/messages/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/messages/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/records/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/registrations/new/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/registrations/new/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(portal)/portal/registrations/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/registrations/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/volunteer/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/volunteer/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(portal)/portal/wellness/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(portal)/portal/wellness/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/layout.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/meets/[id]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/meets/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/messages/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/messages/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/performance/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/performance/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/(swimmer)/swim/profile/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/profile/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/records/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/schedule/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/schedule/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/(swimmer)/swim/wellness/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/(swimmer)/swim/wellness/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/api/health/route.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/api/v1/[...path]/route.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/auth/callback/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/auth/callback/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/error.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/layout.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/club/[slug]/layout.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/news/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/club/[slug]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/records/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/club/[slug]/records/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/[slug]/schedule/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/club/[slug]/schedule/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/club.css` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/club/error.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/deck/workouts/[id]/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/deck/workouts/[id]/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/globals.css` | Pass 20h-20j: read in full |
| `admin/app/layout.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/login/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/login/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/logout/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/logout/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/manifest.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/mfa/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/mfa/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/not-found.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/not-found.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/offline/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/offline/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/providers.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/app/register/page.tsx` | Pass 20h-20j: read in full |
| `admin/app/register/register.css` | Pass 20h-20j: read in full |
| `admin/app/unauthorized/page.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/app/unauthorized/page.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/auth/AuthBackdrop.tsx` | Pass 20h-20j: read in full |
| `admin/components/auth/AuthGuard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/auth/auth.css` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/club/ClubNav.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/dev/DevRoleSwitcher.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/CalendarSubscribeCard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/ClubEventCard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/RSVPControl.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/ReadinessTrend.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/SwimmerClubEvents.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/SwimmerEngagementCard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/WellnessCheckInForm.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/WellnessPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/WellnessPromptCard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/engagement/vizTokens.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/forms/FormWrapper.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/layout/AdminShell.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/layout/CommandPalette.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/layout/Sidebar.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/layout/Sidebar.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/layout/Topbar.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/layout/Topbar.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/performance/ClubRecordsView.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/performance/GoalsPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/performance/PerfControls.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/performance/RecordsBoard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/performance/TopTimesBoard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/portal/PortalNav.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/portal/PortalNav.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/portal/PortalShell.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/portal/portal.css` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/pwa/InstallPrompt.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/pwa/ServiceWorkerRegistrar.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/swimmer/SwimmerNav.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/swimmer/SwimmerNav.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/swimmer/SwimmerShell.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/swimmer/SwimmerShell.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/swimmer/UnlinkedSwimmerNotice.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/swimmer/UnlinkedSwimmerNotice.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/training/WeeklyBars.tsx` | Pass 20h-20j: read in full |
| `admin/components/training/WorkoutSheet.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/Button.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/ConfirmDialog.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/EmptyState.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/Field.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/LoadErrorBanner.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/LoadErrorBanner.tsx` | Pass 20h-20j: read in full |
| `admin/components/ui/Modal.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/Modal.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/ShimmerBlock.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/ShimmerBlock.tsx` | Pass 20h-20j: read in full |
| `admin/components/ui/Skeleton.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/SkeletonLine.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/SkeletonLine.tsx` | Pass 20h-20j: read in full |
| `admin/components/ui/Table.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/Table.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/Tabs.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/Toast.inline.css` | Pass 20j: read line by line (generated inline-style output) |
| `admin/components/ui/Toast.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/icons.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/ui/useDialogA11y.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/volunteer/MeetVolunteersPanel.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/volunteer/NewJobsForm.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/volunteer/StaffJobBoard.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/volunteer/VolunteerHoursStrip.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/components/volunteer/VolunteerPrimitives.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/e2e/access-requests.spec.ts` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `admin/e2e/portal-critical-paths.spec.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/e2e/support/app.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/e2e/support/fixtures.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/adminData.step29e.test.ts` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `admin/lib/adminData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/api-proxy.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/api-proxy.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/api.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/api.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/auth-routing.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/auth-routing.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/autopay.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/autopay.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/avatar-colors.ts` | Pass 20h-20j: read in full |
| `admin/lib/club-events.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/club-events.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/club-site.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/club-site.ts` | Pass 20h-20j: read in full |
| `admin/lib/collection-load-state.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/collection-load-state.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/dates.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/document-submissions.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/document-submissions.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/engagementData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/event-eligibility.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/event-eligibility.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/featureFlags.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/financeData.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/financeData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/form-styles.ts` | Pass 20h-20j: read in full |
| `admin/lib/frontend-telemetry.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/frontend-telemetry.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/gender.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/group-format.test.ts` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `admin/lib/group-format.ts` | Pass 20h-20j: read in full |
| `admin/lib/household-status.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/household-status.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/install-prompt.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/install-prompt.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/localDev.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-draft.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-draft.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-entry-save.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-entry-save.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-events.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-events.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/meet-files.test.ts` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `admin/lib/meet-files.ts` | Pass 20h-20j: read in full |
| `admin/lib/message-format.test.ts` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `admin/lib/message-format.ts` | Pass 20h-20j: read in full |
| `admin/lib/mfa-flow.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/mfa-flow.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/performance-periods.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/performanceData.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/performanceData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-billing-checkout.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-communications-preferences.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-communications-preferences.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-invoice-checkout.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-meet-event-selections.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-meet-event-selections.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-page-failure-states.test.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-registration-status.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-registration-status.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-registration-wizard.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/portal-registration-wizard.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/practices-route.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/practices-route.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/public-org.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/public-org.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/publicSiteData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/relay-builder.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/roles.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/roles.ts` | Pass 20h-20j: read in full |
| `admin/lib/roll-call-queue.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/roll-call-queue.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/session.tsx` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/sidebar-badge.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/sidebar-badge.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-bio.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-bio.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-cut-qualifications.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-cut-qualifications.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-meet-detail-state.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-meet-detail-state.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-performance-cuts.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/swimmer-performance-cuts.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/trainingData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/useMessageStream.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/useMessageThread.ts` | Pass 20h-20j: read in full |
| `admin/lib/useStaffNames.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/user-roles.ts` | Pass 20h-20j: read in full |
| `admin/lib/volunteer-state.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/volunteer-state.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/volunteerData.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/wellness-scale.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/wellness-scale.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/workout-format.test.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/lib/workout-format.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/next.config.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/package.json` | Pass 20o: read in full |
| `admin/playwright.config.ts` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/public/sw.js` | Pass 20h-20j: read in full |
| `admin/scripts/check-inline-style.mjs` | Pass 20h-20j: read in full |
| `admin/scripts/check-offline-build.mjs` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/scripts/e2e/global-setup.mjs` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/scripts/e2e/global-teardown.mjs` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/scripts/e2e/start-api.mjs` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/scripts/generate-pwa-icons.mjs` | named in a TRACKER.md chunk (pass 1-19) |
| `admin/tsconfig.json` | Pass 20o: read in full |
| `admin/tsconfig.test.json` | Pass 20o: read in full |
| `db/migrations/20260315000100_init_extensions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000200_create_users.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000300_create_roles.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000400_create_user_roles.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000500_create_sessions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000600_create_mfa_factors.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000700_harden_user_roles_uniqueness.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000800_seed_default_roles.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315000900_create_households.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001000_create_contacts.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001100_create_swimmers.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001200_create_swimmer_groups.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001400_create_groups.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001500_create_eligibility_rules.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001600_create_registrations.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001700_create_waiver_acceptances.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001800_create_meets.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315001900_create_meet_sessions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002000_create_meet_events.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002100_create_commitments.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002200_create_entries.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002300_create_entry_legs.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002400_add_soft_delete_to_meets_tables.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002500_create_file_imports.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002600_create_file_import_rows.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002700_create_file_import_errors.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002800_create_match_conflicts.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315002900_create_file_exports.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003000_create_results.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003100_create_splits.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003200_create_best_times.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003300_create_time_standards.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003400_results_swimmer_not_null.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003500_create_invoices.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003600_create_invoice_items.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003700_create_payments.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003800_create_refunds.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315003900_create_ledger_events.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004000_create_webhook_events.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004100_create_announcements.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004200_create_email_campaigns.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004300_create_email_deliveries.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004400_create_suppression_list.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260315004500_add_suppressed_count_to_campaigns.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260316000100_create_audit_events.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260316000200_create_data_export_requests.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260316000300_create_data_deletion_requests.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260318090000_create_runtime_snapshots.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260322000100_create_document_types.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260322000200_create_document_submissions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260322000300_document_check_constraints.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260322000400_fix_document_types_unique_constraint.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260323000100_create_practices.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260323000200_fix_missing_columns.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260323000300_fix_missing_columns_2.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260323000400_fix_entries_missing_columns.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260324000100_add_keycloak_user_id_to_households.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260324000200_backfill_households_keycloak_user_id.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260324000300_create_mfa_factors.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260325000100_fix_mfa_factors_schema.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260325000200_fix_invoices_due_date_schema.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260326000100_add_meet_signup_settings.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260326000200_expand_file_import_types.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260421000100_harden_persistent_sessions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260519000100_create_notification_preferences.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260519000200_create_household_autopay_profiles.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260702000100_create_seasons.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260702000200_usa_swimming_id_and_org_settings.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260702000300_entry_scratched_status.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260703000100_add_keycloak_user_id_to_swimmers.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260703000200_create_practice_attendance.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260703000300_add_emergency_contact_to_swimmers.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260703000400_create_swimmer_link_requests.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260703000500_create_messages.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260704000100_org_settings_email_sender.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260704000200_add_message_edits.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260704000300_add_message_reactions.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260704000400_add_message_attachments.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260704000500_add_message_reads.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260705000100_collapse_rbac_roles.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260705000200_drop_file_imports_domain.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260706000100_allow_report_export_types.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260706000200_uppercase_swimmer_group_codes.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260706000300_allow_failed_deletion_status.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260710000100_create_volunteer.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260711000100_create_training_workouts.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260711000200_create_training_plan_phases.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260712000100_create_team_record_entries.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260712000200_create_swimmer_goals.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260712000300_create_relay_lineups.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260713000100_create_billing_finance.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260714000100_create_club_events.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260714000200_create_wellness_checkins.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260715000100_public_team_site.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `db/migrations/20260716000100_billing_stripe_idempotency.sql` | Pass 20n: Up read line by line; Down run by goose reset |
| `deploy/docker-compose.production.yml` | Pass 20o: read in full |
| `deploy/docker-compose.staging.yml` | Pass 20o: read in full |
| `deploy/grafana/datasources.production.yml` | Pass 20o: read in full |
| `deploy/grafana/datasources.staging.yml` | Pass 20o: read in full |
| `deploy/keycloak/realm-aquify-local.json` | Pass 20o: read in full |
| `deploy/keycloak/themes/aquify/login/resources/css/aquify.css` | Pass 20o: read in full |
| `deploy/loki/loki.yml` | Pass 20o: read in full |
| `deploy/prometheus/prometheus.production.yml` | Pass 20o: read in full |
| `deploy/prometheus/prometheus.staging.yml` | Pass 20o: read in full |
| `deploy/scripts/backup.sh` | Pass 20o: read in full |
| `deploy/scripts/promote.sh` | Pass 20o: read in full |
| `deploy/scripts/restore.sh` | Pass 20o: read in full |
| `deploy/scripts/rollback.sh` | Pass 20o: read in full |
| `deploy/scripts/smoke.sh` | Pass 20o: read in full |
| `docker-compose.yml` | Pass 20o: read in full |
| `docs/api/.spectral.yaml` | Pass 20o: read in full |
| `docs/api/openapi.yaml` | Pass 20q: operation-by-operation parity with the router (test), spectral lint, edited for bugs 199 |
| `docs/observability/grafana/api-error-rates.json` | Pass 20o: read in full |
| `docs/observability/grafana/import-success-failure.json` | Pass 20o: read in full |
| `docs/observability/grafana/queue-depth-job-failures.json` | Pass 20o: read in full |
| `docs/observability/grafana/webhook-lag.json` | Pass 20o: read in full |
| `docs/observability/prometheus/alerts.yaml` | Pass 20o: read in full |
| `internal/meetfiles/cl2/parser.go` | Pass 20o: read in full |
| `internal/meetfiles/cl2/parser_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/detector.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/detector_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/fixture_directory_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/hy3.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/hy3_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/hytek_events.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/hytek_events_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/meet_zip.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/model.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/sdif/exporter.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/sdif/exporter_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/sdif/parser.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/sdif/parser_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/sdif/records.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/testdata/build_fixtures.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/testdata/import_report.golden.json` | re-read in full in Pass 20q (final window) |
| `internal/meetfiles/zip.go` | Pass 20o: read in full |
| `internal/meetfiles/zip_limits.go` | named in a TRACKER.md chunk (pass 1-19) |
| `internal/meetfiles/zip_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `package.json` | Pass 20o: read in full |
| `scripts/backfill-household-keycloak-ids.sh` | Pass 20o: read in full |
| `scripts/cutover-billing-check.sh` | Pass 20o: read in full |
| `scripts/cutover-certification.sh` | Pass 20o: read in full |
| `scripts/cutover-rehearsal.sh` | Pass 20o: read in full |
| `scripts/cutover-smoke.sh` | Pass 20o: read in full |
| `scripts/evidence-preflight.ps1` | Pass 20o: read in full |
| `scripts/launch-evidence-bundle.sh` | Pass 20o: read in full |
| `scripts/lib/evidence-api.sh` | Pass 20o: read in full |
| `scripts/parallel-meet-cycle-evidence.sh` | Pass 20o: read in full |
| `scripts/provider-validation-evidence.sh` | Pass 20o: read in full |
| `scripts/stability-report-evidence.sh` | Pass 20o: read in full |
| `scripts/start-dev-api.ps1` | Pass 20o: read in full |
| `scripts/start-keycloak-local.ps1` | Pass 20o: read in full |
| `scripts/test-evidence-preflight.ps1` | Pass 20o: read in full |
| `scripts/wait-for-db.sh` | Pass 20o: read in full |
| `server/cmd/api/healthcheck.go` | Pass 20o: read in full |
| `server/cmd/api/healthcheck_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/api/main.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/api/main_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/e2e-reset-db/main.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/e2e-reset-db/main_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/evidencecheck/main.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/evidencecheck/main_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/cmd/migrate/main.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/attendance/summary.go` | Pass 20o: read in full |
| `server/internal/app/attendance/summary_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/audit/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/audit/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/auth_response_contract_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/authn/localdev.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/authn/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/authn/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_config.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/billing/finance_credits.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_latefees.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_periods.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_reports.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_runs.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_statement.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/finance_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/billing/finance_types.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/service_invariants_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/stripe_credit_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/billing/stripe_regression_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/billing/stripe_transaction.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/stripe_transaction_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing/webhook_repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/billing_roster.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/comms/realtime.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/comms/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/comms/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/comms/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/comms/service_messages_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/compliance/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/compliance/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/compliance/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/compliance/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/config.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/devstorage.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/devstorage_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/documents/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/documents/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/documents/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/documents/types_erasure_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/engagement.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/events/feed.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/events/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/events/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/events/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/exports/reports.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/exports/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/exports/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/exports/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/exports/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/groups/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/groups/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/groups/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/attendance_summary.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/audit.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/audit_record.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/auth.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/authz_ping.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/billing.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/billing_finance.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/club_events.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/comms.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/compliance.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/documents.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/engagement_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/exports.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/groups.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/groups_waiver_ip_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/handlers/health.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/jobs.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/json.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/json_pagination_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/meets.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/members.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/openapi.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/orgsettings.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/pagination.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/performance.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/performance_extensions.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/publicsite.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/reports.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/results_import.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/scope.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/sd3_results_helpers_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/seasons.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/settings.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/stripe.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/stripe_webhook_retry_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/handlers/training.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/training_workouts.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/users.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/handlers/volunteer.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/handlers/wellness.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/household_export.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/httpx/errors.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/httpx/validation.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/event_catalog_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/lifecycle_entries_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/repository_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/results_file.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/meets/service.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/meets/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/members/link_requests.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/members/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/members/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/members/schema_contract_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/members/service.go` | re-read in full in Pass 20q (final window) |
| `server/internal/app/members/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/metrics_config.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/auth.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/auth_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/client_address_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/middleware/csrf.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/csrf_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/impersonation.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/impersonation_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/logging.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/logging_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/middleware/org_scope.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/org_scope_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/permissions.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/rate_limit.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/rbac.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/rbac_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/recovery.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/security_headers.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/session_cookies.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/session_cookies_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/middleware/telemetry.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/orgsettings/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/orgsettings/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/directory.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/extensions_repository.go` | Pass 20o: read in full |
| `server/internal/app/performance/goals.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/progress.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/records.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/records_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/relays.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/relays_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/results_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/runtime_snapshot.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/service.go` | re-read in full in Pass 20q (final window) |
| `server/internal/app/performance/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance/toplists.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/performance_directory.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/publicsite_wiring.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/readiness.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/reports/csv.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/reports/csv_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/reports/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/roster/members_directory.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/roster/roster.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/runtime_persistence.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/runtime_persistence_integration_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/runtime_persistence_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/seasons/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/seasons/service.go` | Pass 20o: read in full |
| `server/internal/app/seasons/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_auth_sessions_db_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_authz_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_billing_finance_db_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_billing_finance_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_billing_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_comms_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_compliance_jobs_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_db_audit_standards_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_db_repos_performance_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_db_repos_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_db_training_workouts_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_documents_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_engagement_db_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_exports_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_groups_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_localdev_db_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_meets_hytek_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_meets_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_members_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_messages_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_metrics_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_openapi_contract_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_org_scope_contract_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_performance_records_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_performance_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_provider_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_publicsite_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_reports_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_results_import_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_sd3_export_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_seasons_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_security_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_settings_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step16_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step17_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step23_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step30b_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step30e_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/server_step30f_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step31_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step32b_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_step32c_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_stripe_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_swimmer_authz_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_training_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_training_workouts_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_volunteer_db_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/server_volunteer_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/startup.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/startup_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/testhelpers_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/app/training/analytics.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/plans.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/quickentry.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/quickentry_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/workout.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/workout_repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/training/workout_service.go` | re-read in full in Pass 20q (final window) |
| `server/internal/app/training/workout_service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/volunteer/reminder.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/volunteer/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/volunteer/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/volunteer/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/volunteer/store.go` | Pass 20o: read in full |
| `server/internal/app/volunteer_wiring.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/wellness/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/wellness/service.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/wellness/service_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/app/worker_metrics.go` | re-read in full in Pass 20p (final window) |
| `server/internal/app/worker_metrics_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/db/pool.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/db/schema.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/db/schema_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/dbutil/id.go` | Pass 20o: read in full |
| `server/internal/dbutil/nullable.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/dbutil/rows.go` | Pass 20o: read in full |
| `server/internal/dbutil/rows_test.go` | test file, not individually read: executed by every gate run (go test / npm test / Playwright) |
| `server/internal/keycloak/client.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/keycloak/client_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/migration/runner.go` | re-read in full in Pass 20p (final window) |
| `server/internal/migration/runner_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/openapi/spec.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/openapi/spec/openapi.yaml` | Pass 20q: byte-identical copy of docs/api/openapi.yaml (`cmp` + existing drift test) |
| `server/internal/openapi/spec_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/postmark/client.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/postmark/client_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/runtimepersist/clone.go` | Pass 20o: read in full |
| `server/internal/runtimepersist/repository.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/storage/client.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/storage/client_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/stripe/client.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/stripe/sdk_client_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/telemetry/metrics.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/telemetry/telemetry.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/testutil/integration.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/testutil/integration_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/testutil/testutil.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/testutil/testutil_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/billing.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/billing_finance.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/billing_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/cleanup.go` | re-read in full in Pass 20q (final window) |
| `server/internal/workers/jobhandlers/cleanup_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/email.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/email_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/jobhandlers/exports.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/metrics.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/metrics_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/queue.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/redis_backend.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/redis_metrics.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/scheduler.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/worker.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/internal/workers/worker_options_test.go` | named in a TRACKER.md chunk (pass 1-19) |
| `server/scripts/check_critical_coverage.go` | named in a TRACKER.md chunk (pass 1-19) |
| `tests/load/k6/admin-browse.js` | named in a TRACKER.md chunk (pass 1-19) |
| `tests/load/k6/import-upload.js` | named in a TRACKER.md chunk (pass 1-19) |
| `tests/load/k6/portal-load.js` | named in a TRACKER.md chunk (pass 1-19) |
| `tests/load/k6/webhook-burst.js` | named in a TRACKER.md chunk (pass 1-19) |
