# DARE Platform - Locked Decisions

Last updated: 2026-10-27
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
- Decision: `TBD`
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
