# ADR: Org/Tenant Strategy for v1

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires tenancy model lock before schema and auth middleware work.

## Decision
Use `single-club deployment for v1` with `org_id` present on business tables for forward compatibility.

## Alternatives Considered
- Immediate multi-tenant architecture
- Single-club with no tenant column
- Single-club with reserved `org_id` (selected)

## Consequences
- v1 complexity stays lower while preserving migration path to multi-tenant.
- Middleware, queries, and constraints must still treat `org_id` as first-class.
- Multi-club SaaS onboarding remains explicitly out of v1 scope.

## References
- `docs/planning/Developer Handoff Master Document.txt` sections 1.3, 7.1, 23
- `Plan.md` sections on tenant field strategy
- `TODO.md` STEP 0 item 7
