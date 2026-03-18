# ADR: P0 Defect Severity Rubric

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires a concrete P0 rubric to enforce release gates and triage consistency.

## Decision
Classify as `P0` when any condition is true:
1. Security: unauthorized access to sensitive data/actions, auth bypass, or privilege escalation.
2. Data integrity: data loss/corruption in source-of-truth entities (memberships, meet entries/results, billing ledger).
3. Billing correctness: incorrect charge/refund/ledger mutation that can affect financial reconciliation.
4. Availability: production outage or critical path failure with no workaround for core v1 flows.
5. Interoperability blocker: SD3 export/import or CL2/SD3/ZIP ingestion failure that blocks meet operations.

P0 response targets:
- Acknowledge within 15 minutes during support coverage.
- Mitigation started immediately.
- No production cutover or release progression until resolved and verified.

## Alternatives Considered
- Broad/semi-subjective P0 definition
- Strict measurable rubric (selected)

## Consequences
- CI and release gate checks must fail on unresolved P0 test failures.
- Incident log must capture timeline, mitigation, root cause, and prevention action.

## References
- `Plan.md` section on severity and response objectives
- `docs/planning/Developer Handoff Master Document.txt` sections 1.5 and 21
- `TODO.md` STEP 0 item 9
