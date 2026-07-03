# Usability & Intuitiveness Audit — Findings

Date: 2026-07-03
Method: see `docs/usability-audit-2026-07-03-plan.md`. Tested live via dev-stub logins against
`localhost:3000` (admin/portal) + `localhost:8080` (API), with real Postgres seed data added for
this pass (no frontend mocks). **Scope is UX/intuitiveness only** — nothing below was debugged at
the code level; root causes are named only where obvious from the outside.

Guiding question: *would a non-technical parent/coach/swimmer understand this immediately, or
would they hesitate, get confused, or give up?*

---

## Cross-cutting issue (affects almost every persona)

> ✅ **FIXED (2026-07-03 follow-up).** Root cause found: `loadRuntimeSnapshots` in
> `server/internal/app/server.go` unconditionally restored a legacy JSON snapshot at API startup,
> silently overwriting the correct data each domain service had just loaded from Postgres. Fixed by
> skipping snapshot hydration whenever a DB pool backs the services. Verified live: Billing now shows
> the real $185 due, Best Times shows recorded swims, Announcements shows the published post (admin
> and portal both).

**Several "check status" screens show empty/zero state despite the data existing in the database.**
Confirmed independently in four places:
- Parent Portal → Billing showed the household's invoice as **"Draft, $0.00"** when the database
  record was actually `issued`, **$185.00 due**. The page made **zero network calls to the API**
  when loaded (confirmed via browser network inspection) — it isn't reading live data at all.
- Coach/Admin → Performance → Best Times Explorer showed **"0 records"** for a swimmer who has
  best times in the database. This one *did* call the API and got a `200 OK`, but rendered nothing.
- Portal Communications and Admin → Announcements both showed **"No announcements"** despite a
  published, all-audience announcement existing.
- By contrast, **Admin → Reports → Invoice Aging correctly showed "1 row"** for the same invoice
  that Billing showed as zero — so the data is reachable somewhere in the system, just not
  consistently surfaced to the screens a parent or coach would actually look at.

This is the single biggest risk in the product today: a parent could look at Billing, see "$0
due," and skip paying a real invoice. A coach could look up "best times," see nothing, and think a
swimmer has no history. **This isn't a cosmetic issue — it actively tells users false things.**
Recommend treating this as a P0 before any real launch, independent of this UX-only pass.

---

## Persona 1 — Parent, first-time, on phone, meet in 20 minutes

| Task | Result |
|---|---|
| Log in first time | Sign-in screen is clean and on-brand (dark theme, serif wordmark). SSO-only ("Continue with SSO") is reasonable for a club-managed identity provider, but a first-time parent has **no visual cue what to type** beyond a generic email placeholder — no club name/logo context, no "what happens next" preview. Minor hesitation point, not a blocker. |
| Find kid's heat/lane for today's meet | **Not possible.** There is no heat/lane assignment concept anywhere in the product — not on the meet detail page, not in entries, not in the data model. The meet detail page shows session structure ("Day 1 Prelims — 3 events") but no per-swimmer heat/lane, and the "Meet Files" card (where a coach-uploaded PDF heat sheet would presumably go) said "No meet files available yet." A parent's single most time-pressured question — *where does my kid swim and when* — has no answer in the app at all today. |
| Find arrival/warm-up time | ✅ **FIXED.** Not shown before; the parent and swimmer meet-detail pages now display each session's date plus "Warm-up 8:00 AM · Starts 9:00 AM" using the backend's existing `warm_up_time`/`start_time` fields. |
| Check outstanding balance | ✅ **FIXED.** See cross-cutting issue above — now shows the real $185 due instead of $0/Draft. |
| Message a coach | **No such feature exists.** "Communications" is one-way (club → member) announcements plus a notification-preferences screen. There is no messaging surface, no coach contact info, no "email the coach" link anywhere in the parent portal. A parent with a quick question has no in-app path — a real gap against the explicit ask in the persona. *(Not addressed — net-new feature, out of scope for the follow-up fix pass.)* |
| Update allergy / emergency contact | Partially works, with a scary detour: navigating to a swimmer's own profile first shows a red banner **"Swimmer activity service is not configured"** with a "Retry" button, and every club-managed field (name, DOB, age, gender) renders as **"—"** even though the same data displays correctly one screen earlier on the household overview. A parent updating urgent medical info would reasonably think the app is broken or their kid's record was wiped. Once past that, there *is* a parent-editable "Medical / Emergency Notes" field — but it's a single freeform text box, not structured fields (no separate Emergency Contact Name / Phone / Relationship), so there's no way to know what format the club expects. *(Not addressed — structured fields are a net-new feature, out of scope for the follow-up fix pass.)* |

