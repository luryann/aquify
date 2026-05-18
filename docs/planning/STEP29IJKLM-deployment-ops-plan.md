# STEP 29I–29M: Deployment, Ops, DR, Perf, and Training Plan

**Date:** 2026-03-23
**Status:** In progress
**Scope:** Steps 29I through 29M of the production-readiness backlog

---

## Overview

These steps cover everything needed to take the DARE platform from "code complete" to "production operable":

- **29I** – Deployment and environment automation
- **29J** – Domains, edge, and operational controls
- **29K** – Backup, restore, and disaster recovery
- **29L** – Performance, accessibility, and compatibility
- **29M** – Training, support, and launch simulation

**Hosting decision (locked):** Self-managed VPS fronted by Cloudflare.

---

## 29I: Deployment and Environment Automation

### Artifacts to create

1. **`deploy/docker-compose.staging.yml`** — Staging compose (full stack: API, workers, admin, Postgres, Redis, MinIO, Loki, Grafana, Prometheus, Keycloak, Mailpit)
2. **`deploy/docker-compose.production.yml`** — Production compose (same stack, hardened settings, no Mailpit)
3. **`deploy/systemd/`** — systemd service unit files for process supervision on bare VPS
4. **`deploy/secrets/README.md`** — Secrets management strategy (env files, Vault integration, secret rotation)
5. **`.github/workflows/deploy-staging.yml`** — CI → staging deploy workflow
6. **`.github/workflows/deploy-production.yml`** — Staging-gate → production deploy workflow
7. **`deploy/scripts/promote.sh`** — Promote staging → production script
8. **`deploy/scripts/rollback.sh`** — Rollback to previous release
9. **`deploy/README.md`** — Deployment overview and operator instructions

### Key decisions
- Use Docker Compose on VPS (simple, portable, matches local dev)
- GitHub Actions for CI/CD; staging deploy on every main merge; production deploy on manual trigger after staging passes
- Secrets via `.env.production` file (not committed) + optional Vault for rotation
- Process supervision via systemd `dare-api.service` wrapping `docker compose`
- Health-check gate before traffic switch

---

## 29J: Domains, Edge, and Operational Controls

### Artifacts to create

1. **`docs/ops/domain-plan.md`** — DNS/TLS/Cloudflare config for `www`, `admin`, `api`, `status`
2. **`docs/ops/cloudflare-waf-config.md`** — WAF rules, rate-limit rules, bot-fight mode settings
3. **`docs/ops/staging-environment-guide.md`** — How to stand up staging, credentials, smoke-test gate
4. **`deploy/scripts/rollback.sh`** — (shared with 29I) Rollback procedure
5. **`docs/ops/operator-config-surfaces.md`** — Which runtime settings are safe to change in-app vs require infra access

### Domain plan
- `dare.club` (example TLD; actual TLD is operator-chosen)
- `www.dare.club` → marketing/status page
- `admin.dare.club` → Next.js admin portal
- `api.dare.club` → Go API
- `status.dare.club` → Uptime status page (Uptime Kuma or BetterStack)

---

## 29K: Backup, Restore, and Disaster Recovery

### Artifacts to create

1. **`deploy/scripts/backup.sh`** — Automated backup: pg_dump + MinIO object export + compress + encrypt + upload to offsite
2. **`deploy/scripts/restore.sh`** — Restore from backup (DB + MinIO objects)
3. **`docs/ops/backup-restore-runbook.md`** — Retention windows, RTO/RPO, restore cadence
4. **`docs/ops/disaster-recovery-runbook.md`** — Full DR procedure including data loss scenarios
5. **`docs/ops/restore-drill-checklist.md`** — Checklist for quarterly restore drill in staging

### DR targets
- **RTO:** < 4 hours for full restore
- **RPO:** < 24 hours (daily backup), < 1 hour if WAL archiving is enabled
- **Retention:** 30 daily, 12 weekly, 3 monthly backups

---

## 29L: Performance, Accessibility, and Compatibility

### Artifacts to create

1. **`tests/load/k6/`** — k6 load test scripts:
   - `admin-browse.js` — Authenticated admin browsing load
   - `import-upload.js` — File import upload + finalize flow
   - `webhook-burst.js` — Stripe webhook burst throughput
   - `portal-load.js` — Concurrent parent portal sessions
2. **`docs/testing/load-test-plan.md`** — Targets, scenarios, pass/fail thresholds
3. **`docs/testing/accessibility-audit.md`** — WCAG 2.2 AA checklist for admin + portal critical flows
4. **`docs/testing/browser-compatibility-matrix.md`** — Browser/OS/mobile matrix + test results

### Load test targets
- API: p95 < 500ms, p99 < 2s at 50 concurrent users
- Import upload: p95 < 3s for 1MB file
- Webhook burst: 100 webhooks/second with zero drops
- Portal: 20 concurrent parent sessions with p95 < 800ms

---

## 29M: Training, Support, and Launch Simulation

### Artifacts to create

1. **`docs/training/operator-guide.md`** — How to operate DARE: deployments, config, DB management
2. **`docs/training/product-admin-guide.md`** — Product/ops guide: member management, billing, meets, imports
3. **`docs/training/finance-guide.md`** — Finance/billing operations guide
4. **`docs/training/support-runbook.md`** — Support incident playbook (import failures, auth issues, billing discrepancies, document-center issues)
5. **`docs/training/launch-owner-matrix.md`** — Named launch owners, escalation paths, on-call rotation
6. **`docs/training/manual-workflows-validation.md`** — End-to-end validation checklist for all manual-at-launch workflows

---

## Implementation Sequence

1. 29I — Deploy compose files, systemd units, CI/CD workflows, scripts
2. 29J — Domain plan, Cloudflare config, staging guide, rollback
3. 29K — Backup/restore scripts, DR runbook, drill checklist
4. 29L — k6 scripts, load test plan, accessibility audit, browser matrix
5. 29M — Training guides, support runbook, launch owner matrix

---

## Progress Tracking

- [x] Planning doc created
- [x] 29I artifacts complete
- [x] 29J artifacts complete
- [x] 29K artifacts complete
- [x] 29L artifacts complete
- [x] 29M artifacts complete
- [x] TODO.md updated
- [x] CLAUDE.md updated
