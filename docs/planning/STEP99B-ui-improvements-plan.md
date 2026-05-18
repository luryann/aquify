# STEP 99B — UI Improvements Plan

## Tasks

### 1. Login page redesign
- Remove the left marketing/visual panel entirely
- Centered minimal box on dark background
- Keep: dark theme, email field, SSO button, dev stub section
- Remove: sidebar features list, wordmark panel, diagonal lane animations

### 2. Separate landing page (empty/barebones)
- Root `/` currently redirects to `/login` via server-side redirect
- Change `admin/app/page.tsx` to render a barebones landing page (client component)
- Landing page: DARE logo/wordmark, brief tagline, "Sign In →" button
- Simple, dark, no content — placeholder for future marketing content

### 3. Performance visualizations on dashboard
- Import `fetchPerformanceBestTimes`, `fetchPerformanceProgression` on dashboard
- Add a "Performance Snapshot" section below existing content:
  - Top events by swimmer coverage (horizontal mini-bars)
  - Top swimmers by best time count
- Fetch in parallel with existing dashboard data fetches

### 4. Entries — "Add Entry for Swimmer"
- Add "Add Entry" button in entries page header
- Opens a right-side slide-over panel:
  - Step 1: select swimmer (dropdown from all swimmers)
  - Step 2: select meet (dropdown, only ENTRY_STATUSES meets)
  - Step 3: event grid (checkbox for each event in that meet)
  - Save button — calls API or shows confirmation
- The panel also allows editing existing entries when triggered from a swimmer row

### 5. Commitments — Swimmer drill-down with event selection
- Make swimmer name in commitments table open a full-screen panel (not navigate away)
- Panel shows:
  - Swimmer header (name, age, household)
  - Commitment status (yes/no badges with override ability)
  - Note from swimmer (if any)
  - Event selection grid (parent-portal-like UI) with all meet events
  - Entries currently assigned to this swimmer for this meet
  - Actions: Save changes, Lock/Approve, Close

### 6. Commitment notes (portal + admin)
- In `portal/meets/[id]/page.tsx`: Add textarea "Message for coaches (optional)" in Step 1 commitment section
- Pass `note` field in commitment respond API call body
- Update `CommitmentRecord` type to include optional `note?: string`
- In commitments drill-down panel (Task 5): show swimmer note if present

## Implementation Order
1. Login page (simple, independent)
2. Landing page (simple, independent)
3. Dashboard performance section (medium complexity)
4. Portal commitment notes (add textarea to existing page)
5. Commitments swimmer drill-down (most complex)
6. Entries add-entry panel (complex, can reuse event grid from #5)

## Status
- [x] Login page redesign
- [x] Landing page
- [x] Dashboard performance section
- [x] Portal commitment notes
- [x] Commitments swimmer drill-down
- [x] Entries add-entry panel