✅ **FIXED.** ~~Household Profile page also shows the raw internal `HOUSEHOLD ID` UUID directly to the parent~~ — the field has been removed from the parent-facing Household Profile page.

## Persona 2 — Parent, returning, at home

- **Register for an upcoming meet**: not testable with current data — the only meet in the system
  is past its commitment window (`entry_finalized`), so the open-commitment flow never renders.
  Not a finding against the product, just a data-state limitation of this pass.
- **Check best times / season progress**: ✅ **FIXED** — see cross-cutting issue (best times now
  shows the swimmer's recorded times).
- **Team calendar**: this one works well. `/portal/calendar` correctly renders today's practices
  and upcoming meets in a clean month view. The meet-duplication issue it surfaced is fixed below.
- **Opt in/out of fundraiser or volunteer slot**: **feature doesn't exist.** "Volunteer" only
  appears as an announcement *category filter* — there is no sign-up, RSVP, or slot-selection
  mechanism anywhere in the codebase. If a club currently runs volunteer shifts or fundraisers
  through TeamUnify, there's no replacement here at all. *(Not addressed — net-new feature, out of
  scope for the follow-up fix pass.)*

✅ **FIXED. Duplicate-looking meets**: the stray seed-data duplicate (the `draft` "Summer
Invitational 2026" row with no real entries/commitments) has been deleted from the database, so the
parent dashboard and calendar no longer show the same meet listed multiple times.

## Persona 3 — Coach, mid-practice, phone/tablet

- **Mark attendance in <30s**: this is the biggest coach-facing gap. `/training` is a **desktop
  admin data table** (dark dense grid, small trash-can icons, no touch-sized targets) — not a
  mobile-optimized view at all, despite this being the exact persona ("phone or tablet in hand").
  Worse, attendance itself is **a single typed headcount number** ("ATTENDANCE: ___"), not a
  per-swimmer roster checklist. A coach can't tap through names to mark who showed up, can't see
  who's missing, and can't message an absent swimmer's parent from there. This directly fails the
  stated task. *(Not addressed — per-swimmer attendance is a net-new feature, out of scope for the
  follow-up fix pass.)*
- ✅ **FIXED.** ~~One workout row displayed the raw group UUID~~ instead of the group name — when a
  workout belongs to a group outside the viewing coach's own roster (by design, coaches only see
  groups they're assigned to), the schedule now shows "Other group" instead of leaking the raw ID.
- **View/update today's practice set**: the "Add Workout" form itself is reasonable (date, group,
  type, description, per-stroke yardage) — this part is usable, just not fast/mobile-friendly for
  the "mid-practice" moment.
- **Message a group of parents**: no dedicated feature, but Announcements does support a
  "Specific group" audience, so a coach *could* reasonably approximate this by drafting a
  group-scoped announcement — reasonably discoverable, though one-way only (no reply).
- **Look up a swimmer's recent times**: ✅ **FIXED** — same cross-cutting empty-state bug, now
  resolved; Best Times Explorer shows the swimmer's recorded times.
- ✅ **FIXED.** ~~Logging in as Coach lands on the same admin dashboard an Owner/Admin sees~~ —
  dashboard quick actions are now scoped by role, so Coach no longer sees "New Invoice" or
  "Upload Import".

## Persona 4 — Coach, at a desk, planning

- **Build/edit a meet roster or lineup**: this works well. The meet detail page's Entries tab shows
  a clean, readable table (swimmer, event, seed time, status) with real names and correct approval
  status. This is one of the better-executed screens in the product.
- ✅ **FIXED.** ~~The meet detail breadcrumb and page title both render the raw meet UUID split into
  title-cased words~~ — the breadcrumb now skips UUID path segments entirely (site-wide, not just
  meets), and the meet subtitle line no longer shows the raw ID, just host and location.
