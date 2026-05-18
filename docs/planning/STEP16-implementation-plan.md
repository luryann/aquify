# STEP 16 — Audit and Compliance Primitives

## Overview
Implement an immutable audit log, before/after diff helper, compliance data-export/deletion request skeletons, an audit query endpoint, retention policy config, and a compliance runbook.

## Tasks Checklist
- [ ] `db/migrations/20260316000100_create_audit_events.sql` — immutable append-only table
- [ ] `db/migrations/20260316000200_create_data_export_requests.sql`
- [ ] `db/migrations/20260316000300_create_data_deletion_requests.sql`
- [ ] `server/internal/app/audit/service.go` — in-memory audit service + Diff helper
- [ ] `server/internal/app/compliance/service.go` — data export/delete request skeletons
- [ ] `server/internal/app/handlers/audit.go` — GET /api/v1/audit-events
- [ ] `server/internal/app/handlers/compliance.go` — compliance request endpoints
- [ ] Update `config.go` — retention policy env vars
- [ ] Update `server.go` — wire audit + compliance services; inject audit into handlers
- [ ] Audit calls in `handlers/billing.go` (write-off, issue, refund)
- [ ] Audit calls in `handlers/meets.go` (entry override, entry approve, lock-commitments)
- [ ] Audit calls in `handlers/performance.go` (best-time override)
- [ ] Audit calls in `handlers/auth.go` (role assignment)
- [ ] Audit calls in `handlers/imports.go` (conflict resolve)
- [ ] Audit calls in `handlers/members.go` (swimmer update)
- [ ] `docs/compliance/breach-response-runbook.md`
- [ ] Update `CLAUDE.md` with STEP 16 summary
- [ ] Mark TODO.md items complete

## Design Decisions

### Audit Events Table (immutable)
```
id TEXT PK
org_id TEXT NOT NULL
actor_user_id TEXT NOT NULL
actor_roles TEXT[]
action TEXT NOT NULL          -- e.g. "billing.invoice.write_off"
resource_type TEXT NOT NULL   -- e.g. "invoice"
resource_id TEXT NOT NULL
before_state JSONB
after_state JSONB
metadata JSONB                -- reason, request_id, ip_address, etc.
occurred_at TIMESTAMPTZ DEFAULT now()
```
No `updated_at`/`deleted_at` — append-only by design.

### Audit Actions (naming convention: domain.resource.action)
- `auth.role.assign`
- `billing.invoice.issue` / `billing.invoice.write_off` / `billing.invoice.void`
- `billing.refund.create`
- `meet.entry.override` / `meet.entry.approve` / `meet.commitments.lock`
- `performance.best_time.override`
- `import.conflict.resolve`
- `members.swimmer.update`
- `compliance.data_export.request` / `compliance.data_deletion.request`

### Compliance Tables
**data_export_requests**: id, org_id, household_id, requested_by, status (pending|processing|ready|failed|expired), download_url, requested_at, completed_at, expires_at

**data_deletion_requests**: id, org_id, household_id, requested_by, status (pending|reviewing|approved|processing|completed|rejected), reason, requested_at, completed_at, review_notes

### Handler Injection Pattern
- Add `*audit.Service` to `BillingHandlers`, `MeetsHandlers`, `PerformanceHandlers`, `AuthHandlers`, `ImportsHandlers`, `MembershipHandlers`.
- Add constructor variant `NewXxxHandlersWithAudit(svc, auditSvc)` or just augment existing constructors.
- Simplest: augment constructors with optional `*audit.Service` field — set nil-safe.

### Retention Policy Config
- `AUDIT_RETENTION_DAYS` (default 2555 ≈ 7 years)
- `COMPLIANCE_EXPORT_TTL_DAYS` (default 30)
- `COMPLIANCE_DELETION_REVIEW_DAYS` (default 30)

## Endpoint Summary
| Method | Path | Roles |
|--------|------|-------|
| GET | /api/v1/audit-events | owner, admin |
| POST | /api/v1/compliance/data-export-requests | owner, admin, parent (own household) |
| GET | /api/v1/compliance/data-export-requests | owner, admin |
| GET | /api/v1/compliance/data-export-requests/{requestID} | owner, admin, requester |
| POST | /api/v1/compliance/data-deletion-requests | owner, admin, parent (own household) |
| GET | /api/v1/compliance/data-deletion-requests | owner, admin |
| GET | /api/v1/compliance/data-deletion-requests/{requestID} | owner, admin, requester |

## Progress
Started: 2026-03-16
