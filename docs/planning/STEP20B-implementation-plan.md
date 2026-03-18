# STEP 20B Implementation Plan

## Goal
Build the remaining admin portal screens: Ledger, Announcements (list + detail), Email Campaigns (list + detail), Audit Log, Background Jobs, and Reports.

## Nav Paths (from roles.ts)
- `/ledger` → Billing > Ledger (owner/admin/billing)
- `/announcements` → Communications > Announcements (owner/admin/coach/read-only)
- `/campaigns` → Communications > Email Campaigns (owner/admin/read-only)
- `/audit` → System > Audit Log (owner/admin)
- `/jobs` → System > Background Jobs (owner/admin)
- `/reports` → System > Reports (owner/admin/billing/read-only)

## Pages to Build

### 1. `/ledger/page.tsx` — Billing Ledger
- Stats: Total Credits, Total Debits, Net Balance, Total Events
- Filters: event type (invoice_issued/payment_succeeded/refund_processed/invoice_voided/invoice_written_off), date range, household search
- Dense append-only table: Timestamp, Event Type (badge), Household, Description, Debit, Credit, Running Balance
- Color-coded: green credits, red debits

### 2. `/announcements/page.tsx` — Announcements List
- Stats: Published, Draft, Archived, Total
- Filters: status, search by title
- Table: Title, Status, Author, Pinned, Published At, Audience, View
- Create new announcement button

### 3. `/announcements/[id]/page.tsx` — Announcement Detail
- Header: Title, status badge, author, dates
- Actions: Publish (from draft), Archive (from draft/published)
- Tabs: Content, Settings (audience, pinned flag)

### 4. `/campaigns/page.tsx` — Email Campaigns List
- Stats: Total, Sent, Failed, Suppressed Total
- Filters: status (draft/sending/sent/failed)
- Table: Subject, Status, Sender, Recipients, Delivered %, Sent At, View
- Create draft button

### 5. `/campaigns/[id]/page.tsx` — Campaign Detail
- Header: Subject, status, from email, created at
- Stats: Total Recipients, Sent, Failed, Suppressed
- Delivery progress bar
- Tabs: Overview, Deliveries (per-recipient table)
- Actions: Send campaign (draft only)

### 6. `/audit/page.tsx` — Audit Log Explorer
- Filters: actor, action, resource_type, date range
- Dense log table: Timestamp, Actor, Action (color-coded badge), Resource Type, Resource ID, Details
- Cursor-paginated (mock: 20 entries)
- Export to CSV button

### 7. `/jobs/page.tsx` — Background Jobs
- Tabs: Queues, Dead-Letter Queue, Metrics
- Queues tab: 5 queue cards (imports/billing/email/reconcile/default) with active/pending/failed stats
- DLQ tab: table of failed tasks with retry/delete actions
- Metrics tab: task type counters + recent execution history

### 8. `/reports/page.tsx` — Reports Download
- Available report types with description, format badge, generate button
- Report types: Billing Summary, Member Roster, Meet Results Export, Time Standards, Audit Log Export, Season Registration Summary
- Generated reports history table: Name, Generated At, Size, Download, Expires

## Design Notes
- Follow established dark admin aesthetic (same as invoices/imports/meets pages)
- CSS variables: --color-bg, --color-bg-elevated, --color-accent, etc.
- Fonts: Instrument Serif for display values, Plus Jakarta Sans for body, JetBrains Mono for IDs/codes
- Status badges consistent with existing pages
- All mock data matches API types from CLAUDE.md service descriptions

## Status
- [x] Ledger page
- [x] Announcements list
- [x] Announcement detail
- [x] Campaigns list
- [x] Campaign detail
- [x] Audit log
- [x] Background jobs
- [x] Reports

## Completed 2026-03-17