- **Enter results after a meet**: not exercised in this pass (no completed meet with pending
  results in the seed data) — flagging as untested rather than pass/fail.
- **Set up next month's practice schedule**: the "Add Workout" form supports this one workout at a
  time; there's no visible bulk/recurring-schedule tool (e.g., "repeat this practice every Mon/Wed/Fri
  for the next 4 weeks"), which a coach planning a month at once would likely expect and miss.
  *(Not addressed — net-new feature, out of scope for the follow-up fix pass.)*

## Persona 5 — Swimmer (young teen), on phone

- Logging in as the dev-stub Swimmer landed on a **swimmer-specific, appropriately simplified**
  `/swim` dashboard (good — distinct from the dense admin view coaches get), with a clear
  "Good morning" greeting and card layout.
- However, the stub swimmer account showed: **"Your account isn't linked to a swimmer profile
  yet... Ask your club admin to link your login to your swimmer record."** This may be a
  dev-environment artifact (the demo login literally isn't wired to a swimmer row), but it reveals
  a real product mechanic worth calling out: a swimmer's user login and their swimmer record are
  two separate things that require a manual admin linking step, with **no self-service recovery
  path** shown to the swimmer beyond a text instruction to "ask your club admin." If this can
  happen in production (e.g., a household adds a swimmer login before an admin links it), a teen
  would see this exact dead-end message with no clear next action. *(Not addressed — self-service
  recovery is a net-new feature, out of scope for the follow-up fix pass.)*
- Because of the above, "check own times," "today's practice group," and "upcoming meets" couldn't
  be verified end-to-end for this persona in this pass. (The swimmer meet-detail page's warm-up/start
  time display was fixed alongside the parent page's — see Persona 1 — but couldn't be re-verified
  live here due to the same unlinked-swimmer blocker.)

## Persona 6 — Admin, light pass

- **Add a new family/swimmer**: not exercised end-to-end, but the entry points ("+ New Household",
  "+ Add Swimmer" quick actions on the dashboard) are clearly labeled and discoverable.
- **Send a team-wide announcement**: the Announcements composer ("All members" / "Specific group",
  "Create Draft") is clear and well-labeled. ✅ **FIXED** — the summary tiles and list now correctly
  reflect the published announcement (same cross-cutting fix, confirmed on the admin side too).
- **Check payment/dues status across the team**: works correctly via Admin → Reports → Invoice
  Aging (correctly showed 1 outstanding invoice), so this task is achievable — just not through
  the more obviously-named Billing-adjacent screens a first-time admin might try first.

---

## Settings pages — content sanity review

This was the most concerning single area of the audit.

> ✅ **FIXED (2026-07-03 follow-up).** `/settings` is now a genuine club-facing page — just "Team &
> Export Identity" plus a link out — and the raw runtime config (Auth/Workers/Compliance/Storage/
> Stripe/Postmark/Telemetry tabs, the `APP_ENV` hardening warning, etc.) moved to a new
> `/settings/system` ("System Diagnostics") page with its own nav entry. A club admin clicking
> "Settings" no longer lands on what reads as an internal ops dashboard.

**`/settings` ("Settings" in the nav) is a raw backend configuration/engineering debug panel, not
a club-facing settings page.** On login as Admin, it renders:
- A yellow warning banner: *"Backend reports `APP_ENV=development`. Verify production-only
  hardening before cutover."*
- Tabs literally labeled **Auth / Workers / Compliance / Storage / Stripe / Postmark / Telemetry**,
  each showing raw config keys and values like `oidc_client_id`, `oidc_redirect_url`,
  `enable_dev_stub_tokens: true`, `allow_dev_session_issue: true`.

A club owner or admin — not a software engineer — clicking "Settings" expecting things like "team
name, season dates, notification defaults, billing details" would instead land on what reads as an
internal ops dashboard, complete with a scary-looking hardening warning. The one genuinely
club-relevant section ("Team & Export Identity" — team code/name/LSC code) is present but buried
beneath/above this technical content rather than being the whole page.

