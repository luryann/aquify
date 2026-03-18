# ADR: Observability Stack Model

- Status: Accepted
- Date: 2026-03-15

## Context
STEP 0 requires selecting managed vs self-hosted observability.

## Decision
Use `self-hosted` observability with OpenTelemetry + Prometheus/Grafana + Loki.

## Alternatives Considered
- Managed APM/logging suite
- Self-hosted OSS stack (selected)

## Consequences
- More ops ownership (capacity, upgrades, retention tuning).
- Lower recurring SaaS cost and tighter data control.
- Must implement baseline dashboards/alerts before production readiness.

## References
- `docs/planning/Developer Handoff Master Document.txt` section 3
- `CLAUDE.md` "Planned Tech Stack"
- `TODO.md` STEP 0 item 5
