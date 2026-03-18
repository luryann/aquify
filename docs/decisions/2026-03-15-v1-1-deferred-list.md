# ADR: Locked v1.1 Deferred List

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires explicit lock of what is deferred beyond v1.

## Decision
Lock v1.1 deferred modules to:
1. SMS/push messaging.
2. Volunteer management/credits automation.
3. Advanced analytics/workout planning analytics.
4. Additional report packs and forecasting.
5. Expanded non-critical integrations.

Out-of-scope (not v1.1 commitment by default):
1. Multi-club public SaaS onboarding.
2. Native mobile apps (PWA-first strategy in v1).
3. Full website CMS replacement.
4. AI-driven training recommendations.

## Alternatives Considered
- Leaving deferrals implicit
- Explicit v1.1 + out-of-scope lock (selected)

## Consequences
- Prevents scope creep during v1 execution.
- Requires backlog tagging by `v1.1` or `out-of-scope`.

## References
- `docs/planning/Developer Handoff Master Document.txt` sections 1.2 and 1.3
- `TODO.md` STEP 0 item 11
