# DARE Platform — Hypercare Runbook

**Last updated:** 2026-03-18
**Window:** First 24 hours after production cutover
**Owner:** Engineering on-call with Product, Operations, and Finance partners

## Goal

Monitor launch health closely during the first 24 hours after traffic moves to
the new platform and define who responds when a launch-risk signal appears.

## Dashboard set

- API error rates: `docs/observability/grafana/api-error-rates.json`
- Queue depth and job failures: `docs/observability/grafana/queue-depth-job-failures.json`
- Import success and failure: `docs/observability/grafana/import-success-failure.json`
- Webhook lag: `docs/observability/grafana/webhook-lag.json`

## Monitoring cadence

| Time from cutover | Action |
|---|---|
| 0h to 2h | Review dashboards every 15 minutes |
| 2h to 8h | Review dashboards every 30 minutes |
| 8h to 24h | Review dashboards hourly |

## Mandatory checks

1. API error rate remains within the agreed launch baseline.
2. Background job queues are draining and not growing unbounded.
3. Import success rate remains stable after the first live uploads.
4. Stripe webhook lag stays within the alert threshold.
5. Billing reconciliation remains within the approved delta.
6. No new P0 defects are opened.

## Incident routing

| Symptom | Primary owner | Secondary owner |
|---|---|---|
| API error spike | Engineering | Operations |
| Import failures | Engineering | Product |
| Payment webhook lag or mismatch | Finance | Engineering |
| Support volume spike | Operations | Product |

## Launch status updates

Post updates to the launch channel at:

- cutover complete
- +30 minutes
- +2 hours
- +8 hours
- +24 hours / hypercare exit

Each update should include:

- current API health
- open incidents
- import status
- billing status
- go-forward / escalation decision

## Hypercare exit criteria

- no unresolved P0 or P1 issues
- billing reconciliation is green
- imports and exports are stable
- support volume is within expected range
- Product, Engineering, Operations, and Finance agree to exit hypercare
