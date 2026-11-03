# Launch Owner Matrix — aquify Platform

**Version:** 1.0
**Last Updated:** 2026-03-23

---

## Owner Assignments

| Function | Primary Owner | Backup | Escalation Path | Contact Channel |
|---|---|---|---|---|
| Engineering — Deployment | Lead Engineer | Senior Engineer | CTO | `#aquify-ops` Slack + PagerDuty |
| Engineering — Incidents | On-call Engineer (rotating) | Lead Engineer | CTO | PagerDuty → `#aquify-alerts` |
| Engineering — DB/Infra | DevOps Engineer | Lead Engineer | Lead Engineer | `#aquify-ops` Slack |
| Product — Member Management | Product Manager | Club Admin Lead | Director of Operations | `#aquify-ops` Slack |
| Product — Training / Practices | Head Coach | Asst. Coach | Club Director | `#aquify-ops` Slack |
| Operations — Day-to-Day Admin | Club Admin Lead | Operations Staff | Director of Operations | `#aquify-support` Slack |
| Operations — Registration | Club Admin Lead | Product Manager | Director of Operations | `#aquify-support` Slack |
| Finance — Billing & Invoicing | Club Treasurer | Finance Admin | Executive Director | `#aquify-finance` Slack |
| Finance — Stripe Reconciliation | Finance Admin | Club Treasurer | Executive Director | `#aquify-finance` Slack |
| Support — Parent Issues | Club Admin Lead | Operations Staff | Director of Operations | `#aquify-support` Slack |
| Support — Swimmer Issues | Head Coach | Club Admin Lead | Director of Operations | `#aquify-support` Slack |
| Security — Data Incidents | Lead Engineer + CTO | — | Legal counsel | Direct call + `#aquify-alerts` |
| Vendor — Stripe | Finance Admin | Lead Engineer | Stripe Support | Stripe Dashboard + email |
| Vendor — Postmark | Lead Engineer | DevOps Engineer | Postmark Support | `#aquify-ops` |
| Vendor — Keycloak / OIDC | Lead Engineer | DevOps Engineer | Keycloak Support | `#aquify-ops` |

---

## On-Call Rotation

Engineering is on-call on a weekly rotation. The rotation schedule is maintained in PagerDuty.

| Week | Primary On-Call | Backup |
|---|---|---|
| Current week | See PagerDuty | See PagerDuty |

To view the current on-call engineer: `https://pagerduty.com` → aquify service.

### On-Call Expectations

- **Response time:** Within 15 minutes of a PagerDuty page for P0/P1.
- **Availability:** Reachable via phone/SMS during on-call week (including evenings and weekends).
- **Handoff:** On Sunday evening, the outgoing on-call engineer briefs the incoming engineer on any open incidents or known issues.

---

## Escalation SLAs

| Priority | Definition | Initial Response | Mitigation Target | Examples |
|---|---|---|---|---|
| **P0** | Service unavailable; data loss risk; all users impacted | < 15 minutes | < 2 hours | API down, DB unreachable, auth service down |
| **P1** | Critical feature broken; significant user impact | < 1 hour | < 4 hours | Stripe payments failing, import processing down, portal inaccessible |
| **P2** | Non-critical feature degraded; workaround exists | < 4 hours | < 24 hours | Reports slow, email delivery delayed, one portal page broken |
| **P3** | Minor issue; cosmetic or rare edge case | Next business day | Next sprint | UI alignment bug, minor typo, edge-case error message |

### Determining Priority

Use the **P0 severity rubric** at `docs/decisions/2026-03-15-p0-severity-rubric.md` as the authoritative guide.

Quick heuristic:
- **P0:** Would you wake up the CEO? (Data loss, full outage, payment processing down)
- **P1:** Would parents notice and complain within the hour?
- **P2:** Is there a workaround you can give a user right now?
- **P3:** Can it wait until the next sprint planning meeting?

---

## Communication Channels

| Channel | Purpose | Who Monitors |
|---|---|---|
| `#aquify-ops` | Engineering ops, deployments, infra | Engineering team |
| `#aquify-alerts` | Automated alerts from Grafana/Prometheus/PagerDuty | Engineering + on-call |
| `#aquify-support` | Parent/swimmer support tickets and admin questions | Ops + support staff |
| `#aquify-finance` | Billing, reconciliation, Stripe issues | Finance + engineering |
| `#aquify-general` | General announcements, releases | All staff |
| PagerDuty | On-call incident escalation | On-call engineer |

---

## Stakeholder Notifications

### For System Outages (P0)

1. Immediately post in `#aquify-alerts` with the incident start time and initial findings.
2. Notify the Club Director and Executive Director directly (phone or Slack DM) if the outage exceeds 30 minutes.
3. If parents are affected (portal down, payments failing): send an email via Postmark (or manually if email is down) with an estimated restoration time.
4. Post updates in `#aquify-alerts` every 30 minutes until resolved.
5. After resolution: post a brief incident summary in `#aquify-general` within 24 hours.

### For Billing Issues

1. Notify Finance Admin and Club Treasurer in `#aquify-finance`.
2. If a discrepancy affects multiple households: notify the Club Director.
3. If involving a data breach or unauthorized access: notify CTO and legal counsel immediately.

### For Data Incidents (Privacy, GDPR/COPPA)

1. **Immediately** notify the CTO, Executive Director, and legal counsel.
2. Do not communicate externally until legal counsel approves messaging.
3. Follow the data incident response plan (to be linked when created).
4. Log all actions in `#aquify-alerts` with timestamps.

---

## Launch Readiness Sign-Off

Before going live, the following sign-offs are required:

| Area | Sign-Off Required From | Status |
|---|---|---|
| Engineering — API and workers stable | Lead Engineer | Open |
| Engineering — DB migrations applied and verified | DevOps Engineer | Open |
| Product — Admin flows tested and approved | Product Manager | Open |
| Finance — Billing and Stripe integration verified | Club Treasurer | Open |
| Operations — Staff trained on admin guide | Club Admin Lead | Open |
| Security — Threat model reviewed | Lead Engineer + CTO | Open |
| Legal — Privacy policy and COPPA compliance confirmed | Legal counsel | Open |
| Executive — Launch approved | Executive Director | Open |

Sign-offs are recorded in `docs/migration/cutover-signoff-template.md`.
