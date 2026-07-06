# Aquify UX / Content-Clarity Audit — 2026-07-03

Scope: every screen across admin/coach, parent portal, swimmer portal, and public/auth surfaces.
Method: three parallel reviews, each asking one question per screen — **"Would a non-technical
parent/coach/swimmer understand this immediately, or would they hesitate, get confused, or give up?"**

This is not a bug-hunt or correctness review. Anything logically broken is noted only if it blocks
a flow; it is not root-caused here.

**Assume the user will not figure it out themselves, will not read instructions, and will not dig
around. If something is even mildly ambiguous, it's flagged.**

> Note: `docs/usability-audit-2026-07-03-*.md` (findings + plan, and a "personas 7-12" follow-up)
> already exist from an earlier pass the same day. That pass covered functional/data bugs (e.g. a
> P0 empty-state bug on billing/best-times/announcements). This document is a separate, UX/IA-only
> pass and doesn't re-litigate those — treat the two as complementary, not competing.

Status key for the plan section: 🟢 done · 🟡 approved, not started · ⚪ proposed, needs your go-ahead.

> **Update 2026-07-03 (later same day):** Tier 1 and Tier 2 are fully implemented. Tier 3 is now
> fully implemented as well: mobile navigation, Competition/Members/Billing/Communications/Users
> consolidation, and the Reports naming collision (resolved as a side effect of the Competition
> merge). Only Tier 4 remains — those need a product decision, not a UI fix.

---

## Already fixed this session (approved before the audit ran)

- 🟢 Removed the "● connected" status indicator from the admin topbar (`admin/components/layout/Topbar.tsx`) — a fake/decorative uptime dot that told users nothing actionable.
- 🟢 Removed the glowing accent dot next to the "aquify" wordmark in the expanded sidebar (`admin/components/layout/Sidebar.tsx`) — pure decoration, no meaning.

---

## Top-line findings (read this part first)

These are the highest-impact problems across the whole app, ranked by how many users they block
and how badly they'd damage trust in the product.

1. **No CSS media queries anywhere in the app, and no responsive nav.** `admin/globals.css` has
   zero `@media` rules (verified). One page (parent Calendar) does its own JS-based width check
   (`windowWidth < 720`) to reflow its sidebar below the calendar — proof the pattern is possible —
   but that trick isn't applied anywhere else, and none of the three main nav bars (admin `Sidebar`,
   portal `PortalNav`, swimmer `SwimmerNav`) collapse into a hamburger menu on narrow screens. Every
   persona in this audit (parent at the pool deck, coach on a deck-side phone, swimmer in a locker
   room) is explicitly phone-first. This is the single biggest structural risk in the app — bigger
   than any individual page's copy or layout.
2. **The competition workflow is the same four concepts wearing different names on different
   screens: Registrations, Commitments, Entries, Meets.** Admin has four separate top-level nav
   items for what a coach or parent experiences as one task ("get my kid signed up and swimming at
   the meet"). The *same underlying status* even uses different words screen to screen (admin
   `/commitments` says "Awaiting/Going/Not Going/Locked"; the meet-detail Commitments tab says
   "Invited/Committed Yes/Committed No/Locked"; the parent portal says "Going/Not Going/Locked").
3. **Billing is five separate nav destinations for one task.** Billing Overview, Invoices, Ledger,
   Autopay, and Stripe are all distinct pages. A volunteer treasurer chasing an overdue family has to
   know to check four of these plus a fifth (Stripe) that's pure engineering config.
4. **Dead buttons that look real.** Parent portal Household page has "Edit Profile" and "Add
   Contact" buttons, fully styled with hover states, that have no click handler at all. A rushed
   parent will click, get no response, and conclude the app is broken.
5. **Engineering/ops tooling sits in the primary nav next to club-admin tools, with no separation.**
   Background Jobs (worker queue/DLQ), Audit Log (raw JSON diffs), System Diagnostics (its own
   subtitle admits "not for club administration"), and Stripe's Configuration tab (raw env var
   names, webhook HMAC secrets) are all one sidebar click away from Settings/Billing for any
   owner/admin. None of these serve a club workflow.
