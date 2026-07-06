# Usability and content-clarity audit, 2026-07-05

Scope: every screen across the admin/coach app, the parent portal, the swimmer portal, and the public login/register pages, evaluated only against one question for each screen: would a non-technical parent, coach, or swimmer understand this immediately, or would they hesitate, get confused, or give up? This was not a bug hunt. Backend correctness, data logic, and broken flows were only noted when they blocked a screen from being reviewed at all.

Reviewed live against the running app (dev-stub logins for Owner, Coach, Billing, Parent, and Swimmer roles) with a source-code check as a backup wherever the browser session dropped.

Two honest caveats that apply to the whole document:

- **Mobile viewport was never actually seen.** Every audit pass tried to resize the browser to a phone width (390x844) and could not get the tool to render below roughly 1280px wide. Every mobile-specific note below is inferred from reading the layout source, not from a real phone screenshot. Treat these as "likely, needs a real device check," not confirmed.
- **The Swimmer test account was not linked to a real swimmer profile**, so several swimmer-portal pages were only seen in their "not linked yet" empty state. Findings there separate what's wrong with the empty state itself from what would apply regardless of data.

## How the findings are organized

1. **Content fixes made this pass** — label, terminology, and copy changes only. No layout changes, no new UI elements, no logic changes.
2. **Found, but outside this pass's scope** — real problems that need a data fix, a layout change, a new feature, or a role/permissions change rather than a word change. These need a decision from Ryan before anyone touches them.

---

## Bucket 2 first: found, but needs a decision (not touched this pass)

These cannot be fixed by changing text alone. Fixing the words would either do nothing or paper over a real problem.

### Flow-blockers (fix soonest, not a copy problem)

- **`/users` crashes with a client-side exception** every time it's opened directly or via its own "Users" tab. Only the Role Management tab (`/users?tab=roles`) works. This fully blocks admins from seeing the main user list.
- **Swimmer meet detail page (`/swim/meets/[id]`) shows nothing at all** for an unlinked swimmer account, not even the meet name, date, or location that were already visible on the card the swimmer tapped from. That basic info doesn't depend on the account being linked.

### Data contradictions (the numbers/labels disagree with each other; a copy edit can't fix a wrong count)

- Parent portal Registrations page: top summary says "0 Active Registrations" directly above three swimmer cards all marked "PENDING."
- Parent portal Documents page says "No documents required yet," while the Registrations page (same household, same session) says a registration waiver is still pending for all three swimmers.
- Parent portal Calendar says the Kevin Perry 2026 commitment deadline is "tonight"; the Meets page says the same deadline is "Jul 10, 2026 - 5 days left."
- Admin household detail page: "Contacts: 0" and "No contact on file," directly below a Primary Email and Primary Phone that are both filled in.
- Swimmer dashboard: stat tiles say "Upcoming Meets: 2" and show a next meet date, directly above a banner saying meet info "can't be shown" because the account isn't linked.
- Admin invoice list shows a due date of `2026-07-13` for an invoice; that same invoice's detail page shows `Jul 12, 2026`.

### Missing plain-language status (a real gap, not a wording problem)

- **Nowhere in the swimmer portal can a swimmer tell if they're actually going to a meet.** The parent makes that decision; the swimmer surface never shows the answer. Dashboard, Schedule, and Meet Detail all only show club-side scheduling state ("Not Yet Open"), never the swimmer's own yes/no/waiting status.
- Parent portal meets list never shows a "needs your answer" state distinct from "not yet open" and "committed."

### Layout and navigation (real UI work, not copy)

- No mobile nav exists at all in the admin app (`Sidebar.tsx` has no responsive/hamburger variant), confirmed by reading the source. The portal and swimmer apps do have a working hamburger-drawer pattern below 768px (also confirmed by source, not by seeing it), so this is admin-specific.
- Parent portal calendar's 7-column month grid overflows even on a normal desktop window and would likely be unusable at phone width; it probably needs a simple list/agenda view on narrow screens instead.
- `/performance/visualization` is titled "Visualization" and described as showing "progression history," but it only shows two ranked bar-list summaries, no per-swimmer time-over-time chart. Either the page needs the chart it promises, or the name and description need a bigger rewrite than a one-line fix (tracked here rather than in Bucket 1 since the mismatch is severe enough it reads as a missing feature).

### Role and permissions scoping (not a text problem)

- Logging in as the **Billing** role shows the exact same full sidebar as Owner, including Members, Training, System Diagnostics, and Background Jobs. A billing volunteer sees navigation to areas that have nothing to do with their job.
- `/settings/system` and the "System Diagnostics" section of `/settings` are raw engineering configuration (its own subtitle admits this is "not club administration") but are reachable from the same nav path a club owner uses for real settings. This may need a permissions/routing change, not just a relabel, to keep it fully out of reach for non-engineering roles.
- `/jobs` (Background Jobs) is pure engineering-ops content reachable by any Owner/Admin; it should probably be walled off entirely rather than just relabeled.

### Data/seed cleanliness (not a copy problem, this is test data)

- Several places show obvious placeholder data that would look unprofessional in front of a real club: a household named "Foreign Family," a swimmer named "Foreign Kid," a message reading "test message for audit fix verification," and dashboard greetings using the literal name "Dev." These aren't product bugs, just noting they exist in this environment's seed data.

---

## Bucket 1: content fixes made this pass

Below is what was actually changed. Everything here is a label, piece of copy, tooltip, small helper text, or a harmless display cleanup (removing a raw ID that had no use to a club user); no navigation was restructured and no data-fetching logic changed. All changes passed `tsc --noEmit` and `next lint --max-warnings=0` and were spot-checked live in the running app.

