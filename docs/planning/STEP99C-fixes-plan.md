# STEP 99C — UI Fixes & Feature Completions

## Status: IN PROGRESS

## Tasks

### 1. Branding Fix — Aquify not DARE
- [x] layout.tsx: title `DARE Admin` → `Aquify`
- [x] landing page: wordmark/tagline/footer text → Aquify
- [x] login page: any DARE references
- NOTE: Go module path (`github.com/dareaquatics/aquify`) NOT changed — too invasive without a full repo rename. UI text only.

### 2. Add Shirt Size to Portal Settings
- [x] `/portal/household/swimmers/[id]/page.tsx` — add `shirt_size` editable field alongside jersey size

### 3. Sync Swimmer Account Fields to Parent Account
- Equipment/gear size visible in swim profile should also appear in parent portal swimmer view
- [x] Parent portal `/portal/household/swimmers/[id]/page.tsx` — add equipment size, shirt size fields

### 4. Admin Dashboard Improvements
- [x] Add "Upcoming Meets" section (next 3 meets by start date)
- [x] Add "Quick Actions" panel (links to common admin tasks)

### 5. Fix Broken Buttons — Add API mutations to adminData.ts + modal forms

Backend endpoints confirmed to exist:
- POST /members/households → createHousehold
- POST /members/swimmers → createSwimmer
- POST /groups → createGroup
- POST /registrations → createRegistration
- POST /meets → createMeet
- POST /invoices → createInvoice
- POST /imports/upload-url → requestUploadURL (then finalize)
- POST /meets/{meetID}/exports → generateExport

Buttons to fix:
- [x] New Household — slide-over form → POST /members/households
- [x] Add Swimmer — slide-over form → POST /members/swimmers
- [x] New Group — add button + form → POST /groups
- [x] New Registration — add button + form → POST /registrations
- [x] Meet Creator (meets/new/page.tsx) — wire handleSave → POST /meets
- [x] New Invoice — add button + form → POST /invoices
- [x] Upload File (imports page) — modal with file upload → POST /imports/upload-url
- [x] Export Button (meets detail page) — verify and wire → POST /meets/{id}/exports
- [x] New Announcement/Campaign/Add Address — verify these work (they have real API calls already)

### 6. New User Approvals Screen
- [x] Create `/admin/app/(admin)/members/approvals/page.tsx`
- Shows households with `pending` status
- Allows bulk or individual approval/rejection
- Wire to PATCH /members/households/{id} (update status)
- Add nav link in admin layout

## Notes
- All new API functions added to adminData.ts
- Forms use slide-over pattern consistent with commitments/entries drill-down style
- Branding: "Aquify" (capitalized) used in display, "aquify" in code identifiers