6. **"Live data" QA language leaked into production copy everywhere.** Nearly every admin page
   subtitle says some variant of "Live records persisted via the API" / "backed by the live billing
   service." This is internal dev-testing language. At best it's noise; at worst a normal user reads
   it and wonders "wait, is some of the data *not* live?"
7. **A fabricated "System Status" card is the first thing every user sees on the dashboard** —
   hardcoded "API healthy / DB 16ms p95 / Storage MinIO / Email Postmark / Billing Stripe," always
   green, regardless of real state, using vendor names meaningless to a club admin.
8. **A fully-built Document Center (waiver/medical/insurance review) has no nav entry at all.**
   Owner/admin can't discover it except by URL. Meanwhile the Registrations page shows its own
   fake "Waiver: Signed/Pending" badge computed from an unrelated status field — two different,
   contradictory signals for the same real-world question ("do we have this family's waiver").
9. **The parent Registrations page and its 4-step "add a swimmer / new season" wizard are both
   fully built and completely unreachable from any nav or dashboard link.** A parent who needs to
   register for a new season has no discoverable path in.
10. **Brand mismatch at the worst possible moment.** The public registration page shows "DARE" as
    the product name (`register/page.tsx`) while every other page — login, root, both portals —
    says "aquify." A brand-new parent entering their child's info mid-signup sees a different
    company name and reasonably wonders if they've landed on a phishing page.
11. **"Reports" means two unrelated things.** Admin `/reports` (System section) is a CSV
    billing/membership report console. Admin `/exports` (Files section, labeled "Meet Reports" in
    nav) is a PDF per-meet report generator. Told to "pull the report," an admin has a coin-flip
    chance of picking the right page.
12. **The auth callback page shows raw backend implementation detail to end users** — literally
    "Exchanging token with Keycloak" — during every sign-in. Nobody outside engineering knows what
    Keycloak is.
13. **Email campaign composer ships with another club's identity hardcoded as the default.**
    `from_name: 'DARE Aquatics'`, `from_email: 'noreply@dareaquatics.org'`, and a body defaulting to
    "Hello from DARE Aquatics." Any admin who doesn't notice and overwrite this sends campaigns
    under a stranger club's name. HUMAN EDIT: Make this configurable in settings.
14. **Autopay card entry looks and behaves like a plain, unencrypted form** (typed "Card Brand,"
    "Last 4," "Expiry Month/Year" fields, no Stripe Elements widget) sitting right next to a
    one-time "Pay Now" flow that correctly redirects to real Stripe Checkout. The inconsistency
    reads as a trust/security downgrade for the flow parents will use most (autopay).
15. **Practice schedule can't show a time of day and isn't filtered to the swimmer's own group** —
    the underlying data model only has a `date` field, no start/end time, and the swimmer schedule
    page pulls every group's practices unfiltered. "What time is my practice" is the single most
    basic thing a swim-team app needs to answer and it currently can't.

---

## Full findings by surface

### A. Admin / coach app

**Navigation & information architecture**
- Meets / Commitments / Entries / Registrations are four top-level items that all describe the same
  underlying "did this swimmer sign up and get entered" story, with heavy data overlap (the Meets
  list already shows Commitments/Entries counts as columns).
- Billing / Invoices / Ledger / Autopay / Stripe: five destinations for "handle a family's payment."
  Ledger uses bookkeeper jargon ("credit/debit," "ledger event") with no plain-language framing.
