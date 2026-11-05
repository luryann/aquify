# aquify Platform - Locked Decisions

Last updated: 2026-11-03
Purpose: remove ambiguity so a new engineer/AI can execute without additional context.

## 1) Authentication Provider
- Decision: `Keycloak`
- Status: Locked

## 2) Hosting Target
- Decision: self-managed deployment on a `VPS`
- Status: Locked
- Note: exact VPS vendor is not yet specified; architecture should remain portable.

## 3) Domain Strategy
- Decision: existing team-owned domain will be used.
- Status: Locked
- Note: DNS is already owned/managed by team.

## 4) Compliance / Legal Controls
- Decision: operational/legal approach already finalized by team.
- Status: Locked
- Implementation instruction: proceed with requirements from planning docs as authoritative.

## 5) Payments / Stripe
- Decision: `TBD` for account ownership and rotation policy; the multi-club model is decided (section 9): each club connects its own Stripe account (Connect).
- Status: Open
- Required before production billing launch:
  - Stripe account/workspace ID
  - API key ownership and rotation policy
  - Webhook endpoint secret management
  - Refund approval policy (roles + thresholds)

## 6) Migration Mapping Source Inputs
- Decision: Not provided yet.
- Status: Open
- Required before migration implementation completion:
  - Legacy export file list and sample files
  - Field-by-field mapping sheet
  - Data quality exception rules

## 7) Definition of Done for Launch
- Decision: production-ready plus explicit user signoff.
- Status: Locked
- Release gate interpretation:
  - System is production-ready by engineering standards.
  - Final approval is user/business signoff.

## 8) Parent and Swimmer Account Policy
- Decision: Parent account is required. Swimmer account is optional.
- Status: Locked
- Implementation requirements:
  - All household-critical workflows must work with parent account alone:
    - registration
    - meet commitments
    - billing/payments
    - document/compliance actions
  - Swimmer account may be disabled globally or per household without blocking launch.
  - Every person who signs in (parent or swimmer) uses their own Aquify account; there are no shared or account-less logins. An admin can turn swimmer sign-in off for the whole club or for one swimmer (parents can do the same for their own swimmers); disabled swimmers keep their records and parents lose nothing.
  - The swimmer portal (`/swim`) is always available to users with the `swimmer` role; there is no feature flag. Households without swimmer accounts are simply unaffected.
  - Swimmer role is read-only by default unless specific field-level permissions are explicitly enabled.

## 9) Meet Commitments and Event Selection Policy
- Decision: After a parent commits `yes`, event selection per swimmer is allowed and required for full entry workflow.
- Status: Locked
- Implementation requirements:
  - Parent can select preferred/eligible events for each committed swimmer.
  - Coach can edit/override parent-selected events before entry lock/submission.
  - Every coach override requires an audit event with actor, timestamp, and before/after event set.
  - Meet files must include event definitions and be parsed/validated against internal meet events.
  - Entry export/import logic must preserve swimmer-event mappings deterministically.

## Execution Rule for New Contributors
If any conflict exists between planning docs and code, follow this precedence:
1. `DECISIONS.md` (this file)
2. `Developer Handoff Master Document.txt`
3. `Plan.txt`
4. `TODO.md`

## 9) Multi-Tenancy (supersedes the v1 single-club tenant strategy)
- Decision: shared database and schema; every tenant row carries `org_id` (UUID, FK to the `orgs` registry); isolation is enforced by the application (org from the session, never the client), by composite foreign keys, and by automated cross-org tests.
- Status: Locked (ADR `docs/decisions/2026-11-03-multi-tenancy.md`)
- Locked sub-decisions:
  - One person belongs to exactly one club (`users.org_id`); no multi-club memberships, no org switcher. One email, one club.
  - Single Keycloak realm; the `org_id` token claim must equal `users.org_id` or sign-in is refused.
  - Each club takes payments on its own Stripe connected account (Stripe Connect, Standard); outside development a club that has not finished onboarding cannot take online payments.
  - Clubs are created, paused and closed only by platform operators (`platform_admin` in the dedicated platform org, MFA required), never by self-service.
- Database-level isolation: Postgres row-level security is enabled (FORCE) on every tenant table (the application role must not be a superuser/BYPASSRLS); repositories bind the org per statement, and the few lookups made before a club is known bind a credential through narrow policies (tracker 4.4, closed).
- Tracker: `docs/multi-tenancy-implementation-tracker.md`
