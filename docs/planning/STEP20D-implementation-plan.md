# STEP 20D — Parent Portal Features Implementation Plan

## Objective
Build out all remaining parent portal screens for STEP 20D. These complete the parent-facing experience, allowing parents to manage their household without requiring a swimmer account.

## Aesthetic
- Light, warm ivory (#F7F4F0) background, contrasting dark admin
- Instrument Serif display font, Plus Jakarta Sans body, JetBrains Mono code
- Accent: #eb5d1e (orangey-red)
- Frosted glass nav bar (rgba(255,255,255,0.88) + backdrop-filter: blur)
- Card-based layouts with staggered slide-up animations
- Soft shadows, rounded corners, color-coded urgency
- Inline CSS (no Tailwind classes) matching existing portal pages

## Pages to Build

| # | Route | File | Status |
|---|-------|------|--------|
| 1 | `/portal/household/swimmers/[id]` | `admin/app/(portal)/portal/household/swimmers/[id]/page.tsx` | TODO |
| 2 | `/portal/registrations` | `admin/app/(portal)/portal/registrations/page.tsx` | TODO |
| 3 | `/portal/meets` | `admin/app/(portal)/portal/meets/page.tsx` | TODO |
| 4 | `/portal/meets/[id]` | `admin/app/(portal)/portal/meets/[id]/page.tsx` | TODO |
| 5 | `/portal/billing` | `admin/app/(portal)/portal/billing/page.tsx` | TODO |
| 6 | `/portal/billing/invoices/[id]` | `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx` | TODO |
| 7 | `/portal/documents` | `admin/app/(portal)/portal/documents/page.tsx` | TODO |

## Key Design Decisions
- Swimmer profile: field-level restrictions; editable fields (emergency notes, jersey size, auth contact override); read-only (name, DOB, gender, USS#, group, registration status); performance tab with best times
- Registrations: per-swimmer enrollment status cards with waiver completion tracking
- Meets inbox: commitment status chips (requested/yes/no/closed) with deadline countdown urgency
- Meet detail: tabbed per-swimmer commitment → event selection workflow; eligibility hints per event
- Billing: balance summary + invoice list + payment history
- Invoice detail: line items + payment progress bar + Stripe pay CTA + receipt history
- Documents: required doc list with upload status, expiry alerts, missing callouts

## Progress
- [x] Swimmer profile `/portal/household/swimmers/[id]`
- [x] Registrations `/portal/registrations`
- [x] Meets list `/portal/meets`
- [x] Meet detail `/portal/meets/[id]`
- [x] Billing overview `/portal/billing`
- [x] Invoice detail `/portal/billing/invoices/[id]`
- [x] Document center `/portal/documents`
- [x] Update CLAUDE.md with new routes
- [x] Mark tasks complete in TODO.md

## Status: COMPLETE