- Document Center (`compliance/documents`) has no nav entry (see top-line #8).
- Jobs, Audit Log, System Diagnostics, Stripe→Configuration are pure engineering tooling reachable
  from the same sidebar as club-admin tasks, with no visual separation or warning beyond body copy.
- Reports vs. Meet Reports (Exports) naming collision (top-line #11).
- Nav label doesn't match the page's own on-screen heading in five places, breaking the mental
  model of "I clicked X, so this page is X": Entries → "Entry Approvals," Best Times → "Best Times
  Explorer," Reports → "Live Report Console," Settings → "Team Settings," Stripe → "Stripe
  Integration."
- Dead/orphaned redirect stub routes left over from a restructor: `dashboard/commitments`,
  `dashboard/entries`, `dashboard/performance/visualization`. Harmless but should be deleted, not
  left as phantom routes.

**Content / labeling**
- "Live data" QA boilerplate on nearly every page subtitle (top-line #6).
- Dashboard "System Status" card is fabricated and always green (top-line #7).
- Meet creation success shows a leftover engineering banner: "LIVE DATA ENABLED — The meet list now
  reloads from persisted API data" instead of a plain "Meet created."
- Registrations page's synthetic "Waiver: Signed/Pending" badge is derived from an unrelated status
  field, not the real document-verification record — can actively mislead an admin into thinking a
  waiver is on file when it isn't.
- Same underlying commitment status uses different words on different screens (top-line #2).
- Document Center's submission table shows raw truncated UUIDs for household/swimmer instead of
  names — inconsistent with the name-resolution used everywhere else in the app.
- Audit Log and Jobs are dev/support tooling, not "recent activity" for club staff — the Dashboard's
  plain-language "Recent Activity" feed already covers the real club-admin need.
- Email campaign composer defaults to another club's branding (top-line #13).
- Gender filter options are inconsistent between the Swimmers list filter ("Male/Female/Other or
  Undisclosed") and the Add Swimmer form ("Female/Male/Non-binary/Undisclosed").
- Best Times only shows one swimmer at a time via dropdown — no team-wide leaderboard for "who's
  fastest in the 100 free on the team," which is a coach's most common question for this screen.
- "Suppression list" (Communications) is email-deliverability jargon never translated to plain
  language ("addresses we'll never email again because they bounced or complained").
- Compliance pages (Data Exports/Data Deletions) correctly use GDPR terminology but never translate
  it to the actual trigger ("a parent asked you to download/delete their data — start here").
- Role Management requires a free-text "reason" for role changes with no explanation of why it's
  required.

**Mobile/legibility**
- Multiple screens use fixed pixel/fraction grid columns that won't reflow on a phone: Training,
  Autopay, Ledger, and most tables built on the shared `Table` component (fixed-width columns,
  dense 12–13px text).
- Sidebar resize handle is mouse-drag only, no touch affordance.

**Genuinely good, worth protecting**
- Sidebar's "locked" nav items (greyed out with "Requires a higher permission level" instead of
  hidden) — keep this pattern, don't regress it.
- Users page proactively explains "Users (logins) vs. Swimmers (roster)" — exactly the kind of
  disambiguation the rest of the app needs more of.
- New Meet wizard and meet-detail lifecycle bar use clear plain-language stage names.
- Stripe Reconciliation tab (not Configuration) explains mismatches in plain money terms.

---

### B. Parent portal

**Navigation & information architecture**
- Portal nav bar (7 links + logo + user menu) has no responsive handling — will clip or overflow on
  a phone (top-line #1).
- Registrations page + 4-step registration wizard: fully built, entirely unreachable (top-line #9).
- Messages (1:1 coach chat) and Communications (announcements + notification prefs) are separate
  nav items with no cross-linking and no guidance on which to use for what.
- Meet detail page crams commitment yes/no + event selection + meet-file downloads into one dense,
  non-responsive two-column layout (fixed 280px sidebar column, no breakpoint) — this is the page
  parents use to respond to invites in a hurry.

**Content / labeling**
- Dead "Edit Profile" and "Add Contact" buttons on Household page (top-line #4).
- Meet fee summary card is labeled "Total estimated entry fee" but displays an event count
  ("3 events"), not a dollar amount.
- Terminology drift across meet flows: "Commitment," "Entry," "Registration," "Going/Not
  Going/Locked" all describe adjacent-but-different things with no glossary or tooltip.
- Autopay setup looks like a plain unencrypted form next to a real Stripe Checkout one-time-pay flow
  (top-line #14).
- Dashboard's per-meet commitment badge can show "Committed" for the whole household even when one
  of two swimmers still needs to respond — silently hides a required action.
- Best Times table uses SCY/SCM/LCM course-code jargon with no legend/tooltip.
- "Download PDF" button on the invoice Receipts tab has no visible handler wired — worth confirming
  it's implemented, since it reads the same way the Household dead buttons do.

**Mobile/legibility**
- Calendar has the best mobile handling in the app (sidebar correctly reflows below the calendar on
  narrow screens) but the day grid itself still forces a fixed ~770px width, so the grid itself
  still requires horizontal scrolling.
- Calendar's 6 type-filter chips will wrap to 2–3 rows on a phone, pushing real content below the
  fold.
- Invoice list is a dense 5-column row that will feel cramped on a phone.

**Genuinely good, worth protecting**
- Dashboard's "needs attention" triage pattern (alerts → swimmers → upcoming meets) is the right
  mental model for a rushed parent.
- Documents page (stats strip, action-needed banner, per-doc status, office-email fallback) is one
  of the best-designed pages in the whole app for this persona.
- Swimmer detail page's "Club-managed vs. Parent-editable" field split with an explanatory banner
  is exactly the right pattern to prevent confusion about locked fields.
- Meet detail's Step 1/Step 2 commit-then-pick-events flow, with "not yet open" / "locked by coach"
  explanatory states, is clear.

---

### C. Swimmer portal & public/auth pages

**Navigation & information architecture**
- Swimmer nav bar has the same no-breakpoint problem as the portal nav — will overflow on a phone,
  the primary device for this persona (top-line #1).
- Root landing page has only one CTA ("Sign in →"); the "Request access" signup link is one extra
  click away on `/login` rather than visible on the first screen a brand-new visitor sees.

**Content / labeling**
- Brand mismatch "DARE" vs. "aquify" on the registration page (top-line #10).
- Auth callback shows "Exchanging token with Keycloak" (top-line #12).
- No swimmer self-registration option exists — `register/page.tsx` only offers Parent/Guardian,
  Coach, or Admin Staff roles, despite a full swimmer portal existing. May be intentional
  (swimmers are meant to be invited via a parent's household) but worth confirming on purpose.
- Login page footer's Privacy / Terms / Support links all point to `href="#"` — dead links, exactly
  where a first-time signer-up entering a child's data would want reassurance.
- "Cut" jargon used with zero explanation in the meet event sign-up flow ("Cut 58.50," "meets
  cut"/"under cut") — the Best Times tab has a legend for AAA/AA/A/BB/B nearby, but the meet
  sign-up screen doesn't reuse or reference it.
- "Session-Split Signup" heading/subtitle on the swimmer meet-detail page is unexplained scheduling
  jargon.
- Duplicate-looking bio UI on the swimmer profile page: an editable "My Bio" textarea directly above
  a "Saved Bio" panel showing the same text post-save — reads like two separate fields.
- Practice schedule has no time-of-day and isn't filtered to the swimmer's own training group
  (top-line #15) — the data model itself needs a start/end time field to fix this properly, so this
  one is flagged but not solvable with a copy/layout change alone.
- Progression text ("at your current rate of improvement... approximately N more period(s)") uses
  dense analytical phrasing and an undefined unit ("period") unlikely to land with a young swimmer.
- Goal-time input requires exact format "58.50 or 1:58.00" with no input mask, just placeholder text.

**Genuinely good, worth protecting**
- Registration page's post-submit success screen clearly explains the approval flow and next steps.
- Unlinked-swimmer notice (dashboard) is a strong non-dead-end pattern — self-service notify-admin
  form instead of a wall.
- Auth callback's step checklist ("Verifying state token / Exchanging authorization code / Creating
  session / Loading workspace") gives good first-time reassurance something is happening (once the
  Keycloak name is fixed).
- MFA setup flow (3-step indicator, recommended apps, manual key fallback, backup codes) is clear
  for its target roles.
- Swimmer profile's read-only-field banner correctly explains why some fields are locked and where
  to go to change them (parent portal).

---

## Proposed fix plan

Grouped by effort/risk. Nothing below has been implemented yet except the two items already marked
🟢 above — **this whole section is for your review before I touch any code.**

Per your instruction, this session is UX/content-clarity only — not bug hunting or logic fixes. A
few things the agents found aren't UX issues at all, they're outright bugs or feature gaps (dead
buttons, fabricated status data, wrong-org defaults). I've pulled those into their own bucket below
so they don't get silently bundled into "UI cleanup." Everything in Tier 1 is genuinely copy/label/
dead-code-removal only — nothing that requires debugging or new logic.

### Bugs/gaps noticed, not fixed (your call — separate from this UX pass)
- Household "Edit Profile" and "Add Contact" buttons: confirmed no `onClick` handler at all — pure
  dead UI. (Verified: `household/page.tsx` L501-529, L761.)
- Invoice "Download PDF" button (Receipts tab): present but unverified whether wired — flagging for
  you to check, not confirmed dead the way the Household buttons are.
- Dashboard "System Status" card: confirmed hardcoded (`StatusRow label="Database" status="ok"
  detail="16ms p95"`, etc.) — not reading any real health signal. This is a fabricated-data bug, not
  a labeling issue.
- Campaign composer defaults: confirmed hardcoded to `'DARE Aquatics'` / `noreply@dareaquatics.org`
  — a real risk (wrong-club sends), not just confusing copy.
- Practice schedule has no start/end time field in the data model and isn't filtered to the
  swimmer's own group — a data-model gap, not fixable with a UI change alone.
- Parent invoice "Download PDF" and a few smaller "does this actually work" flags noted inline in
  the per-surface sections above.

If you want these fixed in this session, say so explicitly and I'll treat them as a separate
mini-punchlist; otherwise they stay as documented findings only.

### Tier 1 — safe, pure UX cleanup, do first (copy/labels/dead-code removal, no logic changes)
- 🟢 Remove the dead "Edit Profile" and "Add Contact" buttons (or grey them out with "coming soon")
  as decluttering — not fixing what they do, just not showing a button that does nothing.
  (Greyed out + disabled with an explanatory tooltip, rather than removed outright.)
- 🟢 Fix "DARE" → "aquify" brand name on the registration page (string swap only).
- 🟢 Replace "Exchanging token with Keycloak" with plain language ("Setting up your session…")
  (string swap only).
- 🟢 Fix the login footer's dead Privacy/Terms/Support links — point them at real pages if they
  exist, or remove the links until they do (no new pages built in this pass).
  (No real Privacy/Terms/Support pages exist, so the links were removed.)
- 🟢 Strip "live data"/"backed by the live API" QA boilerplate from every page subtitle (copy only).
  (Turned out to be far more widespread than originally scanned — fixed across all ~25 admin/portal
  pages that had it, not just the handful first flagged.)
- 🟢 Replace the meet-creation "LIVE DATA ENABLED" banner string with a plain "Meet created."
- 🟢 Delete the orphaned `dashboard/commitments`, `dashboard/entries`,
  `dashboard/performance/visualization` redirect-stub routes (dead route removal).
- 🟢 Reconcile the gender option lists between the Swimmers filter and Add Swimmer form so the same
  field offers the same choices in both places (copy/options list only).
  (Standardized on the backend's canonical vocabulary — female/male/non_binary/undisclosed — since
  the Add Swimmer form already matched it; the filter and table display were the ones that needed
  to change.)
- 🟢 Add a legend/tooltip for "Cut" terminology on the swimmer meet sign-up screen (reuse the
  existing AAA/AA/A/BB/B legend copy from Best Times — no new logic).
- 🟢 Rename mismatched nav-label-vs-heading pairs so they match: Entries/"Entry Approvals," Best
  Times/"Best Times Explorer," Reports/"Live Report Console," Settings/"Team Settings,"
  Stripe/"Stripe Integration" (pick one label per pair, string swap only).
  (Headings changed to match the nav label in every case.)

### Tier 2 — nav/IA changes (moving/adding links, no page redesign)
- 🟢 Add a nav entry for Document Center (Compliance section), and stop deriving the Registrations
  page's "Waiver" badge from an unrelated status field — point it at real document-verification
  data instead.
  (The registrations API already returns a real `waiver_accepted` field per registration — the
  admin Registrations page just wasn't using it. Now it does.)
- 🟢 Add the parent Registrations page + registration wizard to the portal nav (or link them
  prominently from the portal dashboard) so parents can actually find them.
- 🟢 Move Jobs, Audit Log, and System Diagnostics out of the primary owner/admin nav into a clearly
  separated "Advanced/Engineering" area (or gate them behind an explicit "show advanced tools"
  toggle) so they stop sitting next to everyday club-admin tasks.
  (Added a new "Advanced" nav section, distinct from "System," rendered last in the sidebar.)
- 🟢 Cross-link Messages ↔ Communications with a one-line "use this for..." hint on each, so parents
  know which to use.

### Tier 3 — page consolidation (bigger structural changes, want your explicit sign-off per group)
- 🟢 **Competition workflow**: folded Commitments, Entries, Imports, and Meet Reports (Exports) into
  the per-meet detail page (`/meets/[id]`) as additional tabs, in chronological order: Overview →
  Sessions → Events → Commitments → Entries → Import History → Reports. The Commitments and Entries
  tabs keep a lightweight read-only table plus a "Manage commitments/entries →" link into the full
  rich workflow (swimmer slide-over panel, event-selection toggling, entry approve/reject/scratch,
  lock-all) pre-filtered to that meet via `?meet=<id>` — that workflow was too substantial to safely
  duplicate inline, so it stays a dedicated page but is no longer in primary nav and always arrives
  pre-scoped to one meet. Import History is now read-only (starting new imports happens from the
  meet-creation flow, per your call); Reports is a fully self-contained per-meet generate/list panel.
  Top-level Commitments/Entries pages are reachable only via meet-detail deep links now; top-level
  Imports/Exports pages are redirect stubs to `/meets` (their functionality fully moved into
  meet-detail tabs). The `Files` nav section (now empty) was removed.
- 🟢 **Members consolidation**: merged Households, Swimmers, Groups, Registrations, and Document
  Center into a single `/members` page with tabs (Documents gated to owner/admin, matching its
  original access level). Approvals stayed a separate nav item as planned. All `[id]` detail routes
  (household/swimmer/group) are untouched; old list routes are redirect stubs to `/members?tab=X`.
- 🟢 **Billing consolidation**: merged Billing Overview, Invoices, Ledger, and Autopay into a single
  `/billing` page with tabs in that order; Stripe stayed split out as engineering-only. Tabs are
  role-gated per the original per-page access (read-only sees only Invoices; Overview/Ledger/Autopay
  need owner/admin/billing). Old routes are redirect stubs to `/billing?tab=X`.
- 🟢 **Communications consolidation**: merged Email Campaigns and Suppression List into one
  `/campaigns` page with tabs (Suppression gated to owner/admin, matching its original access level).
- 🟢 **Users & Roles consolidation**: merged Users and Role Management into one `/users` page with
  tabs. Old `/settings/roles` route redirects to `/users?tab=roles`.
- 🟢 **Seasons re-homed**: moved the Seasons nav item from the System section to Performance (no page
  changes).
- 🟢 **Reports naming collision**: resolved as a side effect of the Competition consolidation above —
  per-meet PDF reports now live only inside the meet detail page's Reports tab, so "Reports" (System
  section) unambiguously means the CSV/membership report console.
- 🟢 **Mobile navigation**: add a responsive hamburger/collapse behavior to all three nav bars
  (admin Sidebar, PortalNav, SwimmerNav) — this is the highest-impact fix in the whole audit but
  also the most code, since there are currently zero `@media` queries anywhere in the app to build
  from.
  (Added one shared breakpoint (768px) and interaction pattern across all three: below it, the
  inline nav collapses behind a hamburger button that opens a left slide-over drawer with the same
  links stacked vertically. Admin's drawer reuses the existing `Sidebar` component as-is; the
  portal and swimmer drawers are per-nav since their headers/themes differ, but share the same
  CSS classes (`.mobile-nav-toggle`, `.mobile-nav-overlay`, `.mobile-nav-drawer`, `.mobile-only`,
  `.desktop-nav-links`) added to `globals.css`. Verified interactively via browser automation:
  drawer open/close, link navigation auto-closing the drawer, and clean revert to desktop layout.)

### Tier 4 — needs a product decision, not just a UI fix
- Practice schedule times: the underlying `PracticeRecord` model has no start/end time field, so
  showing "what time is my practice" requires a data-model change, not just a frontend fix. Flagging
  for your call on scope/priority.
- Swimmer self-registration: confirm whether swimmers are intentionally required to go through a
  parent's household (if so, no change needed — but the registration page's role picker doesn't
  currently explain that constraint anywhere).

---

## What I'd suggest doing next
Confirm which tiers you want me to execute, and in what order. My recommendation: Tier 1 in full
(it's all low-risk cleanup), then Tier 2, then come back to discuss Tier 3 page-by-page since
merging Billing or the competition workflow touches a lot of surface area and I'd want to walk
through the target layout with you before writing code.
