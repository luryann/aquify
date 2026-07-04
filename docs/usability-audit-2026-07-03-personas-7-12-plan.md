# Usability & Intuitiveness Audit — Personas 7–12 — Plan

Date: 2026-07-03
Continuation of the 2026-07-03 audit (`docs/usability-audit-2026-07-03-plan.md` /
`docs/usability-audit-2026-07-03-findings.md`, personas 1–6). This pass covers personas 7–12.

Scope: **UX/intuitiveness only.** Not hunting bugs, logic errors, or backend correctness. If
something blocks a flow entirely, note it once and move on — do not debug. The "7 missing
features" referenced in this session's original request are the ones already identified in the
prior findings doc (heat sheets, parent↔coach messaging, volunteer signup, structured emergency
contacts, per-swimmer attendance, recurring practice scheduling, self-service swimmer-link
recovery) — this pass does not build them, only continues the audit.

Guiding question for every screen: *"Would a non-technical parent/coach/swimmer understand this
immediately, or would they hesitate, get confused, or give up?"*

## Environment
- API: http://localhost:8080 (Go, dev stub tokens)
- Admin/portal: http://localhost:3000 (Next.js)
- Login: `/login` dev-stub quick-switch buttons (owner/admin/coach/parent/billing/read-only/swimmer)
- Reuse the Rivera/Chen household seed data from the prior pass (3 swimmers, issued invoice,
  best times, today-dated practices, one published announcement). Add seed data only if a
  persona's task is untestable without it (e.g., a second child in a different group, a second
  meet on the same day, a second household for finance-wide totals).

## Method
Same as prior pass: log in as the relevant dev-stub role, attempt each listed task cold, record
friction points (hesitation, unclear labels/icons, unexpected navigation, dead ends, no
empty-state guidance, jargon), and blocked flows (noted, not debugged). Persona 11 is run with
an explicit "abandon after ~3 confused taps" patience budget. Persona 12 re-runs persona 1's
tasks narrating as a low-tech-literacy user, flagging icon-only buttons, jargon, and missing
progress indicators.

## Personas & task checklists

7. **Parent with multiple kids in different groups/meets** — see all kids' schedules for the same
   day in one place; tell which balance/payment belongs to which kid; message each kid's coach
   without mixing threads (expect this to hit the already-known no-messaging gap — note how the
   absence itself is discovered/surfaced, not just that it's absent).
8. **New/prospective family, no account** — find and complete signup/registration cold, no help;
   figure out what happens next (approval? payment? waitlist?) without asking anyone.
9. **Assistant coach, limited permissions** — try every nav item a head coach has; note which are
   blocked and whether it's an explained locked/grayed-out state vs. silently missing/erroring; do
   core attendance/practice-set tasks to confirm they still work at this permission level.
10. **Admin/board member, finance-only** — find total outstanding dues across the whole team;
    find/export something resembling an invoice or payment history for one family; find where a
    refund would be issued/processed.
11. **Parent volunteer at meet, phone, deck-side, low patience** — find the heat sheet/running
    order for the current session; find where times/results would be entered/confirmed live;
    abandon and note the exact tap where a real user would give up (max ~3 taps of confusion).
12. **Low-tech-literacy parent (persona 1 tasks re-run)** — re-run persona 1's checklist (first
    login, find heat/lane, arrival time, balance, message coach, update emergency contact)
    narrating reactions as someone who doesn't use apps much; flag icon-only buttons, unexplained
    jargon (splits, DQ, seed time, heat, lane, scratch, etc.), and multi-step flows with no visible
    progress indicator.

Also (repeated from prior pass, extended if new instances found):
- **Missing features** vs. TeamUnify/SportsEngine norms — confirm/extend the existing list of 7,
  don't duplicate.
- **Dead weight** — screens/features adding clutter without serving personas 7–12.

## Output
Findings doc: `docs/usability-audit-2026-07-03-personas-7-12-findings.md`, organized by persona,
then Missing Features / Dead Weight addenda. Written findings only — no code fixes.
