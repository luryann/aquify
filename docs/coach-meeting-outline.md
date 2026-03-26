# Coach Meeting — DARE Platform Overview

## 1. What Is DARE?
- Full-stack swim club operations platform
- Built to replace TeamUnify / SportsEngine Motion
- Covers everything: members, meets, entries, billing, communications, performance tracking

---

## 2. What's Already Built

### Member & Household Management
- Swimmer and household records, group assignments
- Parent portal with calendar, communications inbox
- Swimmer portal with performance history, profile, schedule

### Meet Management
- Full meet lifecycle: draft → published → commitment open → entry finalized → completed
- Commitment tracking per swimmer per meet
- Coach event selection and override before entry lock
- Entry creation, approval, and SD3/CL2 export for submission

### File Interoperability
- Import SD3, CL2, ZIP, HY3 meet files
- Export SD3 (standard and extended with seed times)
- Swimmer matching by ID or name + DOB + gender fallback

### Performance Tracking
- Best times per swimmer/event
- Coach override with full audit log
- Cut qualification support (wiring in progress)

### Billing
- Stripe integration (invoices, refunds, webhook reconciliation)
- Parent portal invoice view

### Communications
- Announcement/campaign system with role-based recipient targeting
- Parent inbox and notification preferences

### Admin Infrastructure
- Role-based access: owner > admin > coach > billing > parent > swimmer
- Full audit trail on all privileged actions
- GDPR compliance: data export and deletion requests
- Background job queue (imports, billing, email, reconcile)
- OpenTelemetry tracing, Prometheus metrics, structured logging

### Auth & Security
- Keycloak OIDC (self-hosted), full user lifecycle management from admin dashboard
- Invite users, assign roles, reset passwords, disable/delete accounts

### Deployment
- Dockerized, CI/CD via GitHub Actions (staging auto-deploy, production gated)
- Backup/restore runbooks, disaster recovery, smoke tests
- Load tests (k6) and accessibility audit completed

---

## 3. What's Still In Progress

### High Priority (nearly done)
- **Stripe/Postmark real credentials** — provider stubs need to be swapped for live in staging
- **Schema drift fixes** — a handful of DB columns referenced in code need migrations applied
- **User management UI** — backend is complete; admin UI pages still need building

### Backend Wiring Gaps
- Parent and swimmer roles need scoped API endpoints (currently hitting org-wide routes that deny them)
- Public registration → admin approval lifecycle needs real persistence (currently simulated)
- Portal registration wizard uses hardcoded season/waiver data instead of live APIs

### Frontend Placeholder Flows (need real API hookup)
- Portal invoice checkout (currently shows an alert instead of Stripe redirect)
- Autopay enrollment (local-only toggle, not persisted)
- Swimmer profile bio save (disabled, "coming soon")
- Swimmer cut-qualification tab (placeholder, not wired to time standards)
- Meet files in parent/swimmer portal (placeholder panels)
- Communications notification preferences (local-only, not persisted)
- Practice scheduler page needs to consolidate with `/training` (live API)

### Production Hardening
- In-memory session/auth state needs persistent/shared storage for multi-instance
- Background jobs need durable Redis-backed queue (currently in-process)
- Request body size limits and pagination caps on list endpoints
- Token values should not be returned in auth response bodies
- E2E test suite (Playwright/Cypress) — planned, not yet implemented

### Real Data Migration
- Dry-run migration with anonymized data against staging (blocked until staging env ready)
- Two full parallel meet cycles before cutover
- Final production cutover and stabilization

---

## 4. What a Coach Cares About — Status

| Feature | Status |
|---|---|
| Meet scheduling and publishing | ✅ Done |
| Commitment tracking | ✅ Done |
| Event selection per swimmer | ✅ Done |
| Coach event override before lock | ✅ Done |
| SD3/CL2 export for submission | ✅ Done |
| Import results back in | ✅ Done |
| Performance / best times | ✅ Done (coach override audited) |
| Cut qualifications | 🔄 Backend ready, UI wiring in progress |
| Practice scheduling | 🔄 Partially live — consolidation needed |
| Swimmer profile details | 🔄 Display done, bio save in progress |
| Parent communications | ✅ Done (persistence wiring in progress) |
| Role-based access (coach vs. admin) | ✅ Done |

---

## 5. Key Differentiators vs. TeamUnify

- **You own the data** — no vendor lock-in, full export at any time
- **CL2/SD3/HY3 compatible** — works with existing meet management tools
- **Audit trail on everything** — every coach override, entry change, and billing action is logged
- **Modern stack** — fast, mobile-friendly, no legacy UI
- **Self-hosted** — data stays in your infrastructure

---

## 6. Questions to Expect

- *How do we migrate our existing data?* → SD3/CL2 import pipeline is ready; timing depends on staging environment
- *What happens during a meet weekend?* → Entries export via SD3; results import back in; performance auto-updated
- *Can coaches access it on mobile?* → Yes, responsive UI; swimmer/parent portals are mobile-first
- *Who manages user accounts?* → Admin dashboard — invite, assign roles, reset passwords without touching any external system
- *When can we go live?* → Staging validation and real-data dry-run are the remaining gate; production cutover is planned and scripted
