Absolutely — here is your plan reorganized into clearer, easier-to-follow sections, with **nothing added or removed** and only the order/structure changed.

---

# Full Stack AI Application Plan

## 1) Project Goal and Core Interoperability Priority

1. Import results from `.cl2`, `.sd3`, `.hy3`, and `.zip` containing those files ([SportsEngine: Import Meet Results](https://motion-help.sportsengine.com/en/articles/8538070-how-to-import-meet-results)).
2. Export meet entries as **Standard SD3** and **Extended SD3** ([SportsEngine: Generate Meet Entry File](https://motion-help.sportsengine.com/en/articles/8538091-how-to-generate-a-meet-entry-file)).
3. Support SDIF roster export/import workflows for IDs and Meet Manager interoperability ([SportsEngine: Preferred Name + SDIF/SD3 notes](https://motion-help.sportsengine.com/en/articles/8537636-how-to-use-a-preferred-name-in-the-athlete-s-profile)).
4. Handle update-times workflow from imported result files (`.cl2`/`.sd3`/`.hy3`) ([SportsEngine: Import Updated Meet Entry Times](https://motion-help.sportsengine.com/en/articles/8537981-how-to-import-updated-meet-entry-times)).

Inference: if we nail those 4, you’ll cover the core “works natively with TeamUnify ecosystem” expectation for most clubs.

---

## 2) Scope, Success Metrics, and Guardrails

### 1) Scope, Success Metrics, and Guardrails

1. Define release boundaries:

* `v1 Must-Have`: required for cutover.
* `v1.1`: post-cutover enhancements.
* `Out of Scope`: explicitly deferred items.

2. Define measurable KPIs:

* Meet-file import success rate target (for example: >= 99% of valid files parse cleanly).
* Billing reconciliation accuracy target (for example: >= 99.9% invoice/payment parity).
* Time-to-resolve import errors SLA (for example: < 1 business day).
* Admin task completion benchmarks (registration, meet setup, fee posting).

3. Define go/no-go gates:

* No cutover unless all P0 acceptance tests pass for 2 consecutive meet cycles.
* No cutover unless billing totals reconcile against legacy for agreed periods.

---

## 3) Required Product Features

## Required Features (MVP Replacement)

1. Authentication and roles (admin, coach, parent, swimmer read-only).
2. Member system with swimmer IDs, DOB, gender, contacts, billing group, roster group.
3. Team/groups management with eligibility rules.
4. Meet/event management with commitments and entries.
5. Meet file engine:

* Import parser for `.sd3`, `.cl2`, `.hy3`, `.zip`.
* Export generator for Standard SD3 + Extended SD3.
* Validation and error reporting (missing swimmer IDs, unknown athletes, bad event codes).

6. Time standards / best-times store and automatic update from results imports.
7. Billing basics (dues, fees, invoice status, payment records).
8. Communications (email + announcement center).
9. Audit logs and change history.
10. Admin reporting (attendance, meet fees, membership, aging-out, registration status).

### Parent/Swimmer Portal Requirements (Launch-Critical Clarification)

1. Account policy:

* Parent account is mandatory for all household operations and enrollment.
* Swimmer account is optional and enabled per swimmer/household preference.
* No workflow may require a swimmer login to complete registration, commitments, or payment.

2. Parent portal (required in v1):

* Household dashboard with urgent alerts (payment due, commitment deadlines, missing forms).
* Registration and enrollment workflow with waiver acceptance.
* Household and swimmer profile management (field-level restrictions).
* Meet commitment workflow (yes/no per swimmer) with deadline visibility.
* Billing center (invoices, payment status, receipts, credits/refunds visibility).
* Announcements and communication center.
* Document center (uploaded forms, policy acknowledgements, compliance status).
* Calendar/schedule view across practices, meets, and team events.

3. Swimmer portal (optional in v1):

* Read-only by default, with optional limited self-service fields if enabled by policy.
* Dashboard for schedule, commitments status, announcements, and deadlines.
* Performance view for best times, results history, and standards/cut status.
* Visibility respects household/coach privacy settings and age-based policy controls.

4. Non-functional requirements for both:

* Mobile-first behavior for parent-centric workflows.
* Full WCAG 2.2 AA accessibility compliance.
* Audit logging for sensitive data/profile edits and commitment/payment-impacting actions.
* Explicit role and resource scoping across household boundaries.

**Required Feature Set (TeamUnify-replacement baseline)**

1. Identity and access:

* Roles: owner, admin, coach, parent, swimmer.
* Fine-grained permissions by module.
* MFA for privileged users.

2. Membership and households:

* Household records, emergency contacts, medical flags.
* Swimmer profile, team IDs, registration status.
* Group assignment and eligibility tags.

3. Meet operations:

* Meet setup, sessions, events, qualifying rules.
* Commitments and scratch workflow.
* Parent event selection per swimmer after `commit yes`.
* Coach edit/override of parent event selections with audit trail.
* Entry approval and relay management.
* Export SD3 (standard + extended).

4. Meet file interoperability:

* Import `.sd3`, `.cl2`, `.zip`.
* Parse + validate + reconcile against swimmers/events.
* Parse meet event definitions and validate swimmer-event compatibility.
* Import updated entry times from results files.
* Produce detailed import report with fixable errors.

5. Times and performance:

* Best times by course/event.
* Time standards and cut checks.
* Historical results and progression views.

6. Billing:

* Dues, one-off fees, meet fees.
* Invoices, payment status, refunds.
* Stripe webhook-driven payment state updates.

7. Communications:

* Announcement feed.
* Bulk and segmented email.
* Audit trail of sends.

8. Compliance and operations:

* Audit logs for sensitive actions.
* Retention policy and export/delete workflows.
* Backups + restore drills.

---

## 4) Feature Docs To Copy

# Feature Docs To Copy

## TeamUnify / SportsEngine Motion features to match

1. Member management (households, profiles, roster groups).
2. Website management / integrated site tools.
3. Financial tools: automated billing, meet fees, refunds/workflows.
4. Events & meet management: meet scheduling, declarations/commitments, entry approvals.
5. Communication hub: email, text/push, roster-group messaging.
6. Class management + attendance monitoring + self check-in.
7. Coaching toolkit + performance tracking (times/progression).
8. Volunteer/job signup tracking.
9. Mobile app workflows for attendance, comms, meet results visibility.
10. Meet file interoperability:

* Import results from `.cl2`, `.sd3`, `.hy3`, and `.zip` containing those
* Export **Standard SD3** and **Extended SD3**
* SDIF roster export
* Update future meet entry times from imported results
* Team-code and swimmer-ID based import logic
* Meet fee generation from results/imports

## Commit Swimming features to match

1. Workout builder/parser (structured sets, stroke/distance/intensity analytics).
2. Workout sharing (staff/swimmers/public link), favorites, season planning.
3. Meet entries & results workflow:

* Add host event file
* Invite/commit athletes (coach or parent workflow)
* Entry management using time history
* Entry-file export

4. Team management: registrations, billing, roster, volunteers, website.
5. Stripe-powered billing:

* Program registration
* One-off and recurring invoices
* Coupons
* Balance adjustments
* Refunds
* Transaction history/report export

6. Messaging:

* Email + in-app + optional push
* Read/open tracking
* Contextual sends (from meet/event)
* Drafts/autosave

7. Reports & analytics:

* Top times
* Swimmer progression
* Training analytics
* Attendance reporting
* Meet entry reports
* Financial reports

8. Roster management:

* Searchable unified roster
* Custom fields
* Profile edits
* Best-time overrides
* SWIMS sync

9. Volunteer tracking:

* Events/sessions/jobs
* Signup portal
* Credit/hour reporting

---

## 5) Architecture, Stack, and Repository Shape

## Technical shape for this repo

1. Keep current public pages.
2. Add `server/` (Go API), `db/migrations/`, `internal/meetfiles/`.
3. Add admin app (`/admin`) that calls API.
4. Add background jobs for imports, exports, emails, and nightly integrity checks.

**Recommended Stack**

1. Frontend (public site): keep current static HTML/CSS/JS.
2. Admin app: `Next.js (TypeScript) + Tailwind + shadcn/ui`.
3. Backend API: `Go 1.24 + Chi router + sqlc + pgx`.
4. Database: `PostgreSQL 16`.
5. Cache/queue: `Redis` (jobs, rate limits, short-lived caches).
6. Background jobs: `Asynq` (Go + Redis).
7. Auth: `Keycloak` (self-hosted) or `Auth0` (managed).
8. Object storage: `S3-compatible` (imports, exports, report files).
9. Email: `Postmark` (transactional), optional `SendGrid` fallback.
10. Payments: `Stripe`.
11. Observability: `OpenTelemetry + Grafana/Loki/Tempo` (or Datadog if managed).
12. Hosting: `Fly.io/Render` for app, managed Postgres/Redis, Cloudflare in front.

Why this combo:

1. Go + Postgres is stable and fast for operations software.
2. Next.js admin gives faster UI iteration.
3. sqlc keeps SQL explicit and safer than heavy ORMs.
4. Redis+Asynq is simple and robust for parsing and file workflows.

## Full Implementation Plan (Feature + Tech)

### 1) Final stack choice

1. Public site: keep current static pages.
2. Admin/portal UI: `Next.js (TypeScript, App Router)`, `Tailwind`, `shadcn/ui`, `TanStack Query`, `React Hook Form`, `Zod`.
3. API: `Go 1.24`, `Chi`, `sqlc`, `pgx`, `go-playground/validator`.
4. DB: `PostgreSQL 16` with migrations via `goose`.
5. Jobs: `Redis + Asynq`.
6. Files: `S3-compatible storage` (AWS S3 or Cloudflare R2).
7. Auth: `Auth0` (fastest secure managed path) or `Keycloak` (self-hosted control).
8. Messaging: `Postmark` (email), `Twilio` (SMS), `FCM` (push).
9. Payments: `Stripe` (+ webhook workers).
10. Observability: `OpenTelemetry`, `Prometheus/Grafana`, `Loki`.
11. Edge/security: `Cloudflare` (WAF, CDN, rate-limit).

### 2) Feature-to-technology mapping

| Feature               | Core services                            | Key tech choices                                      |
| --------------------- | ---------------------------------------- | ----------------------------------------------------- |
| Registration engine   | Programs, waivers, waitlists, discounts  | Go API + Postgres + Stripe + DocuSign/embedded e-sign |
| Parent/swimmer portal | Dashboard, commitments, invoices, docs   | Next.js portal + TanStack Query + RBAC                |
| Coach operations      | Attendance, group moves, workout planner | Next.js coach UI + Asynq jobs + analytics tables      |
| Volunteer management  | Shifts, hours, credits/penalties         | Scheduling tables + rules engine + billing link       |
| Compliance center     | Form expirations, checks, reminders      | Policy tables + reminders worker + audit logs         |
| Messaging             | Email/SMS/push + receipts                | Postmark + Twilio + FCM + delivery event tables       |
| Meet-day ops          | Scratch, relay, lock deadlines           | Meet workflow engine + role checks + reporting        |
| Data quality          | Dedupe, merge, conflict resolution       | Matching service + review queue + admin tooling       |
| Advanced reporting    | Finance, roster, retention               | Materialized views + scheduled jobs + CSV export      |
| Admin productivity    | Bulk tools, templates, automations       | CSV/SDIF import jobs + template service               |
| Reliability/DR        | retries, DLQ, restore                    | Asynq retry/DLQ + backup automation + runbooks        |
| Support/governance    | tickets, impersonation, flags            | Helpdesk integration + audited impersonation          |
| Mobile experience     | quick actions + notifications            | PWA first (Next.js), then React Native optional       |
| Legal/privacy         | consent versioning, retention            | Consent tables + deletion/export workflows            |
| Integrations          | accounting, SWIMS, webhooks              | Signed webhooks + connector jobs                      |

**Portal Scope Rule (authoritative)**

1. Parent portal is required at launch and is the operational self-service surface.
2. Swimmer portal is optional at launch and can be disabled without blocking go-live.
3. Parent workflows must remain complete even when swimmer logins are disabled.
4. Parent commitment `yes` flow must support per-swimmer event selection before coach review.
5. Coaches may change swimmer event selections before entry lock with mandatory audit logging.

### 4) Repo implementation shape

1. `server/` Go API and workers.
2. `admin/` Next.js app for admins/coaches.
3. `portal/` Next.js app for parents/swimmers (or combined with `admin` by role).
4. `db/migrations/` schema + seed + views.
5. `internal/meetfiles/` SD3/CL2/SDIF/HY3 parser/exporter.
6. `internal/billing/` ledger + Stripe integration.
7. `internal/comms/` email/SMS/push providers.
8. `infra/` Docker, compose, deploy manifests, monitoring configs.

---

## 6) Core Domain Model and API Design

**Core Domain Model (minimum tables)**

1. `users`, `roles`, `user_roles`, `sessions`.
2. `households`, `household_members`, `contacts`.
3. `swimmers`, `groups`, `swimmer_groups`.
4. `meets`, `meet_sessions`, `meet_events`.
5. `entries`, `entry_legs`, `commitments`.
6. `results`, `splits`, `best_times`.
7. `invoices`, `invoice_items`, `payments`, `refunds`.
8. `announcements`, `email_campaigns`, `email_deliveries`.
9. `file_imports`, `file_import_errors`, `file_exports`.
10. `audit_logs`, `webhook_events`.

**API Modules (v1)**

1. `/auth` login, refresh, logout, MFA.
2. `/members` households/swimmers/contacts.
3. `/groups` CRUD + assignment endpoints.
4. `/meets` meet/session/event CRUD.
5. `/entries` commitments, event selections, entries, approvals.
6. `/commitments/event-selections` parent event picks and coach overrides.
7. `/results/import` upload + async parse + reconciliation.
8. `/exports/sd3` generate and download.
9. `/billing` invoices, fees, webhook sync.
10. `/comms` announcements and email sends.
11. `/admin/audit` immutable audit query.
12. `/portal/parent` parent dashboard, household, commitments, billing, docs, notifications.
13. `/portal/swimmer` optional swimmer dashboard and performance endpoints (feature-flagged).

---

## 7) Identity, Permissions, Tenant Model, and Data Governance

### 5) Identity and Permission Matrix

1. Define role model:

* Owner, admin, coach, parent, swimmer, billing-only, read-only staff.

2. Define resource-level permissions by module:

* Members, meets, entries, billing, communications, reports, system settings.

3. Add privileged-action controls:

* MFA required for sensitive actions.
* Step-up auth for payouts/refunds/permission changes.
* Break-glass admin flow with mandatory reason logging.

### 6) Tenant and Org Model Decision

1. Decide now:

* Single-club architecture or multi-tenant architecture.

2. Document impact:

* Database keys and indexes.
* Auth claims and authorization scope.
* Data isolation tests and reporting boundaries.

3. Add migration path if starting single-club:

* Reserved tenant columns and backward-compatible API design.

## 3) Data Governance - Implementation

1. Add source-of-truth matrix per field:

* Member profile, billing, results, meet entries, compliance docs.

2. Add precedence policy:

* “Imported results override best-times; manual override requires reason + audit.”

3. Add data lifecycle:

* Retention/deletion schedule and legal-hold flagging.

---

## 8) Interoperability, Matching, and Certification

### 3) Interop Certification Test Pack

1. Build anonymized fixture library:

* Valid `.sd3`, `.cl2`, `.hy3`, `SDIF`, `.zip`.
* Malformed files and edge cases.
* Mixed course/event edge samples.

2. Add automated validation:

* Golden-file tests for import parsing.
* Round-trip tests for SD3/SDIF generation.
* Regression suite pinned to known production fixtures.

3. Define certification checklist:

* “TeamUnify-compatible” pass criteria per file type and workflow.

## 1) Interop Specs (SD3/CL2/SDIF/HY3) - Implementation

1. Deliverables:

* `docs/interop/sd3.md`, `docs/interop/cl2.md`, `docs/interop/sdif.md`, `docs/interop/hy3.md`
* Record-level field map, required/optional flags, normalization rules, and error codes.

2. Engineering:

* Parser packages under `internal/meetfiles/{sd3,cl2,sdif}` with HY3 ingest orchestration in `internal/meetfiles`.
* Versioned schema validators and golden fixtures.

3. QA gate:

* “Meet Manager import passes without manual edits” certification checklist.

## 2) Canonical Matching Rules - Implementation

1. Deterministic order:

* Team code + swimmer ID
* Governing ID (if present)
* Exact name + DOB + gender
* Fuzzy candidate queue (never auto-merge)

2. Conflict handling:

* `match_conflicts` table with status, reason, reviewer, resolution audit trail.

3. QA gate:

* No nondeterministic imports on repeated runs.

---

## 9) Security, Compliance, and Legal

**Security Baseline**

1. OIDC/OAuth2 with short JWT access tokens + rotating refresh.
2. RBAC middleware at route and resource level.
3. Row-level ownership checks (household-scoped data).
4. CSRF protection for cookie-auth admin flows.
5. Strict input validation and file-type/content sniffing.
6. AV scan pipeline for uploaded files.
7. Secrets in managed vault; no `.env` secrets in repo.
8. TLS everywhere, HSTS, CSP, secure cookies.
9. Rate limiting on auth/import endpoints.
10. Encrypted DB backups and tested restore every month.

### 8) Security and Compliance Operations

1. Security baseline implementation:

* OIDC, short-lived tokens, rotation, RBAC, CSRF, rate limiting.

2. Operational security:

* Threat model per module.
* Quarterly dependency and vulnerability review.
* Annual external pen test (or equivalent).

3. Compliance controls:

* PII classification and retention policy.
* Data export/delete workflows for user requests.
* Key management and rotation cadence.

4. Incident readiness:

* Incident response runbook.
* Breach communication checklist.
* Post-incident review template.

### Applicable compliance baseline (CA + federal)

1. CCPA/CPRA program (if thresholded business, or if you choose to implement rights flows proactively).
2. CalOPPA privacy policy requirements for commercial online services collecting CA resident data.
3. California breach law workflow (Civil Code 1798.82), including 30-day resident notice timeline and AG sample submission for >500 CA residents.
4. AB 506 youth service organization controls (training/background checks/policies) for staff and regular volunteers.
5. COPPA controls for under-13 handling if you have actual knowledge or child-directed flows.
6. TCPA/CAN-SPAM consent + opt-out controls for SMS/email.

## 8) Contract / Legal Pack - Implementation

1. Publish:

* Terms, Privacy Policy, Cookie/Tracking notice, DPA template.

2. Vendor due diligence:

* DPAs with Auth, Stripe, email/SMS, hosting vendors.

3. Rights workflows:

* Access/delete/correct request intake + verification + fulfillment logs.

## CA/LA Compliance Workstream (cross-cutting)

### A) CCPA/CPRA controls

1. Build privacy request center:

* Access, delete, correct, opt-out request intake and status tracking.

2. Honor opt-out preference signals where applicable.
3. Add risk-assessment/audit readiness artifacts if your org meets regulated thresholds.

### B) CalOPPA controls

1. Conspicuous privacy policy link on all public pages.
2. Policy must list categories collected/shared and update-effective-date handling.

### C) CA breach response (1798.82)

1. Incident runbook with 30-day notification clock start from discovery.
2. Generate CA-compliant notice template sections automatically.
3. If >500 CA residents affected, auto-task AG sample notice submission.

### D) AB 506 (youth org)

1. Compliance module for administrators/employees/regular volunteers:

* Training status
* Live Scan/background-check status
* Renewal/reverification reminders

2. Two-mandated-reporter policy acknowledgment workflow.
3. Prevent assignment of non-compliant adults to child-supervision roles.

### E) COPPA/TCPA messaging safeguards

1. Under-13 handling:

* Parent consent flow or strict no-collection path.

2. SMS:

* Explicit consent capture, STOP/HELP handling, consent logs.

3. Email:

* Unsubscribe and suppression list enforcement.

---

## 10) Billing and Financial Reconciliation

### 9) Billing and Financial Reconciliation Design

1. Use immutable financial events:

* Ledger-style transaction records for all billing changes.

2. Ensure idempotency:

* Stripe webhook dedupe and replay safety.

3. Define reconciliation workflow:

* Daily auto-reconcile jobs.
* Monthly finance-close report with discrepancy queue.

4. Support corrections safely:

* Credit memo, refund, reversal workflows with audit trail.

## 4) Pricing / Cost Model - Implementation

1. Create `docs/finance/unit_economics.md`:

* Infra, email/SMS, storage, support, payment fees.

2. Add budget alerts:

* Cloud spend, messaging spend, webhook failure costs.

3. Monthly review:

* Actual vs projected cost per swimmer/household.

---

## 11) Non-Functional Requirements, Observability, and Operations

### 7) Non-Functional Requirements (NFRs)

1. Reliability:

* Uptime SLO for API/admin.
* Job completion SLOs for imports/exports.

2. Performance:

* API latency targets (p50/p95/p99).
* Import processing time limits by file size.

3. Scale:

* Expected households/swimmers/meets per year.
* Peak concurrency scenarios (registration open, meet deadline).

4. Data durability:

* Backup frequency, retention windows, restore-time objectives.

### 11) Observability and Supportability

1. Monitoring:

* Metrics, traces, structured logs for API/jobs/webhooks.

2. Alerting:

* Pager thresholds for import failures, payment sync errors, auth anomalies.

3. Support tooling:

* Admin diagnostics page with job history, error details, retry action.

4. Operational runbooks:

* “Import failed,” “webhook delayed,” “billing mismatch,” “mail provider outage.”

## 7) SLA / Support Policy - Implementation

1. Severity model:

* P0 (billing/auth/data loss), P1 (import blocked), P2/P3.

2. Targets:

* Response and resolution time objectives per severity.

3. Support tooling:

* In-app ticket links, incident banner, status page updates.

## 5) Staffing Plan - Implementation

1. Minimum team:

* Product owner, 1 backend, 1 frontend, 1 QA/ops shared.

2. On-call:

* Primary + secondary rotation for imports, billing, auth incidents.

3. Ownership map:

* Module owner per epic with backup owner.

---

## 12) Acceptance Criteria, QA, Release, and Rollback

### 2) Acceptance Criteria by Epic

1. Add testable acceptance criteria for every feature.
2. Examples:

* SD3 export imports into Meet Manager without manual edits.
* CL2/SD3/HY3 result import produces deterministic matching with conflict queue for ambiguous records.
* Role permissions deny unauthorized read/write with audit evidence.
* Invoice lifecycle supports create, partial pay, full pay, refund, and void with immutable history.

### 10) QA, Release, and Rollback Strategy

1. CI quality gates:

* Unit + integration + e2e + fixture interoperability tests.

2. Environment strategy:

* Dev, staging (production-like), production.

3. Release controls:

* Feature flags for high-risk modules.
* Canary rollout for parser/billing changes.

4. Rollback plan:

* DB rollback policy, job drain protocol, versioned file format compatibility.

## 9) Accessibility / UX Standards - Implementation

1. Standard:

* WCAG 2.2 AA for admin + parent portals.

2. Testing:

* Keyboard-only, screen reader, color contrast, mobile low-bandwidth checks.

3. Release gate:

* Accessibility checklist required in PR template.

---

## 13) Delivery Plan, Milestones, and Execution Order

## Build Plan (phased)

1. Phase 0: Foundation (2–3 weeks)

* Stand up Go API + Postgres.
* Keep existing static site; add `/api`.
* Add auth, roles, migrations, audit log framework.

2. Phase 1: Core Club Ops (3–5 weeks)

* Members, households, groups, registrations.
* Admin UI for CRUD + CSV import/export.
* Permission matrix finalized.

3. Phase 2: Meets + Entries (4–6 weeks)

* Meet creation, commitments, relay planning basics.
* Entry approval pipeline.
* Export Standard/Extended SD3.

4. Phase 3: Results + Parsing (4–6 weeks)

* Import `.cl2/.sd3/.zip`.
* Reconciliation engine (match by swimmer ID + team code).
* Update best times + meet history.
* Validation dashboard with fix/retry workflow.

5. Phase 4: Billing + Comms (3–5 weeks)

* Invoicing, fee batches, payment tracking.
* Email templates, notices, announcement feed.

6. Phase 5: Parallel Run + Cutover (4–8 weeks)

* Run both systems during one full meet cycle.
* Compare outputs (entries, results, fees).
* Data freeze + final migration + go-live.

**Delivery Plan (realistic)**

1. Milestone 1 (4 weeks): auth, members, groups, admin shell.
2. Milestone 2 (5 weeks): meet setup, commitments, entries, SD3 export.
3. Milestone 3 (6 weeks): `.sd3/.cl2/.zip` import + reconciliation + best-times update.
4. Milestone 4 (4 weeks): billing + email + audit tooling.
5. Milestone 5 (4 weeks): parallel-run with current platform, gap fixes, cutover.

## Full Implementation Plan (Feature + Tech)

### 3) Delivery phases (recommended)

1. Phase 0 (2 weeks): platform foundation

* Monorepo layout, CI/CD, Auth0, base RBAC, Postgres schema core, audit logging.

2. Phase 1 (4 weeks): members + portal basics

* Households/swimmers/groups, parent dashboard, coach dashboard starter, document center.

3. Phase 2 (5 weeks): meets + interoperability core

* Meet setup, commitments/entries, SD3 export, CL2/SD3/ZIP import pipeline, conflict queue.

4. Phase 3 (4 weeks): registration + billing

* Program registration, waivers, invoices, Stripe checkout, webhook reconciliation.

5. Phase 4 (4 weeks): messaging + volunteer + compliance

* Email/SMS/push campaigns, volunteer jobs/hours, form expiration alerts.

6. Phase 5 (3 weeks): reporting + admin productivity

* Financial/attendance/retention reports, saved filters, bulk actions, scheduled exports.

7. Phase 6 (3 weeks): reliability + cutover

* Load tests, backup/restore drills, incident runbooks, parallel-run with legacy, go-live gates.

## Execution order (recommended)

1. Security/compliance foundations first (A-D + incident runbook).
2. Interop and matching engine.
3. Billing and reconciliation.
4. Training/support and decommission prep.

---

## 14) Backlog and Delivery Management

### 12) Execution Backlog Structure

1. Convert plan into delivery artifacts:

* Epics -> stories -> tasks.

2. Add planning fields:

* Estimate, owner, dependency, risk level, acceptance criteria.

3. Mark critical path:

* Meet interoperability, billing integrity, migration readiness.

4. Add milestone exit criteria:

* Every phase needs explicit “definition of done.”

---

## 15) Migration, Cutover, and Decommission

### 4) Data Migration Plan

1. Build source-to-target mapping matrix:

* Source field, target field, transform rule, nullable policy, owner.

2. Execute migration phases:

* Discovery export, dry-run import, reconciliation report, production run, post-check.

3. Add rollback strategy:

* Snapshot before migration, reversible migration scripts, freeze window protocol.

4. Define cutover reconciliation:

* Member counts, active registrations, unpaid balances, best-times parity checks.

## Cutover Readiness Checklist (Final Gate)

1. Two real meet cycles run in parallel with no P0 defects.
2. SD3/CL2/SDIF/HY3 workflows pass certification suite.
3. Billing reconciliation meets target thresholds.
4. Migration dry-run and production run both reconciled.
5. Security controls validated and incident runbook tested.
6. Backup restore drill succeeds within RTO/RPO targets.
7. Support team runbooks validated in staging simulation.

## 10) Legacy Decommission Plan - Implementation

1. Parallel run:

* At least 2 meet cycles and 1 billing cycle.

2. Final migration:

* Freeze window, reconciliation report, sign-off checklist.

3. Archive:

* Read-only export of legacy records with retrieval process.

---

## 16) Launch Readiness and Replacement Criteria

**Definition of “Ready to Replace”**

1. 2 full meet cycles processed without manual data patching.
2. Billing/invoice totals match legacy outputs within agreed tolerance.
3. All critical workflows have role tests and audit logs.
4. Backup restore test passes in staging and production.

### 5) Must-pass launch criteria

1. Two meet cycles completed with no manual meet-file patching.
2. Billing reconciliation variance below agreed threshold.
3. P0 security checklist passed (MFA, RBAC, audit, backups, restore test).
4. Parent, coach, and admin critical workflows pass end-to-end tests.
5. Support team can resolve import/billing incidents using runbooks.

---

## 17) Training, Change Management, and Support Readiness

## 6) Change Management / Training - Implementation

1. Training tracks:

* Admin, coach, parent quick-start guides and videos.

2. Rollout:

* Pilot group first, then full-club rollout with office hours.

3. SOPs:

* Meet import SOP, billing correction SOP, refund SOP, incident SOP.

---

## 18) Risk Register

## Risk Register (Add As a Permanent Section)

1. Meet-file parsing mismatch.
2. Swimmer identity reconciliation errors.
3. Billing correctness and refund edge cases.
4. Permissions leakage across households.
5. Migration quality issues.
6. Vendor dependency outages (email/payments/storage).
7. Performance bottlenecks on registration deadlines.
8. Incomplete audit coverage for compliance events.

---

## 19) Sources

## Sources

* [SportsEngine Motion swimming overview](https://www.sportsengine.com/motion/sports/swimming/)
* [SportsEngine Motion swim club page](https://www.sportsengine.com/motion/organization/swim-clubs/)
* [SportsEngine Motion swimming pricing/includes](https://www.sportsengine.com/motion/pricing/swimming/)
* [Import meet results (.cl2/.sd3/.zip)](https://motion-help.sportsengine.com/en/articles/8538070-how-to-import-meet-results)
* [Generate meet entry file (Standard/Extended SD3)](https://motion-help.sportsengine.com/en/articles/8538091-how-to-generate-a-meet-entry-file)
* [Import updated meet entry times](https://motion-help.sportsengine.com/en/articles/8537981-how-to-import-updated-meet-entry-times)
* [Preferred name + SDIF/SD3 notes](https://motion-help.sportsengine.com/en/articles/8537636-how-to-use-a-preferred-name-in-the-athlete-s-profile)
* [Generate meet fees from results](https://motion-help.sportsengine.com/en/articles/8538067-how-to-generate-meet-fees-from-results)
* [Commit Swimming homepage](https://www.commitswimming.com/)
* [Commit Swimming all features](https://www.commitswimming.com/product/all-features)
* [Commit workout management](https://www.commitswimming.com/product/workout-management)
