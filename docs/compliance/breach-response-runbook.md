# Security Breach Response Runbook

**Version:** 1.0
**Owner:** Engineering / Operations
**Last updated:** 2026-03-16

---

## Purpose

This runbook defines the step-by-step process for detecting, containing, investigating, and notifying relevant parties in the event of a data security breach involving DARE Platform.

---

## Severity Classification

| Severity | Description | Example |
|----------|-------------|---------|
| P0 — Critical | Confirmed exfiltration of PII or financial data | DB credentials leaked; production dump accessed |
| P1 — High | Suspected breach, unauthorized access confirmed | Unexpected logins; suspicious audit log entries |
| P2 — Medium | Policy violation, no confirmed data access | Mis-configured bucket (public but not accessed) |
| P3 — Low | Near-miss or theoretical exposure | Dependency CVE with no known exploit |

---

## Phase 1: Detection and Initial Triage (0–2 hours)

1. **Identify the trigger** — alert, customer report, automated scan, or internal discovery.
2. **Assign Incident Commander (IC)** — on-call engineer or most senior available engineer.
3. **Open an incident channel** (e.g., `#incident-YYYY-MM-DD`) and document all findings in real time.
4. **Preserve evidence** — snapshot affected logs, audit events, and system state before any changes.
   - Export audit events: `GET /api/v1/audit-events?from=<ISO_TIMESTAMP>&to=<ISO_TIMESTAMP>`
   - Snapshot relevant DB tables (do NOT alter them).
5. **Classify severity** using the table above.
6. **Activate response team** based on severity:
   - P0/P1: IC + CTO + Legal + external security counsel (if needed)
   - P2: IC + Engineering lead
   - P3: IC + responsible engineer

---

## Phase 2: Containment (2–6 hours for P0/P1)

### For credential compromise
- Rotate all affected credentials immediately: DB passwords, API keys, OIDC client secrets.
- Revoke all active sessions: drain `sessions` table or restart auth service with new signing keys.
- Disable affected Keycloak accounts pending investigation.

### For unauthorized data access
- Review `audit_events` table for the actor's activity: filter by `actor_user_id` or IP in metadata.
- Revoke sessions for the affected user(s): call `POST /api/v1/auth/sessions/revoke`.
- If a webhook endpoint was compromised, rotate `STRIPE_WEBHOOK_SECRET` and `POSTMARK_SERVER_TOKEN`.

### For infrastructure breach
- Isolate the affected host/container from the network.
- Notify cloud provider support if underlying infrastructure is involved.
- Do NOT destroy evidence — take a forensic snapshot first.

---

## Phase 3: Investigation (within 24 hours for P0/P1)

1. **Scope of exposure**: identify exactly which records were accessed or exfiltrated.
   - Query `audit_events` for the impacted time window and resource types.
   - Cross-reference with access logs (application, DB, storage).
2. **Attack vector**: determine how access was obtained (stolen credentials, injection, misconfiguration, etc.).
3. **Data classification**: determine whether exposed data includes:
   - PII (names, dates of birth, email addresses)
   - Financial data (payment methods, invoices)
   - Medical flags
   - Authentication tokens / session data
4. **Timeline reconstruction**: produce a chronological event log for legal and regulatory purposes.

---

## Phase 4: Notification

### Internal notification (always)
- Notify CTO within 1 hour of P0/P1 confirmation.
- Notify Legal within 2 hours.
- Log all actions taken in the incident channel.

### Regulatory notification
- **GDPR (if EU residents are affected):** notify the relevant Data Protection Authority within 72 hours of becoming aware of the breach if it is likely to result in risk to individuals. See `GDPR Art. 33`.
- **CCPA (if California residents are affected):** notify affected California residents in the most expedient time possible without unreasonable delay.
- Consult Legal before sending any external notification.

### Customer/household notification
- Notify affected households via email using the communications module.
- Template must include:
  - What happened
  - What data was involved
  - What we are doing
  - What affected users can do (e.g., change passwords)
  - Contact for questions
- Do not notify until Legal has reviewed the message.

---

## Phase 5: Recovery

1. **Patch** the vulnerability or misconfiguration that enabled the breach.
2. **Restore** any corrupted or deleted data from backup.
3. **Re-enable** affected services only after containment is confirmed.
4. **Reissue** credentials to affected users if their credentials were compromised.
5. **Test** the patched environment before returning to production traffic.

---

## Phase 6: Post-Incident Review (within 5 business days)

1. **Root cause analysis (RCA)** — document in `docs/incidents/YYYY-MM-DD-incident.md`.
2. **Lessons learned** — identify gaps in detection, response time, and tooling.
3. **Action items** — create tracked issues for all remediation steps.
4. **Policy updates** — update security policies and runbooks if warranted.
5. **Audit report** — generate a final report for Legal and any affected regulator.

---

## Key Contacts

| Role | Responsibility |
|------|---------------|
| Incident Commander (IC) | Coordinates response, makes containment decisions |
| CTO | Executive escalation, approves major decisions |
| Legal Counsel | Regulatory notification, customer communications review |
| Security Lead | Technical forensics, vulnerability analysis |
| Operations Lead | Infrastructure actions, credential rotation |

> Update this table with real names and contact info before production launch.

---

## Relevant DARE Platform Resources

| Resource | Location |
|----------|----------|
| Audit log API | `GET /api/v1/audit-events` |
| Session revoke API | `POST /api/v1/auth/sessions/revoke` |
| Data export request API | `POST /api/v1/compliance/data-export-requests` |
| Data deletion request API | `POST /api/v1/compliance/data-deletion-requests` |
| Audit event DB table | `audit_events` |
| Webhook events DB table | `webhook_events` |
| Suppression list API | `GET /api/v1/suppression` |

---

## Regulatory References

- GDPR Article 33 — Notification of a personal data breach to the supervisory authority
- GDPR Article 34 — Communication of a personal data breach to the data subject
- CCPA §1798.150 — Consumer rights and enforcement
- COPPA (if any users are under 13) — Children's Online Privacy Protection Act