### Terminology and jargon cleanup

- Login page: "Continue with SSO" and its helper text rewritten in plain language for a first-time parent.
- Registration page: the vague "Admin Staff" role (described as "read-only or administrative access") renamed to "Staff (view-only)" with an honest description, since the underlying role actually only grants view access.
- The swimmer's external identifier was called three different things depending on the screen ("External ID" on admin pages, "USS Number" on parent-portal pages). Standardized on "External ID" everywhere it appears in list/detail views. (This is a different field from "USA Swimming ID," which is shown separately on the admin swimmer detail page and was left alone.)
- "Restricted" (shown when medical info is hidden from the current viewer) reworded to "Hidden from you (medical staff only)" so it reads as a permission note, not a claim about the swimmer.
- "NT" (no time recorded) spelled out as "No time yet" on the swimmer-facing profile, performance, and meet pages, where a newcomer is most likely to see it without swim-meet context.
- Time standards page: single-letter gender codes spelled out ("Girls"/"Boys"/"Mixed"), and the standard-level and cut-time cells given explanatory tooltips.
- Groups: age range placeholder text ("-+") replaced with "All ages" / "18+" / "Up to 12" as appropriate; "Registrations" column renamed to "Swimmers"; "Role" column (primary/secondary group) renamed to "Primary group?" with Yes/No values.
- Billing Overview, Billing > Stripe, Ledger, Reports, Audit Log, and Background Jobs: accounting and engineering jargon ("reconciliation," "unreconciled rows," "ledger events," a raw `WORKER_RECONCILE_INTERVAL` string, "rows," "artifact delivery," "Credits"/"Debits") rewritten in plain club language throughout. The Stripe page's "Reconciliation"/"Configuration" tabs are now "Payment Checks"/"Technical Setup" (kept honestly technical rather than mislabeled "Settings," since that tab is genuinely raw env vars and webhook config).
- Audit log filter dropdowns and per-row resource labels (e.g. "keycloak_user," "practice_attendance") relabeled to plain category names ("Sign-ins," "Practice attendance," etc).
- Background Jobs page subtitle now states up front that it's for engineering use only, not day-to-day club administration.
- `/performance/visualization` renamed to "Team Coverage" (nav label and page title) with a description that matches what the page actually shows (two ranked lists), instead of promising a "progression history" chart that doesn't exist there.
- "SCY / SCM / LCM" course codes given full-name tooltips and a plain-text caption everywhere they appear as bare toggles or filters (meets list filter, meet creation form).
- "Pay $185.00 via Stripe" simplified to "Pay $185.00 now."
- Meet commitment flow: "Step 1 - Commitment" and "Step 2 - Select Events" headers reworded to "Step 1 - Are you coming?" and kept the plain-language buttons underneath consistent.
- Reports page: "Payment Reconciliation" report renamed to "Payment Check"; "Rows" stat relabeled "Records"; "Artifact delivery enabled" relabeled "Ready to download."
- Swimmer portal: "cut qualifications" given a plain-language explanation; "Not Yet Open" reworded to "Sign-ups Not Started" so it reads as club scheduling state, not the swimmer's own status.
- Email campaigns subtitle detoned to match the rest of the app's plain, businesslike language instead of a jokey aside about "addresses that never get emailed again."

### Small copy and empty-state improvements

- Session name fallback text (shown when a meet session can't be matched to a real one) changed from a raw ID fragment to a plain "Unscheduled," and entry rows in the coach commitment review now show the actual event name instead of a truncated entry ID where that data was already available on the page.
- Meet file upload step: relabeled "optional" and added a one-line plain-language explanation of what the accepted files are and where to get them.
- Meet creation Review step: dates and deadlines now go through the same friendly date formatting used elsewhere instead of showing raw ISO strings.
- Autopay "Charge Timing" options given a one-line explanation of what charging early means for a parent.
- Entries page: the unexplained "OVERRIDE" pill renamed to "CHANGED" with a tooltip showing the actual override reason where one was recorded.
- Approvals queue: shortened the "(account only, not yet registered for the season)" annotation to plainer wording.
- Swimmer meet commitment "no eligible events" message replaced with a plain explanation ("your coach hasn't opened sign-ups yet").
- Swimmer schedule: "Commitment Deadline: [meet]" event title reworded to "Parent RSVP due: [meet]" since responding is the parent's action, not the swimmer's.
- Swimmer meet detail back link relabeled "Back" instead of always saying "Schedule" regardless of where the swimmer came from.
- Swimmer profile: removed a duplicated "not linked" message so it only appears once; "Unknown" status pill reworded to "Not set."
- Household detail (admin) and household overview (portal): the "Contacts" tab and stat tile renamed "Additional Contacts" with an explanation that the primary email/phone are shown separately, resolving the "0 contacts" contradiction next to a filled-in email and phone.
- Household ID moved to the bottom of the Account Info list (de-emphasized, still copyable) instead of leading above the email and phone a club user actually cares about; the equivalent raw IDs were removed entirely from the Households and Groups list tables, the Groups detail header, the Registrations table, and the Autopay household list, since none of them had any use to a non-technical viewer.
- Invoices list: the "Due" date column now uses the same friendly date format as the "Issued" column instead of a raw ISO string.
- Invoice detail: the "Ledger" tab renamed "History."
- Fixed a couple of pre-existing stray em dashes in on-screen copy encountered while editing nearby text.
