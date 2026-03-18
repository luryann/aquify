DARE Platform — Frontend Directory
MARK EACH PAGE DONE AS YOU GO!!!!!
  ---
  Application Structure

  The frontend splits into three portals, all under the same Next.js app, segmented by role:

  admin/
    app/
      (auth)/              ← unauthenticated routes
      (admin)/             ← owner, admin, coach, billing, read-only staff
      (parent)/            ← parent role
      (swimmer)/           ← optional swimmer role
      shared/              ← shared components, hooks, lib

  ---
  Portal 1: Admin / Staff (/admin/*)

  Roles: owner, admin, coach, billing, read-only

  Navigation Modules

  Dashboard · Members · Meets · Files · Performance · Billing · Comms · Audit · Jobs · Reports · Settings

  ---
  Dashboard

  ┌────────┬─────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │ Route  │      Page       │                                                                  Key Content                                                                   │
  ├────────┼─────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin │ Admin Dashboard │ KPI cards (active swimmers, open meets, unpaid invoices, pending conflicts), recent activity feed, system health indicators, quick-action rail │
  └────────┴─────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Members Module

  ┌────────────────────────────────┬──────────────────┬────────────────────────────────────────────────────────────────────────────────────────────┐
  │             Route              │       Page       │                                        Key Content                                         │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members                 │ Roster           │ Searchable/filterable swimmer list, group filter, registration status badges, bulk actions │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/households      │ Households       │ Household list with balance + registration status; create new household                    │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/households/[id] │ Household Detail │ Contacts, emergency info, linked swimmers, invoice summary, registration history           │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/swimmers/[id]   │ Swimmer Detail   │ Profile fields, group assignments, best times summary, registration status, audit history  │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/groups          │ Groups           │ Group list with swimmer counts, eligibility rule badges                                    │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/groups/[id]     │ Group Detail     │ Member list, eligibility rules editor, group settings                                      │
  ├────────────────────────────────┼──────────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/members/registrations   │ Registrations    │ Registration queue with status filter (pending/active/expired), waiver status              │
  └────────────────────────────────┴──────────────────┴────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Meets Module

  ┌───────────────────────────────┬──────────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────┐
  │             Route             │         Page         │                                           Key Content                                           │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets                  │ Meets List           │ Meet cards with lifecycle state, commitment deadline countdown, entry status                    │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/new              │ Create Meet          │ Name, dates, course, host, sessions/events setup wizard                                         │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]             │ Meet Overview        │ Status banner, lifecycle controls (draft→published→commitment_open→...), summary stats          │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]/sessions    │ Sessions             │ Session list with time/venue; add/edit sessions                                                 │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]/events      │ Events               │ Event list (stroke/distance/age group/course); qualifying times; add/edit events                │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]/commitments │ Commitment Dashboard │ Per-swimmer yes/no/pending grid; publish commitment request; lock commitments action            │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]/entries     │ Entry Management     │ Entry table by swimmer/event; approve/override with audit reason; batch approve                 │
  ├───────────────────────────────┼──────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/meets/[id]/exports     │ Export Files         │ Generate Standard SD3 / Extended SD3 / SDIF roster; download generated files; export job status │
  └───────────────────────────────┴──────────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  File Interoperability Module

  ┌─────────────────────────────────────┬───────────────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │                Route                │           Page            │                                                         Key Content                                                         │
  ├─────────────────────────────────────┼───────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/files/imports                │ Import Jobs               │ Job list with status (uploaded→parsing→conflicts_pending→applied), file type badges, timestamps                             │
  ├─────────────────────────────────────┼───────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/files/imports/[id]           │ Import Detail             │ File info, parse progress, match stats, error summary, download report                                                      │
  ├─────────────────────────────────────┼───────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/files/imports/[id]/conflicts │ Conflict Resolution Queue │ Conflict cards (unknown_athlete, ambiguous_match, bad_event_code, duplicate_row); resolve/reject per conflict; bulk actions │
  ├─────────────────────────────────────┼───────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/files/exports                │ Export Jobs               │ Generated file list with pending→ready→expired states; download links; re-generate action                                   │
  └─────────────────────────────────────┴───────────────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Performance Module

  ┌───────────────────────────────────┬───────────────────┬────────────────────────────────────────────────────────────────────────────────┐
  │               Route               │       Page        │                                  Key Content                                   │
  ├───────────────────────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/performance/time-standards │ Time Standards    │ Standards list by course/age group; create/edit/delete; cut criteria           │
  ├───────────────────────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/performance/best-times     │ Best Times Lookup │ Search by swimmer; best times table (SCY/SCM/LCM); manual override with reason │
  └───────────────────────────────────┴───────────────────┴────────────────────────────────────────────────────────────────────────────────┘

  ---
  Billing Module

  ┌──────────────────────────────┬────────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────┐
  │            Route             │        Page        │                                          Key Content                                           │
  ├──────────────────────────────┼────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/billing               │ Billing Overview   │ Receivables summary, recent payments, flagged discrepancies, reconciliation status             │
  ├──────────────────────────────┼────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/billing/invoices      │ Invoices           │ Invoice list with state filter (draft/issued/paid/voided/written_off); create invoice          │
  ├──────────────────────────────┼────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/billing/invoices/[id] │ Invoice Detail     │ Line items editor, issue/void/write-off actions, payments history, refunds; role-gated actions │
  ├──────────────────────────────┼────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/billing/ledger        │ Ledger Events      │ Append-only event log; filter by type, date, household                                         │
  ├──────────────────────────────┼────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/billing/stripe        │ Stripe Integration │ Reconciliation report (ledger vs Stripe balance); webhook event log; alert threshold status    │
  └──────────────────────────────┴────────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Communications Module

  ┌─────────────────────────────────┬─────────────────────┬──────────────────────────────────────────────────────────────────────────────────────────────┐
  │              Route              │        Page         │                                         Key Content                                          │
  ├─────────────────────────────────┼─────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/comms/announcements      │ Announcements       │ List with draft/published/archived states; create new                                        │
  ├─────────────────────────────────┼─────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/comms/announcements/[id] │ Announcement Detail │ Rich text content; publish/archive controls                                                  │
  ├─────────────────────────────────┼─────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/comms/campaigns          │ Email Campaigns     │ Campaign list with delivery stats (sent_count/failed/suppressed); create draft               │
  ├─────────────────────────────────┼─────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/comms/campaigns/[id]     │ Campaign Detail     │ Recipient config, template, delivery log, send controls; re-send guard (409 if already sent) │
  ├─────────────────────────────────┼─────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/comms/suppression        │ Suppression List    │ Suppressed addresses table; add/remove; reason/date recorded                                 │
  └─────────────────────────────────┴─────────────────────┴──────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Audit & Compliance Module

  ┌──────────────────────────────────┬────────────────────────┬─────────────────────────────────────────────────────────────────────────────┐
  │              Route               │          Page          │                                 Key Content                                 │
  ├──────────────────────────────────┼────────────────────────┼─────────────────────────────────────────────────────────────────────────────┤
  │ /admin/audit                     │ Audit Event Log        │ Filterable by actor/action/resource/time range; cursor-paginated; immutable │
  ├──────────────────────────────────┼────────────────────────┼─────────────────────────────────────────────────────────────────────────────┤
  │ /admin/compliance/data-exports   │ Data Export Requests   │ Request queue with status; create/track                                     │
  ├──────────────────────────────────┼────────────────────────┼─────────────────────────────────────────────────────────────────────────────┤
  │ /admin/compliance/data-deletions │ Data Deletion Requests │ Deletion request queue with review countdown; approve/reject                │
  └──────────────────────────────────┴────────────────────────┴─────────────────────────────────────────────────────────────────────────────┘

  ---
  Jobs & System Module

  ┌───────────────────────┬───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────┐
  │         Route         │       Page        │                                       Key Content                                       │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/jobs           │ Job Queue         │ All tasks across queues (imports/billing/email/reconcile/default), filterable by status │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/jobs/queues    │ Queue Stats       │ Per-queue depth, throughput, error rate                                                 │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/jobs/metrics   │ Job Metrics       │ Type counters, rolling execution history, latency                                       │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/jobs/dlq       │ Dead Letter Queue │ Failed tasks; retry/discard per task                                                    │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/settings/roles │ Role Management   │ User → role assignments; MFA enforcement status per user                                │
  ├───────────────────────┼───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────┤
  │ /admin/settings       │ System Settings   │ Org config, env flags (AUTH_ENABLE_DEV_STUB_TOKENS, retention days, worker intervals)   │
  └───────────────────────┴───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Reports Module

  ┌───────────────────────────────┬───────────────────────┬───────────────────────────────────────────────────────────────────────┐
  │             Route             │         Page          │                              Key Content                              │
  ├───────────────────────────────┼───────────────────────┼───────────────────────────────────────────────────────────────────────┤
  │ /admin/reports                │ Reports Hub           │ Report cards linking to each report type                              │
  ├───────────────────────────────┼───────────────────────┼───────────────────────────────────────────────────────────────────────┤
  │ /admin/reports/membership     │ Membership Report     │ Active/inactive/aged-out breakdown; group distribution; CSV export    │
  ├───────────────────────────────┼───────────────────────┼───────────────────────────────────────────────────────────────────────┤
  │ /admin/reports/meet-fees      │ Meet Fees Report      │ Per-meet fee totals; swimmer-level breakdown                          │
  ├───────────────────────────────┼───────────────────────┼───────────────────────────────────────────────────────────────────────┤
  │ /admin/reports/billing-status │ Billing Status Report │ Receivables aging; invoice state distribution; household balance list │
  ├───────────────────────────────┼───────────────────────┼───────────────────────────────────────────────────────────────────────┤
  │ /admin/reports/attendance     │ Attendance Report     │ Practice/meet attendance rates; swimmer-level view                    │
  └───────────────────────────────┴───────────────────────┴───────────────────────────────────────────────────────────────────────┘

  ---
  Portal 2: Parent (/portal/*)

  Role: parent

  Navigation Modules

  Home · Household · Meets · Registration · Billing · Announcements · Documents · Calendar

  ---

  ┌─────────────────────────────────┬─────────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │              Route              │        Page         │                                                Key Content                                                 │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal                         │ Parent Dashboard    │ Alert cards (unpaid invoices, commitment deadlines, missing documents), swimmer quick-view, upcoming meets │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/household               │ Household Profile   │ Contacts, emergency details, edit allowed fields                                                           │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/household/swimmers/[id] │ Swimmer Profile     │ Editable within allowed fields; group/registration status; read-only best times                            │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/registrations           │ Registration Center │ Active registrations, enrollment status, waiver status per swimmer                                         │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/registrations/new       │ New Registration    │ Program selection → waiver acknowledgment → confirmation wizard                                            │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/meets                   │ Meet Center         │ Upcoming meets with commitment deadlines; status per swimmer                                               │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/meets/[id]              │ Meet Commitment     │ Commit yes/no per swimmer; event selection per swimmer (after yes); view entry status                      │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/billing                 │ Billing Center      │ Invoice list, balance summary, payment history, credits/refunds visibility                                 │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/billing/invoices/[id]   │ Invoice Detail      │ Line items, pay button → Stripe Checkout handoff, payment receipts                                         │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/announcements           │ Announcement Feed   │ Published announcements, read/unread state                                                                 │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/communications          │ Communications      │ Message history; notification preference controls                                                          │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/documents               │ Document Center     │ Required forms list; upload/status tracking; expiry alerts                                                 │
  ├─────────────────────────────────┼─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /portal/calendar                │ Calendar            │ Practices, meets, key deadlines in a month/week view                                                       │
  └─────────────────────────────────┴─────────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Portal 3: Swimmer (/swim/*)

  Role: swimmer (optional — disabled globally or per household)

  ┌───────────────────┬───────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │       Route       │       Page        │                                            Key Content                                             │
  ├───────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /swim             │ Swimmer Dashboard │ Schedule, announcements, upcoming meet status, commitment summary                                  │
  ├───────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /swim/performance │ Performance       │ Best times by course/event, historical progression chart, time standard / cut qualification status │
  ├───────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /swim/profile     │ Profile           │ Read-only (limited edits if policy allows)                                                         │
  ├───────────────────┼───────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ /swim/schedule    │ Schedule          │ Practices and upcoming meets                                                                       │
  └───────────────────┴───────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────────┘

  ---
  Shared / Auth Routes

  ┌────────────────┬────────────────────────────────────────────────┐
  │     Route      │                      Page                      │
  ├────────────────┼────────────────────────────────────────────────┤
  │ /login         │ OIDC login entry point (redirects to Keycloak) │
  ├────────────────┼────────────────────────────────────────────────┤
  │ /auth/callback │ OIDC callback handler                          │
  ├────────────────┼────────────────────────────────────────────────┤
  │ /mfa           │ MFA challenge (TOTP / backup code)             │
  ├────────────────┼────────────────────────────────────────────────┤
  │ /logout        │ Session revoke + redirect                      │
  ├────────────────┼────────────────────────────────────────────────┤
  │ /unauthorized  │ 403 page                                       │
  └────────────────┴────────────────────────────────────────────────┘

  ---
  Page Count Summary

  ┌────────────────┬─────────────────┐
  │     Portal     │      Pages      │
  ├────────────────┼─────────────────┤
  │ Admin/Staff    │ ~40 pages/views │
  ├────────────────┼─────────────────┤
  │ Parent Portal  │ ~13 pages/views │
  ├────────────────┼─────────────────┤
  │ Swimmer Portal │ ~4 pages/views  │
  ├────────────────┼─────────────────┤
  │ Auth/Shared    │ ~5 routes       │
  ├────────────────┼─────────────────┤
  │ Total          │ ~62 pages/views │
  └────────────────┴─────────────────┘

