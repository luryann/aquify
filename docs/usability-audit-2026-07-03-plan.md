# Usability & Intuitiveness Audit — Plan

Date: 2026-07-03
Scope: **UX/intuitiveness only.** Not hunting bugs, logic errors, or backend correctness. If something
blocks a flow entirely, note it once and move on — do not debug.

Guiding question for every screen: *"Would a non-technical parent/coach/swimmer understand this
immediately, or would they hesitate, get confused, or give up?"*

## Environment
- API: http://localhost:8080 (Go, dev stub tokens enabled via `.env.local`)
- Admin/portal: http://localhost:3000 (Next.js)
- Login: `/login` dev-stub quick-switch buttons (owner/admin/coach/parent/billing/read-only/swimmer)
- Seed data added for this session (SQL, not frontend mocks): Summer Invitational 2026 session 1
  moved to today/tomorrow with times, invoice INV-2026-001 issued with a real balance due,
  best times for 3 household swimmers, two today-dated practices (Senior Squad, Age Group 10&U),
  one published team-wide announcement.
- Household under test: Rivera/Chen household (3 swimmers: Mia, Diego, Sofia), linked to the
  `stub-parent` and `stub-swimmer` dev logins.

## Method
For each persona: log in as the relevant dev-stub role, attempt each listed task cold (as if
seeing the product for the first time), and record for each step:
- **Friction points**: hesitation, unclear labels/icons, unexpected navigation, dead ends, no
  empty-state guidance, confirmation-less destructive actions, jargon a parent wouldn't know.
- **Blocked flows**: only noted, not debugged.
- Screenshot or note of anything surprising.

Then a separate pass on:
- **Settings pages** (all roles) — is there real, meaningful content, and would a regular user
  understand what each setting does?
- **Missing features** a parent/coach would expect from TeamUnify/SportsEngine norms.
- **Dead weight** — screens/features that add clutter without persona value.

## Personas & task checklists

1. **Parent, first-time, on phone, meet in 20 min** — log in first time; find kid's heat/lane/event
   for today's meet; find arrival time; check outstanding balance; message a coach; update kid's
   allergy/emergency contact.
2. **Parent, returning, at home** — register kid for upcoming meet; check best times/season progress;
   find team calendar (next 2 weeks); opt in/out of fundraiser or volunteer slot.
3. **Coach, mid-practice, phone/tablet** — mark attendance for a group in <30s; view/update today's
   practice set; message a group of parents; look up a swimmer's recent times.
4. **Coach, at a desk, planning** — build/edit a meet roster/lineup; enter results after a meet;
   set up next month's practice schedule.
5. **Swimmer (young teen), on phone** — check own times/PBs; check today's practice group/set; see
   upcoming meets signed up for.
6. **Admin, light pass** — add a new family/swimmer; send a team-wide announcement; check
   payment/dues status across the team.

## Output
Single findings doc: `docs/usability-audit-2026-07-03-findings.md`, organized by persona, then
Settings review, then Missing Features / Dead Weight sections. Written findings only — no code
fixes as part of this pass.
