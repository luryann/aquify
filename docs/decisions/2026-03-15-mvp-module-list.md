# ADR: Locked MVP Module List (v1)

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires freezing the launch-blocking module list.

## Decision
Lock v1 MVP modules to:
1. Auth + MFA + RBAC.
2. Households, swimmers, coaches, groups, registrations.
3. Meet setup, commitments, entries.
4. SD3 export (standard + extended).
5. CL2/SD3/ZIP import + reconciliation queue.
6. SDIF roster import/export.
7. Best-times + update-times workflow from imports.
8. Billing core (invoices, payments, refunds, ledger).
9. Communications core (announcements + email).
10. Audit logging for privileged actions.
11. Core admin reports (membership, meet fees, billing status).

## Alternatives Considered
- Broader v1 scope including volunteer and advanced analytics
- Minimal subset missing billing/comms
- Feature registry aligned v1 scope (selected)

## Consequences
- Launch scope changes require decision supersession and schedule impact review.
- Backlog grooming should prioritize these modules only for launch readiness.

## References
- `docs/planning/Developer Handoff Master Document.txt` sections 1.1 and 2
- `TODO.md` STEP 0 item 10
