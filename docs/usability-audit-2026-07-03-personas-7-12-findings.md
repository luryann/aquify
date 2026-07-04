# Usability & Intuitiveness Audit — Personas 7–12 — Findings

Date: 2026-07-03
Continuation of `docs/usability-audit-2026-07-03-findings.md` (personas 1–6). Method: see
`docs/usability-audit-2026-07-03-personas-7-12-plan.md`. Tested live via dev-stub logins against
`localhost:3000` (admin/portal) + `localhost:8080` (API) + local Keycloak, reusing the Rivera/Chen
household seed data (Sofia Chen 12, Diego Rivera 15, Mia Rivera 12). **Scope is UX/intuitiveness
only** — nothing below was debugged at the code level.

Guiding question: *would a non-technical parent/coach/swimmer understand this immediately, or
would they hesitate, get confused, or give up?*

---

## Important correction to the 2026-07-03 "7 missing features" list

Two of the seven items the prior pass listed as **not built** are actually built and working today.
Whoever asked "what are the 7 missing TU features" should get this update before scoping any build
work — building these again would be wasted effort:

- **Per-swimmer attendance tracking** ("missing feature" #5 in the prior doc) — **actually exists.**
  `/training` → "Take" on a workout row opens a per-swimmer roster modal (`Diego Rivera — Here /
  Excused / Absent`) with a live "X of Y marked" progress count and instant save. This is good,
  touch-friendly UX, not the "single typed headcount" the prior pass described — that description
  was likely based on the yardage/summary row, not the modal behind "Take."
- **Structured emergency contact fields** ("missing feature" #4) — **actually exists.** Swimmer
  Profile → Parent-Editable Fields → Emergency Contact now has separate `Contact name` /
  `Relationship` / `Phone number` inputs plus a distinct `Allergies` field — not a single freeform
  box. The scary "Swimmer activity service is not configured" banner from the prior pass is also
  gone; club-managed fields (name, DOB, age, gender) render correctly instead of "—".

The remaining five items from that list (heat sheets, parent↔coach messaging, volunteer/fundraiser
signup, recurring/bulk practice scheduling, self-service swimmer-link recovery) were **not**
re-verified as fixed in this pass — see confirmations below where they came up again under
personas 7–12.

---

## Persona 7 — Parent with multiple kids in different groups/meets

- **See all kids' schedules for the same day in one place**: partially works. `/portal/calendar`
  correctly aggregates every event across the whole household onto one month view — a real
  strength, this is the "one place" the persona wants. But on a day with two practices (Fri, Jul 3),
  the day-detail panel shows only *"Practice — AM aerobic + IM sets"* and *"Practice — Stroke
  technique + kick sets"* — neither entry names a swimmer or even a training group. With Sofia/Mia
  (Age Group 10&U) and Diego (Senior Squad) on different schedules, a parent looking at this list
  cannot tell *whose* practice each row belongs to. The single-calendar-view idea is right; the
  entries just need a swimmer/group tag.
- **Tell which balance/payment belongs to which kid**: **does not work at all.** Billing
  (`/portal/billing` and the invoice detail page) shows one household-level invoice ("August dues,
  $185.00") with **zero line items** ("No line items found" on the invoice detail). There is no
  way, anywhere in Billing, to see that this charge is (for example) $65 for Sofia + $60 for Diego +
  $60 for Mia. A parent with three kids has no way to know which child's fees make up the total, or
  to spot-check that all three were actually billed.
- **Message each kid's coach separately**: still not possible — Communications remains one-way
  (club → member) announcements only; confirms the prior pass's finding, no messaging surface
  exists for any persona.

## Persona 8 — New/prospective family, no account

- **Find and complete the signup/registration flow cold**: reasonably discoverable. The sign-in
  screen has a clearly labeled "No account? Request access →" link (not buried), leading to
  `/register`, a clean single-page form (name, email, "I am a… Parent/Guardian, Coach, Admin
  Staff", optional club code, optional note). Submission gives an immediate, clear confirmation:
  *"Your access request has been submitted to your club administrator. You'll receive an email at
  [address] when your account is approved."* This closes the loop on "what happens next" for the
  account-approval step specifically.
- **Figure out what happens next (approval? payment? waitlist?)**: only half-answered. The flow
  is explicit about account approval timing, but it **never asks about the swimmer/child at all** —
  no child name, no age/group, no season registration, no waiver, no payment. A prospective family
  finishing this form would reasonably assume they've registered their kid for the team, when
  they've actually only requested a login. Compared to TeamUnify norms (where "registration" bundles
  the account, the athlete, a waiver, and often payment into one flow), this is a narrower flow than
  the label "Request access" implies, and the confirmation copy doesn't set that expectation
  ("you'll get an account" — silent on what happens to actually get the kid swimming).

## Persona 9 — Assistant coach, limited permissions

- There is **no distinct "assistant coach" role** — the role model (`owner > admin > coach >
  billing > parent > swimmer > read-only`) has one coach tier. This pass used that "Coach" stub.
- **Blocked vs. explained**: nav items outside a role's `allowedRoles` (confirmed in
  `admin/lib/roles.ts`) are **simply absent from the sidebar**, not shown grayed-out with an
  explanation. A Coach's sidebar has Households/Swimmers/Groups/Registrations/Meets/Commitments/
  Entries/Training/Time Standards/Best Times/Visualization/Imports/Meet Reports/Announcements —
  Billing, Documents, Compliance, Users/Settings, and Seasons are not there at all. This matches
  the "silently missing vs. locked-with-explanation" failure mode the persona asks about: there is
  no visual cue *why* a coach doesn't see Billing, only its absence.
- One partial exception: on the Training schedule, a workout row belonging to a group outside the
  coach's own roster shows its "Take" attendance button **visually disabled** (grayed) rather than
  hidden — better than nothing, but still has no tooltip/explanation for *why* it's disabled.
- **Attendance/practice-set tasks at this permission level**: work correctly. Coach could open
  "Take Attendance" on their own group's workout and mark a swimmer "Here" with an instant, visible
  save — see the corrected finding above.

## Persona 10 — Admin/board member, finance-only concerns

- **Find total outstanding dues across the whole team**: works, but **not where you'd first look.**
  The main `/dashboard` "Open Invoices" tile shows **$0** — misleading, since the org has a real
  $185 outstanding. The correct figure ($185.00 "Outstanding") is on `/billing` (Billing Overview),
  one click deeper via the Billing nav section. A finance board member landing on the main
  dashboard first (the natural first stop) would see $0 and could reasonably conclude there's
  nothing owed — this is the same "screen shows false/inconsistent data" risk the prior pass
  flagged as the single biggest trust problem in the product, still present here on the admin
  Dashboard tile specifically.
- **Find/export an invoice or payment history for one family**: partially works. `/invoices` lists
  invoices with household name, status filters, and amounts (found "INV-2026-001, The Rivera
  Family, $185.00" easily). But there's **no visible export/download/print action on the invoice
  detail page itself** — the only CSV export mechanism found in the whole product lives on
  `/reports` under "Payment Reconciliation" / "Invoice Aging," a separate, more technical page
  ("Live Report Console," "artifact delivery enabled") not linked from the invoice screen. A board
  member asked to "get me this family's invoice" would not discover that path from the invoice
  itself. Also: the invoice detail page shows the raw internal UUID next to the household name
  (`47d6da05-e2c5-46ad-8526-c3382fe4f803 · The Rivera Family`) — a lower-priority but real jargon/
  clutter item on an admin-only page.
- **Issue or process a refund**: **no discoverable path found anywhere in the admin UI.** Checked
  the invoice detail's "Refunds" tab (shows a table with "No refunds have been recorded" and no
  action button), Billing Overview, and Billing → Stripe (a technical ledger-reconciliation page
  with a "Total Refunded $0.00" metric but no refund-issuing control). A board member's task ends
  in "I don't see how" — refunds appear to only be operable via direct Stripe dashboard access or a
  backend script, not from within Aquify's own admin screens.

## Persona 11 — Parent volunteer at a meet, phone, deck-side, low patience

Simulated a 3-tap patience budget starting from the dashboard, using the closest-fit role
(Read-Only staff, the typical access level for a non-coaching event volunteer):

- **Tap 1** — Meets nav (fine, clearly labeled).
- **Tap 2** — click the one listed meet ("Summer Invitational 2026") → **hard failure.** The page
  renders *"Unable to load meet — You are not allowed to list commitments."* This is a raw backend
  permission error, not a designed "you don't have access" state, and it fully blocks the Read-Only
  role from opening the one meet in the system. **This is the exact tap-3-and-abandon moment** the
  persona describes — except it happens at tap 2, and there's no recovery path shown (no "ask an
  admin," no link back). Noting as a blocking flow per audit scope, not debugging further.
- Separately (confirmed earlier via the parent-portal pass): the meet detail page's "Meet Files"
  card still says *"No meet files available yet"* — so even for a role that *can* open the meet,
  there is no heat sheet / running order to find, confirming the prior pass's finding #1 is still
  accurate.
- **Live results entry**: no such surface exists anywhere in the product; results only arrive via
  post-meet file import (SD3/CL2/HY3), not live in-app entry. Confirms this is still a gap, not
  newly investigated in depth this pass (out of scope to design a new feature here).

## Persona 12 — Low-tech-literacy parent (re-running Persona 1)

- **Icon-only affordances**: the ">" chevron button used to open a swimmer's profile from the
  dashboard swimmer cards has an accessible name ("View Mia Rivera") but **no visible text label**
  — just an arrow glyph. A low-tech user scanning the card may not register it as the way "in" to
  see more about their kid; the swimmer's name itself isn't a visible click target/link styling cue
  either. Everywhere else checked (portal top nav: Home/Meets/Billing/Documents/Communications/
  Calendar), labels are full words, which is good and should be the standard the rest of the app is
  held to.
- **Jargon**: "SCY" appears repeatedly as an unexplained badge on meet cards (dashboard, Meets list,
  meet detail) with no tooltip or expansion — a parent unfamiliar with swim-meet course codes
  (Short Course Yards) has no in-app way to learn what it means. "Prelims"/"Finals" appear
  unexplained too, though these are more commonly picked up by swim parents quickly.
- **Progress indicators**: the swimmer profile edit flow is a good example done right — "Fields
  marked with a pencil icon can be updated by you. Other fields are managed by club staff" is
  stated up front before the user has to guess, and the emergency contact form is now broken into
  named `Contact name` / `Relationship` / `Phone number` boxes (see correction above) instead of one
  ambiguous text field — a low-tech parent filling this out today would not be confused about what
  goes where, which is a meaningful improvement over the prior pass's finding.
- **Empty states**: `/portal/documents` is a good example — clear counts (0/0 Complete, 0/0 Action
  Needed), a plain-language empty message ("No documents required yet. Your coach will request
  documents when needed."), and a support contact fallback ("Contact office@aquify.club if you have
  questions"). This is the pattern the rest of the app's empty/error states should be held to,
  contrasted against Persona 11's raw permission-error dump above.
- **Balance clarity**: re-confirms Persona 7's billing finding — "$185.00 outstanding" is clear in
  isolation, but a low-tech parent with three kids has no way to see what it's *for*, beyond the
  single label "August dues."

---

## Missing features vs. TeamUnify/SportsEngine norms (delta over the prior list)

The prior list of 7 is now a list of **5** confirmed-still-missing plus 2 corrected as already built
(see top of this doc). No new missing-feature categories were found in this pass beyond what
personas 7–11 surfaced as instances of the existing 5:

1. Heat sheets / heat-lane-time lookup — still absent (Persona 11).
2. In-app parent↔coach messaging — still absent, and now also confirmed as a blocker for
   multi-kid households needing separate per-kid threads (Persona 7).
3. Volunteer sign-up / fundraiser participation — not re-tested this pass, no new evidence either
   way; carrying over from the prior finding.
4. Recurring/bulk practice scheduling — not re-tested this pass; carrying over.
5. Self-service recovery when a swimmer login isn't linked to a swimmer record — not re-tested this
   pass; carrying over.

New, not on the prior list:
6. **Per-invoice line-item breakdown by swimmer** — billing is entirely household-level with no way
   to attribute charges to a specific child (Persona 7, Persona 10, Persona 12). This is a distinct
   gap from the two features above; TeamUnify-style billing typically itemizes by athlete/fee type.
7. **In-app refund issuance** — no refund action exists anywhere in the admin UI (Persona 10);
   Stripe-side refund evidence tooling exists per `CLAUDE.md`/Makefile targets, but there's no
   corresponding UI control a board member could use directly.

## Dead weight / clutter

- No new dead-weight screens identified in this pass. The Reports "Live Report Console" wording
  ("artifact delivery enabled," row counts as the primary metric) reads as more for engineers
  validating the reporting pipeline than for a finance user glancing at totals — not dead weight
  exactly, but its target audience doesn't match a board member's actual need for a headline dollar
  figure, which lives one page over on Billing Overview instead.

---

## Summary

The most consequential findings this pass: (1) two features previously logged as missing —
per-swimmer attendance and structured emergency contact fields — are actually built and working
well, so the real remaining gap list is shorter than previously scoped; (2) billing has no
per-swimmer itemization at all, a real problem for any multi-child household trying to understand
what they owe; (3) there is no refund-issuing control anywhere in the admin UI; (4) a Read-Only
staff role (the natural fit for a meet-day volunteer) cannot even open the one meet in the system
due to a raw permission error, which is a harder blocker than the still-missing heat-sheet feature
itself; and (5) the admin Dashboard's "Open Invoices: $0" tile repeats the same false-empty-state
trust problem flagged as P0 in the prior pass, on a screen a finance board member would check
first.
