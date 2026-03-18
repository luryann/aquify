# ADR: Manual-at-Launch Workflows

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires explicit manual workflows allowed at launch.

## Decision
Allow only the following manual workflows in v1 launch:
1. Conflict queue review for ambiguous swimmer matching.
2. Finance review for flagged reconciliation discrepancies.
3. Manual override for edge-case imported records with mandatory audit reason.

## Alternatives Considered
- Zero manual ops at launch
- Broader manual exception list
- Narrow, explicit list (selected)

## Consequences
- Runbooks and owner assignment for these workflows are required before cutover.
- Any additional manual workflow requires explicit approval and decision update.

## References
- `docs/planning/Developer Handoff Master Document.txt` section 1.4
- `TODO.md` STEP 0 item 8
