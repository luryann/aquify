# Mobile-First QA Checklist — Parent Portal Critical Flows

## Scope
Covers the three launch-critical parent flows that must work without a swimmer account.
Test on iOS Safari 17+ and Android Chrome 120+ at 375×812 (iPhone 14), 390×844 (iPhone 15), and 412×915 (Pixel 7).

---

## 1. Registration Flow (No Swimmer Account Required)

### Pre-conditions
- Parent account active, swimmer record exists (created by admin)
- No swimmer-role account exists for the swimmer

### Checklist
- [ ] `/portal/registrations` renders without layout overflow at 375px
- [ ] Swimmer accordion cards expand/collapse correctly with touch tap (not hover)
- [ ] Waiver status chips are readable (font size ≥ 11px, sufficient contrast)
- [ ] "Pending waiver" alert banner is visible above fold on mobile
- [ ] Waiver list scrolls without locking body scroll
- [ ] Registration season filter dropdown opens correctly on mobile
- [ ] USS status badges wrap correctly on narrow screens
- [ ] No horizontal scroll on main content at 375px
- [ ] CTA buttons are ≥ 44px touch target height
- [ ] All links navigate correctly in mobile browser (no JS errors)

---

## 2. Meet Commitment Flow (No Swimmer Account Required)

### Pre-conditions
- Meet in `commitment_open` state
- Commitment request sent to household

### Checklist
- [ ] `/portal/meets` urgency countdown visible on first screen load (above fold)
- [ ] Deadline urgency color coding renders correctly (red ≤3 days, amber ≤7 days)
- [ ] Per-swimmer commitment status chips wrap correctly
- [ ] "Respond" CTA is visible and tappable (≥ 44px)
- [ ] `/portal/meets/[id]` swimmer tab selection works with touch
- [ ] Step 1 (Yes/No commitment) buttons are tappable on narrow screens
- [ ] Step 2 event selection grid is scrollable horizontally if needed OR reflows to single column
- [ ] Best-time data in event grid is legible (font ≥ 11px)
- [ ] Coach override notice banner is visible if override exists
- [ ] Estimated fee summary renders without truncation
- [ ] Deadline urgency block is sticky or clearly visible

---

## 3. Payment Flow (No Swimmer Account Required)

### Pre-conditions
- Invoice in `issued` state with balance > 0

### Checklist
- [ ] `/portal/billing` outstanding balance banner visible above fold
- [ ] "Pay Now" CTA is ≥ 44px touch target
- [ ] Invoice list renders without truncation on 375px screen
- [ ] Invoice status chips are readable
- [ ] `/portal/billing/invoices/[id]` payment progress bar renders correctly
- [ ] "Pay with Stripe" button is visible and tappable
- [ ] Line items table scrolls or reflows (no hidden overflow)
- [ ] Receipt download links work in mobile browser
- [ ] Navigation back from invoice detail works correctly

---

## 4. General Portal QA

### Navigation
- [ ] PortalNav links are tappable at 375px (no overlap)
- [ ] User avatar menu opens and closes correctly with tap
- [ ] All 6 nav links visible or nav wraps gracefully
- [ ] Active state underline/highlight visible on current page

### Communications
- [ ] `/portal/communications` announcement list scrolls correctly
- [ ] "Mark read" button is tappable (≥ 44px)
- [ ] Tab switching (Inbox / Notifications) works with touch
- [ ] Toggle switches in notification preferences are tappable (≥ 44px)

### Calendar
- [ ] `/portal/calendar` month grid renders at correct aspect ratio
- [ ] Day cells are tappable and show selection state
- [ ] Event chips within cells are legible (font ≥ 9px acceptable for chips)
- [ ] Sidebar upcoming list reflows below calendar on narrow screens
- [ ] Month navigation arrows are ≥ 44px touch targets

### Documents
- [ ] `/portal/documents` category sections render correctly
- [ ] Sign/upload CTAs are tappable
- [ ] Expiry warnings display correctly

---

## 5. Policy Assertion

For each flow above, assert:
- The flow can be completed **start to finish** without ever logging in as or referencing a swimmer account
- If a swimmer account is absent, no error state is presented to the parent
- All data (best times, registration status, commitment options) is visible to the parent without swimmer portal access
