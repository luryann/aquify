# ADR: Hosting Target for v1

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requested hosting target lock (Fly.io vs Render) before build-out.

## Decision
Use `self-managed VPS` deployment for v1, fronted by Cloudflare.

## Alternatives Considered
- `Fly.io`
- `Render`
- `Self-managed VPS` (selected)

## Consequences
- Infrastructure automation and runbooks become launch-critical.
- Vendor lock-in is minimized; architecture remains portable.
- Existing TODO wording (Fly.io vs Render) is superseded by locked project decision.

## References
- `DECISIONS.md` section "Hosting Target"
- `docs/planning/Developer Handoff Master Document.txt` sections 3 and 19
- `TODO.md` STEP 0 item 4
