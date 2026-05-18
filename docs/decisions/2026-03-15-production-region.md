# ADR: Single Production Region

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires a single primary production region to remove ambiguity in deployment and data residency.

## Decision
Set v1 primary production region to `US West`.

## Alternatives Considered
- US East
- Multi-region active-active in v1
- US West (selected)

## Consequences
- Lower latency for current operating geography assumptions.
- Disaster recovery will be warm-standby or restore-based, not active-active.
- If team operating geography changes, this ADR should be superseded with migration plan.

## References
- `TODO.md` STEP 0 item 6
- `docs/planning/Developer Handoff Master Document.txt` sections 19 and 24