**`/settings/roles` ("Role Management")** is better-scoped (assign a role to a user, with a reason
and history log) and the concept is understandable. ~~its summary tiles... don't reflect the org's
actual ~7 seeded role assignments~~ — **on closer inspection this is not a bug**: this page's
"Assignment History" is a separate in-memory-only audit trail of role changes made *through this
UI*, with no DB persistence yet, so it genuinely starts at zero every run — it's unrelated to the
Keycloak realm's seeded user/role data and to the cross-cutting snapshot bug fixed above. Left
as-is; a real fix would mean adding persistence to that feature, which is out of scope here.

**Recommendation**: ✅ done — see above.

---

## Missing features (expected from TeamUnify/SportsEngine norms, not present today)

*(None of the items below were addressed in the 2026-07-03 follow-up pass — all 7 are net-new
features, explicitly deferred to their own scope rather than bundled into the fix/polish pass.)*

1. **Heat sheets / heat-lane-time lookup** — no structured way for a parent to see which
   heat/lane/session their swimmer is in for a meet; relies entirely on an (currently unused)
   PDF-export mechanism.
2. **In-app parent ↔ coach messaging** — communications are one-way, club → member only.
3. **Volunteer sign-up / fundraiser participation** — no signup or RSVP mechanism; "Volunteer" is
   only an announcement category label.
4. **Structured emergency contact fields** — swimmer medical/emergency info is a single freeform
   notes box, not separate Name/Phone/Relationship/Allergy fields.
5. **Per-swimmer attendance tracking** — practice attendance is a single aggregate headcount, not a
   roster check-in.
6. **Recurring/bulk practice scheduling** — practices are entered one at a time; no "repeat weekly"
   tool for planning a month ahead.
7. **Self-service recovery when a swimmer login isn't linked to a swimmer record** — currently a
   dead-end message pointing the user to "ask your club admin" with no in-app request/notify action.

## Dead weight / clutter

- ✅ **FIXED.** ~~Coach role sees the full Owner/Admin dashboard~~, including "New Invoice" and
  "Upload Import" quick actions that are irrelevant to a coach's job — dashboard quick actions are
  now scoped per role.
- ✅ **FIXED.** ~~Raw UUIDs surfacing in user-facing text~~ in the three places named (household ID
  on the Parent Household Profile, group ID in the Training schedule, meet ID in the meet-detail
  breadcrumb/subtitle) — all three cleaned up.
- ✅ **FIXED.** ~~Duplicate "Summer Invitational 2026" meet rows~~ — the stray seed-data duplicate
  was deleted from the database.

---

## Summary

The core data-entry and structural screens (meet roster/entries, workout scheduling, announcement
composition, calendar) are well-designed and would likely make sense to a non-technical user on
first look. The two areas that would most damage trust in a real launch were: (1) multiple
"status" screens quietly showing wrong/empty data instead of what's actually in the database, and
(2) the "Settings" page being a raw engineering config dump rather than a club-facing settings
page. The heat-sheet/heat-lane gap and lack of parent-coach messaging are the most consequential
*missing* features relative to what a TeamUnify-familiar family would expect on meet day.

---

## Follow-up fix pass — status (2026-07-03)

**Fixed:**
- Cross-cutting empty-state data bug (root cause: stale `runtime_snapshots` bridge overwriting
  fresh DB-loaded state on API startup) — Billing, Best Times, and Announcements (admin + portal)
  all confirmed showing correct live data now.
- `/settings` split into a real club-facing page + a separate `/settings/system` diagnostics page.
- Arrival/warm-up time now shown on parent and swimmer meet-detail pages.
- Household ID UUID removed from the Parent Household Profile.
- Raw group UUID in the Training schedule replaced with a readable fallback.
- Raw meet UUID removed from the meet-detail breadcrumb and subtitle.
- Coach dashboard quick actions scoped to what a coach actually does.
- Stray duplicate seed-data meet row deleted from the database.

**Clarified, not a bug:** Role Management's "0 assignments" — that page's history is a separate,
not-yet-persisted audit trail (starts empty every run by design), unrelated to the snapshot bug.

**Not addressed (net-new features, explicitly deferred):** heat sheets/heat-lane lookup, in-app
parent↔coach messaging, volunteer/fundraiser signup, structured emergency contact fields,
per-swimmer attendance tracking, recurring/bulk practice scheduling, self-service swimmer-link
recovery. Also not addressed: mobile-friendly `/training` layout, bulk-schedule tooling, and the
"Swimmer activity service is not configured" error on the swimmer profile edit flow.
