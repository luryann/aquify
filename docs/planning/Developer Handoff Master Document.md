# DARE Platform - Developer Handoff Master Document
Version: 1.0  
Date: March 15, 2026  
Owner: ____________________  
Status: Draft / Review / Approved (circle one)

---

## How To Use This Document
This is the single implementation packet for building the app end-to-end.  
Do not start feature coding until required pre-build sections are completed and approved.

---

## 0) Executive Summary

### Goal
Build a full-stack swim club operations platform that can replace TeamUnify/SportsEngine Motion workflows for DARE, including native meet-file interoperability and operational workflows for members, meets, billing, comms, and reporting.

### Primary interoperability requirements
1. Import results from `.cl2`, `.sd3`, `.hy3`, and `.zip` containing those files.
2. Export meet entries as Standard SD3 and Extended SD3.
3. Support SDIF roster export/import for Meet Manager interoperability.
4. Support update-times workflow from imported `.cl2/.sd3/.hy3` results.

### Launch definition
Production cutover is allowed only when all module signoff criteria are met and cutover checklist is green.

---

## 1) Locked Product Scope (PRD)

### 1.1 v1 Must-Have (Launch Blocking)
1. Authentication and role-based access control.
2. Households, swimmers, coaches, groups, registrations.
3. Meet setup, commitments, entry workflow.
4. SD3 export (standard + extended).
5. CL2/SD3/ZIP results import and reconciliation queue.
6. Best-times store and updates from imported results.
7. Billing core: invoices, payments, refunds, ledger.
8. Comms core: announcements + email.
9. Audit logging for privileged/sensitive actions.
10. Core admin reports (membership, meet fees, billing status).

### 1.1A Portal Scope and Account Policy (Launch Blocking)
1. Parent account is required for household operations and cannot be optional.
2. Swimmer account is optional at launch and controlled per household/policy.
3. Registration, commitments, billing, and document workflows must be fully completable via parent account only.
4. No production workflow may force swimmer login for launch-critical tasks.

### 1.1B Parent Portal Required Functional Inventory (v1)
1. Household dashboard with alerts:
   - unpaid invoices
   - upcoming meet commitment deadlines
   - missing/expiring documents
2. Household profile management:
   - contacts and emergency details
   - swimmer profile edits within allowed fields
3. Registration center:
   - program selection
   - waiver/policy acknowledgement
   - enrollment status
4. Meet center:
   - view meet invitations and deadlines
   - commit yes/no per swimmer
   - view commitment status and entry status
5. Billing center:
   - invoices, payments, balances, receipts
   - credits/refunds visibility
   - payment method handoff to Stripe checkout
6. Communications:
   - announcement feed
   - message history
   - notification preference controls
7. Document center:
   - required forms list
   - upload/status tracking
8. Calendar view:
   - practices, meets, and key household deadlines

### 1.1C Swimmer Portal Optional Functional Inventory (v1)
1. Optional account activation path:
   - swimmer login may be disabled globally or per household.
2. Swimmer dashboard:
   - schedule, announcements, deadlines, meet status.
3. Performance:
   - best times, historical progression, standards/cuts.
4. Profile:
   - read-only by default, limited edits only if policy allows.
5. Permissions:
   - must never expose sibling/other-household financial data.
   - must honor family and coach visibility controls.

### 1.2 v1.1 (Post-Launch)
1. SMS/push messaging.
2. Volunteer credits automation.
3. Advanced workout planning analytics.
4. Additional report packs and forecasting.
5. Expanded integrations.

### 1.3 Out of Scope (For Now)
1. Multi-club public SaaS onboarding.
2. Native mobile apps (PWA first).
3. Full website CMS replacement.
4. AI-driven training recommendations.

### 1.4 Manual-at-Launch Allowed
1. Manual review queue for ambiguous swimmer matching.
2. Manual finance review for flagged reconciliation discrepancies.
3. Manual override for edge-case imported records with required audit reason.

### 1.5 Go/No-Go Gates
1. Two complete meet cycles processed in parallel with no P0 defects.
2. Billing reconciliation passes threshold (>= 99.9% parity target).
3. Security controls and restore drill passed.
4. Signoff from Product, Engineering, Operations, Finance.

---

## 2) Canonical Feature Registry (Single Source of Truth)

Use this table as the only feature truth source.

| Feature ID | Feature Name | Version | Priority | Owner | Dependencies | Acceptance Criteria |
|---|---|---|---|---|---|---|
| FEAT-001 | OIDC Auth + MFA | v1 | P0 |  | none | Admin/coach MFA enforced, secure sessions |
| FEAT-002 | RBAC + Resource Scoping | v1 | P0 |  | FEAT-001 | Access controls pass authorization test suite |
| FEAT-003 | Households + Swimmers | v1 | P0 |  | FEAT-002 | CRUD + import/export + audit logs |
| FEAT-004 | Groups + Eligibility | v1 | P0 |  | FEAT-003 | Eligibility rules correctly applied |
| FEAT-005 | Meet Setup | v1 | P0 |  | FEAT-003 | Meet/session/event workflows complete |
| FEAT-006 | Commitments + Entries | v1 | P0 |  | FEAT-005 | Parent/coach workflow passes e2e |
| FEAT-007 | SD3 Export Std/Ext | v1 | P0 |  | FEAT-006 | Meets Meet Manager import validation |
| FEAT-008 | CL2/SD3/HY3/ZIP Import | v1 | P0 |  | FEAT-003, FEAT-005 | Import parses and queues conflicts |
| FEAT-009 | SDIF Roster Import/Export | v1 | P0 |  | FEAT-003 | Round-trip tests pass |
| FEAT-010 | Best-Times + Update Times | v1 | P0 |  | FEAT-008 | Deterministic updates + audit trail |
| FEAT-011 | Billing Core + Ledger | v1 | P0 |  | FEAT-003 | Invoices/payments/refunds accurate |
| FEAT-012 | Stripe Integration | v1 | P0 |  | FEAT-011 | Idempotent webhook handling |
| FEAT-013 | Announcements + Email | v1 | P1 |  | FEAT-002 | Segment send + delivery tracking |
| FEAT-014 | Audit Log + Admin Audit UI | v1 | P0 |  | FEAT-002 | Immutable logs queryable |
| FEAT-015 | Core Reports | v1 | P1 |  | FEAT-003, FEAT-011 | Exports and filters validated |
| FEAT-016 | Volunteer Management | v1.1 | P2 |  | FEAT-003 | Job signup and hours tracking |
| FEAT-017 | SMS/Push Messaging | v1.1 | P2 |  | FEAT-013 | Consent and suppression honored |
| FEAT-018 | Advanced Analytics | v1.1 | P3 |  | FEAT-010, FEAT-015 | Performance targets met |

---

## 3) Final Tech Stack (Locked)

### Frontend
1. Public site: existing static HTML/CSS/JS.
2. Admin/portal: Next.js (TypeScript), Tailwind, shadcn/ui, TanStack Query, React Hook Form, Zod.

### Backend
1. Go 1.24 API with Chi.
2. sqlc + pgx for DB access.
3. Validation: go-playground/validator.
4. Workers: Asynq + Redis.

### Data and Storage
1. PostgreSQL 16.
2. S3-compatible object storage for file imports/exports/artifacts.

### Auth and Payments
1. Auth0 (managed) OR Keycloak (self-hosted).  
Decision: ____________________ by date ____________
2. Stripe for payment collection and refunds.

### Messaging
1. Postmark for transactional email.
2. Twilio/FCM optional in v1.1.

### Observability and Edge
1. OpenTelemetry + Prometheus/Grafana + Loki.
2. Cloudflare (TLS, WAF, CDN, rate-limits).

---

## 4) Roles and Permission Matrix (Endpoint + UI)

### 4.1 Roles
1. Owner
2. Admin
3. Coach
4. Parent
5. Swimmer (read-only)
6. Billing-only Staff
7. Read-only Staff

### 4.2 Permission Matrix (minimum)
| Action | Owner | Admin | Coach | Parent | Swimmer | Billing Staff | Read-only |
|---|---|---|---|---|---|---|---|
| View swimmer profile | Y | Y | Y | household only | self only | Y | Y |
| Edit swimmer profile | Y | Y | limited | household limited | N | N | N |
| View medical flags | Y | Y | Y | household only | N | N | N |
| Create meet | Y | Y | Y | N | N | N | N |
| Approve entries | Y | Y | Y | N | N | N | N |
| Import CL2/SD3/HY3/ZIP | Y | Y | Y (optional) | N | N | N | N |
| Export SD3 | Y | Y | Y | N | N | N | N |
| Create invoice | Y | Y | N | N | N | Y | N |
| Refund payment | Y | Y (step-up) | N | N | N | Y (step-up) | N |
| Send bulk comms | Y | Y | Y (group scoped) | N | N | N | N |
| View household financial history | Y | Y | N | household only | N | Y | N |
| Manage roles/permissions | Y | Y (limited) | N | N | N | N | N |

### 4.3 Enforcement
1. Deny by default on all API routes.
2. Resource scope checks required for every query.
3. Privileged actions require audit event with actor, reason, before/after diff.

---

## 5) Core User Flows (Step-by-Step)

### 5.1 Parent Registration Flow
1. Parent creates account.
2. Verifies email.
3. Creates household.
4. Adds swimmer(s).
5. Accepts waivers.
6. Selects program/group.
7. Sees invoice.
8. Pays (or marked pending).
9. Receives confirmation.

### 5.2 Meet Entry Flow
1. Admin/coach creates meet + events.
2. Coach publishes commitment request.
3. Parent commits yes/no per swimmer.
4. Parent selects events per swimmer when commitment is `yes`.
5. Coach reviews commitments and parent-selected events.
6. Coach edits/overrides event selections where needed (audit reason required).
7. Coach finalizes entries.
8. System validates eligibility/time standards and swimmer-event compatibility.
9. Export SD3 generated.
10. Audit trail created.

### 5.3 Results Import Flow
1. Admin uploads `.cl2/.sd3/.hy3/.zip`.
2. File validation and malware scan.
3. Async parse job starts.
4. Records matched by canonical rules.
5. Ambiguous records sent to conflict queue.
6. Admin resolves conflicts.
7. Best times and history updated.
8. Import report generated.

### 5.4 Billing Flow
1. Invoice generated (program/meet fees/manual charges).
2. Parent pays via Stripe or marked external payment.
3. Webhook updates payment state.
4. Reconciliation job confirms ledger consistency.
5. Refund/credit/void handled by authorized roles only.
6. Finance audit report available.

---

## 6) State Machines (Lifecycle Rules)

### 6.1 Meet
Draft -> Published -> Open for Commitments -> Entry Locked -> Submitted -> Completed -> Archived

### 6.2 Commitment
Not Requested -> Requested -> Committed Yes/No -> Closed

### 6.3 Entry
Draft -> Proposed -> Approved -> Exported -> Submitted -> Final

### 6.4 Invoice
Draft -> Issued -> Partially Paid -> Paid | Overdue | Voided

### 6.5 Payment
Pending -> Succeeded | Failed -> Refunded (partial/full)

### 6.6 Refund
Requested -> Approved -> Processed -> Settled | Failed

### 6.7 File Import
Uploaded -> Validating -> Parsing -> Matching -> Conflict Review -> Applied -> Completed | Failed

### 6.8 Conflict Resolution
Open -> In Review -> Resolved -> Applied -> Closed

### 6.9 Compliance Document
Not Submitted -> Submitted -> Verified -> Expiring Soon -> Expired -> Renewed

### Transition rules template (for each state machine)
1. Allowed actor(s):
2. Preconditions:
3. Side effects:
4. Notifications:
5. Audit event required:

---

## 7) Database Schema Specification (Implementation-Ready)

### 7.1 Required conventions
1. Every table has: `id`, `created_at`, `updated_at`, `created_by`, `updated_by`.
2. Soft delete where needed: `deleted_at`, `deleted_by`.
3. Tenant/org field strategy: `org_id` on all business tables.
4. Immutable financial/event tables use append-only design.

### 7.2 Minimum core tables (expand into SQL migrations)
1. Identity: `users`, `roles`, `user_roles`, `sessions`, `mfa_factors`
2. Membership: `households`, `contacts`, `swimmers`, `swimmer_groups`
3. Meets: `meets`, `meet_sessions`, `meet_events`, `commitments`, `entries`, `entry_legs`
4. Results: `results`, `splits`, `best_times`, `time_standards`
5. Interop: `file_imports`, `file_import_rows`, `file_import_errors`, `match_conflicts`, `file_exports`
6. Billing: `invoices`, `invoice_items`, `payments`, `refunds`, `ledger_events`, `webhook_events`
7. Comms: `announcements`, `email_campaigns`, `email_deliveries`, `suppression_list`
8. Compliance: `documents`, `document_requirements`, `background_checks`, `policy_acknowledgements`
9. Audit: `audit_logs`

### 7.3 Constraints checklist
1. Unique swimmer identity per org for key identifiers.
2. Unique invoice number per org.
3. Idempotency key uniqueness for webhook/event handling.
4. Foreign keys with explicit delete behavior.
5. Indexes on high-read and filter paths.

---

## 8) Data Ownership and Conflict Rules

### 8.1 Ownership model
1. Parent editable: contact info, non-locked profile fields.
2. Coach editable: group assignment, meet operational fields.
3. Admin editable: global and sensitive records.
4. Imported-only fields: raw result metadata from meet files.
5. System-generated: best-time derived values, ledger balances.

### 8.2 Canonical precedence
1. Official imported results override derived best times.
2. Manual override allowed only with reason + role.
3. Swimmer identity conflicts never auto-merge when confidence below threshold.
4. DOB changes after import trigger revalidation task.

### 8.3 Conflict audit
1. Who changed what.
2. Why.
3. Before/after values.
4. Linked import/export/job IDs.

---

## 9) Interoperability Specs and Fixture Pack

### 9.1 Deliverables
1. `docs/interop/sd3.md`
2. `docs/interop/cl2.md`
3. `docs/interop/sdif.md`
4. `docs/interop/hy3.md`
5. `fixtures/interop/` with valid and malformed files.
6. `fixtures/expected/` parsed JSON and expected export files.

### 9.2 Required test classes
1. Happy path import.
2. Unknown swimmer IDs.
3. Duplicate swimmers.
4. Missing fields.
5. Invalid event code.
6. ZIP mixed valid/invalid files.
7. Re-run idempotency.

### 9.3 Certification criteria
1. SD3 exports accepted by target Meet Manager without manual edits.
2. CL2/SD3 import runs deterministic matching.
3. SDIF roster round-trip verified.

---

## 10) Migration Packet

### 10.1 Inputs required
1. Legacy roster export samples.
2. Legacy billing exports.
3. Meet history and entries.
4. Results history samples.
5. Known bad-data samples.

### 10.2 Deliverables
1. Field mapping matrix.
2. Transform rules.
3. Dry-run scripts.
4. Reconciliation report templates.
5. Rollback procedure.

### 10.3 Migration gates
1. Dry-run parity meets threshold.
2. Production run verified.
3. Signoff by data owner and finance owner.

---

## 11) UI/UX Blueprint

### 11.1 Required screen inventory
1. Login + MFA
2. Admin dashboard
3. Parent dashboard
4. Swimmer profile
5. Household billing page
6. Meet setup
7. Commitments and entries
8. Import queue and conflict resolver
9. Invoice and payment pages
10. Comms composer
11. Reports hub
12. Audit log viewer
13. Parent commitments inbox with deadline status chips
14. Parent household documents/compliance center
15. Parent invoice history + receipt detail page
16. Optional swimmer dashboard (feature-flag controlled)
17. Optional swimmer performance page (best-times + progression)

### 11.2 Interaction requirements
1. Table behaviors: filters, sort, pagination, column visibility.
2. Standard form pattern with inline validation.
3. Empty states and guided next steps.
4. Error states with retry and support path.
5. Mobile-first responsive behavior for parent workflows.
6. Parent critical flows (commitment response + payment + form upload) must complete in <= 4 primary interactions from dashboard.
7. If swimmer portal is disabled, swimmer CTAs route to parent account context with clear explanatory copy.

### 11.3 Design system rules
1. Components: buttons/forms/tables/modals/toasts/badges.
2. Accessibility: WCAG 2.2 AA target.
3. Destructive actions need confirmation and reason capture where applicable.

---

## 12) API Contract (OpenAPI Required)

### 12.1 Standards
1. OpenAPI 3.1 spec in repo.
2. Versioned endpoints: `/api/v1`.
3. Consistent error shape.
4. Cursor or offset pagination standard.
5. Filter/sort conventions documented.
6. Idempotency for mutation endpoints that need it.

### 12.2 Required endpoint groups
1. `/auth`
2. `/members`
3. `/groups`
4. `/meets`
5. `/entries`
6. `/commitments/event-selections`
7. `/results/import`
8. `/exports/sd3`
9. `/billing`
10. `/comms`
11. `/admin/audit`
12. `/reports`
13. `/compliance`
14. `/portal/parent`
15. `/portal/swimmer` (optional, feature-flagged)

---

## 13) Background Job Specs

For each job define: trigger, inputs, idempotency key, retries, timeout, DLQ policy, alert threshold, UI status mapping.

### Required jobs
1. File import parse/match/apply.
2. SD3/SDIF export generation.
3. Email send and delivery sync.
4. Stripe webhook reconciliation.
5. Nightly integrity checks.
6. Reminder jobs (invoice/compliance deadlines).
7. Materialized report refresh.

---

## 14) Error Handling Rules (Product-Level)

### Required scenarios
1. Partial parse results.
2. Ambiguous swimmer matching.
3. Duplicate webhook events.
4. Provider outage (email/SMS/Stripe).
5. Mixed ZIP validity.
6. Concurrent roster edits during import.
7. Fee conflicts from imports.
8. Late payment after invoice adjustment.

### Rules template
1. User-visible status:
2. Auto-retry behavior:
3. Manual intervention path:
4. Audit requirement:
5. Notification target:

---

## 15) Reporting Definitions

For each report define:
1. Columns
2. Filters
3. Grouping
4. Date logic
5. Access permissions
6. Export type
7. Performance target
8. Source-of-truth tables

### Core v1 report list
1. Membership roster report.
2. Active registration report.
3. Meet fee report.
4. Invoice aging report.
5. Payment and refund report.
6. Import job success/failure report.

---

## 16) Billing Business Rules (Exact)

1. Invoice numbering format and uniqueness.
2. Due date policy.
3. Partial payment handling.
4. Credit vs refund vs write-off definitions.
5. Household vs swimmer charge ownership.
6. Meet fee calculation rules and rounding.
7. Void rules.
8. Refund approval requirements.
9. Failed payment retry cadence.
10. Coupon/discount stacking rules.
11. Immutable ledger event definitions.

---

## 17) Communications Rules

1. Sender eligibility by role.
2. Recipient selection logic and exclusions.
3. Template approval and variable controls.
4. Suppression and unsubscribe behavior.
5. Attachment policy.
6. Bounce/failure handling.
7. Resend policy.
8. Message audit visibility and retention.

---

## 18) Compliance Workflows

1. Required docs per role/group.
2. Status tracking and expiration.
3. Reminder schedule.
4. Upload/view/delete permissions.
5. Verification workflow.
6. Retention and purge policy.
7. Privacy request workflow (access/delete/correct).

---

## 19) Deployment and Environment Plan

### 19.1 Environments
1. Local
2. Dev
3. Staging
4. Production

### 19.2 Operational requirements
1. Branch strategy and release process.
2. CI/CD pipeline and required checks.
3. Secret management policy.
4. Domain/subdomain architecture.
5. Backup automation and restore drills.
6. Rollback mechanics.
7. Staging data policy.

---

## 20) Local Development Bootstrap

1. Tooling versions (Go, Node, Docker, Postgres, Redis).
2. `.env.example` complete.
3. One-command startup (make/dev script).
4. Seed data command.
5. Test user accounts.
6. Migration commands.
7. Worker startup commands.
8. Stripe local webhook instructions.
9. Email sandbox setup.

---

## 21) Test Strategy and Quality Gates

### 21.1 Required test types
1. Unit tests
2. Integration tests
3. E2E tests
4. API contract tests
5. Parser fixture tests
6. Authorization matrix tests
7. Billing reconciliation tests
8. Migration dry-run tests
9. Accessibility checks
10. Smoke tests pre-release

### 21.2 Quality gates
1. Critical modules need passing integration + e2e.
2. Import/export changes must pass fixture suite.
3. Billing changes must pass ledger invariants and replay tests.

---

## 22) Seed and Demo Data

Include realistic seed packs for:
1. Households and contacts
2. Swimmers and groups
3. Meets and sessions
4. Commitments and entries
5. Results history
6. Invoices and payments
7. Edge-case records

---

## 23) Decision Log (Unresolved Choices)

| Decision | Options | Owner | Due Date | Final Choice | Reason |
|---|---|---|---|---|---|
| Auth provider | Auth0 / Keycloak |  |  |  |  |
| Single vs multi-tenant | Single / Multi |  |  |  |  |
| Admin+Portal app split | Combined / Separate |  |  |  |  |
| SMS/push in v1 | Yes / No |  |  |  |  |
| Email fallback provider | Yes / No |  |  |  |  |

No implementation starts for affected modules until decisions are resolved.

---

## 24) Go-Live Ownership

| Area | Primary Owner | Backup Owner | Signoff Required |
|---|---|---|---|
| Migration execution |  |  | Y |
| Billing reconciliation |  |  | Y |
| Interop certification |  |  | Y |
| Incident response |  |  | Y |
| Training and support |  |  | Y |
| Rollback authority |  |  | Y |

---

## 25) Content and Policy Assets

Must be ready before launch:
1. Email templates
2. Announcement templates
3. Waiver text
4. Privacy policy
5. Terms of service
6. Help docs
7. Admin SOPs
8. Onboarding copy
9. Support macros

---

## 26) Post-Launch Support Model

1. Who monitors alerts.
2. Who handles failed imports.
3. Who handles payment disputes.
4. Hotfix vs backlog criteria.
5. Communication process for incidents.
6. Day-1, week-1, month-1 support schedule.

---

## 27) Module Signoff Criteria

### Auth complete when
1. OIDC + MFA enforced for privileged users.
2. Session controls and logout verified.
3. Auth-related audit logs confirmed.

### Billing complete when
1. Invoice/payment/refund lifecycle passes tests.
2. Stripe webhook idempotency verified.
3. Reconciliation report meets threshold.

### Import engine complete when
1. Fixture suite passes.
2. Conflict queue works with deterministic resolution.
3. Reprocessing is idempotent.

### Migration complete when
1. Dry-run and production reconciliation pass.
2. Data integrity checks green.
3. Stakeholder signoff obtained.

---

## 28) Time and Budget Constraints

1. Target launch date: ____________________
2. Max budget: ____________________
3. Team size and allocation: ____________________
4. Acceptable manual operations at launch: ____________________
5. Must-build vs buy list: ____________________

---

## 29) Legacy System Transition Plan

1. What remains external in v1.
2. What is replaced immediately.
3. Temporary sync processes and sunset dates.
4. Read-only archive strategy.
5. Final decommission date.

---

## 30) Dependency-Driven Build Order (Execution Sequence)

1. Auth + RBAC foundation.
2. Membership core.
3. Meet and commitment models.
4. Entries and SD3 export.
5. Import pipeline and reconciliation.
6. Best-time engine updates.
7. Billing ledger then billing UI.
8. Comms permissions then bulk send.
9. Audit and reporting hardening.
10. Migration and cutover.

No downstream module starts if upstream dependency is incomplete.

---

## 31) Security Framework (App Controls Only)

1. OIDC auth; no custom password auth.
2. MFA for admin/coach.
3. RBAC + resource-level authorization.

# IMPLEMENTATION_PACKET_MASTER.md
Last Updated: 2026-03-15
Owner: ____________________
Project: DARE Full-Stack Club Platform (TeamUnify/Commit-compatible)

---

## 0) How To Use This File
This is the single copy/paste implementation packet for engineering handoff.
- Fill every `TBD`.
- Convert each section into tracked tickets after approval.
- No coding starts until Section 21 gate is passed.

---

## 1) Product Boundary (Locked Scope)

### 1.1 v1 Must-Have (Launch Blocking)
1. Auth + RBAC (owner/admin/coach/parent/swimmer read-only).
2. Member/household management.
3. Groups + eligibility tags.
4. Meet creation + commitments + entries.
5. Meet file interoperability:
- Import `.cl2`, `.sd3`, `.hy3`, `.zip`.
- Export Standard SD3 + Extended SD3.
- SDIF roster export/import.
- Update-times workflow from imported results.
6. Best-times + standards checks.
7. Billing core (invoice/payment/refund/write-off).
8. Comms core (announcement + email).
9. Audit logs.
10. Essential admin reports.

### 1.2 v1 Allowed Manual Ops
1. Manual conflict resolution queue triage.
2. Manual exception handling for malformed legacy records.
3. Manual support escalation for edge-case billing disputes.

### 1.3 v1 Excluded (v1.1+)
1. Native mobile apps (PWA only in v1).
2. Advanced workout analytics.
3. Multi-club tenancy (if single-club chosen now).
4. Non-critical integrations listed as optional.

### 1.4 Go/No-Go Rules
1. No launch without 2 successful parallel meet cycles.
2. No launch without billing reconciliation threshold met.
3. No launch without backup-restore drill pass.

---

## 2) Canonical Feature Registry (Single Source of Truth)

Use this table as master. Duplicate features elsewhere are non-authoritative.

| Feature ID | Feature Name | Version | Priority | Owner | Dependencies | Acceptance Criteria | Status |
|---|---|---|---|---|---|---|---|
| F-001 | Auth + RBAC | v1 | P0 | TBD | None | All protected endpoints enforce role checks | TBD |
| F-002 | Household + Member CRUD | v1 | P0 | TBD | F-001 | Admin CRUD + parent self-service restrictions work | TBD |
| F-003 | Groups + Eligibility | v1 | P0 | TBD | F-002 | Rule-based assignment and manual override tracked | TBD |
| F-004 | Meet Management | v1 | P0 | TBD | F-002,F-003 | Meets/sessions/events lifecycle works | TBD |
| F-005 | Commitments | v1 | P0 | TBD | F-004 | Parent/coach commitment flows complete | TBD |
| F-006 | Entries + Approval | v1 | P0 | TBD | F-005 | Entry approval state machine enforced | TBD |
| F-007 | SD3 Export (Std/Ext) | v1 | P0 | TBD | F-006 | Export imports into target software without manual edits | TBD |
| F-008 | CL2/SD3/HY3/ZIP Import | v1 | P0 | TBD | F-004 | Parser + validation + conflict queue works | TBD |
| F-009 | SDIF Roster I/O | v1 | P0 | TBD | F-002 | Import/export roundtrip verified on fixture set | TBD |
| F-010 | Update Times from Results | v1 | P0 | TBD | F-008 | Best-times updated with audit trail + override rules | TBD |
| F-011 | Billing Ledger/Invoices | v1 | P0 | TBD | F-001,F-002 | Invoicing/payment/refund/write-off rules enforced | TBD |
| F-012 | Stripe Integration | v1 | P0 | TBD | F-011 | Webhook idempotency + reconciliation passes | TBD |
| F-013 | Announcements + Email | v1 | P1 | TBD | F-001,F-002 | Segmented sends + suppression rules work | TBD |
| F-014 | Audit Log | v1 | P0 | TBD | F-001 | All privileged actions logged immutably | TBD |
| F-015 | Admin Reports Core | v1 | P1 | TBD | F-011,F-008 | Required reports generated with correct filters | TBD |

---

## 3) Roles and Permissions (Endpoint + UI)

### 3.1 Roles
1. `owner`
2. `admin`
3. `coach`
4. `parent`
5. `swimmer_readonly`
6. `billing_only` (optional)
7. `readonly_staff` (optional)

Account policy note:
- Parent role is required for household self-service lifecycle.
- Swimmer role is optional and can be disabled without launch impact.

### 3.2 Permission Matrix (fill exact endpoints)
| Resource/Action | Owner | Admin | Coach | Parent | Swimmer RO | Billing Only | Notes |
|---|---|---|---|---|---|---|---|
| View swimmers (all) | Y | Y | Y (scope) | N | N | N | Scope rules TBD |
| Edit swimmer profile | Y | Y | Y (limited) | Y (limited fields) | N | N | Field-level restrictions |
| Select swimmer meet events (after commit yes) | Y | Y | Y | Y (household only) | N | N | Pre-entry-lock only |
| Override swimmer event selections | Y | Y | Y | N | N | N | Audit reason required |
| View medical flags | Y | Y | Y (need-to-know) | Own child only | N | N | Strict audit |
| Create invoice | Y | Y | N | N | N | Y | |
| Issue refund | Y | Y (step-up MFA) | N | N | N | Y (if allowed) | |
| Import result files | Y | Y | Y (if permitted) | N | N | N | |
| Approve entries | Y | Y | Y | N | N | N | |
| Send bulk comms | Y | Y | Y (group scope) | N | N | N | |

---

## 4) Core User Flows (Step-by-Step)

### 4.1 Parent Registration Flow
1. Parent creates account.
2. Parent verifies email/SMS.
3. Parent adds household + swimmer profiles.
4. Parent accepts waivers.
5. Parent selects program/group.
6. Parent submits payment.
7. System confirms enrollment + receipts + audit log.

### 4.2 Meet Entry Flow
1. Admin creates meet/session/events.
2. Coach publishes commitment request window.
3. Parent commits swimmer.
4. Parent selects events per swimmer when committed `yes`.
5. Coach reviews commitments and selected events.
6. Coach updates/overrides selections where needed.
7. Coach approves/finalizes entries.
8. System validates swimmer-event compatibility and exports SD3.
9. Audit + notification events emitted.

### 4.3 Results Import Flow
1. Admin uploads `.cl2/.sd3/.hy3/.zip`.
2. Worker validates file type + security checks.
3. Parser extracts records.
4. Matcher links swimmer/event records.
5. Ambiguous records go to conflict queue.
6. Admin resolves conflicts.
7. System posts results + updates best-times + optionally update future entry times.
8. Import report stored + downloadable.

### 4.4 Billing Flow
1. System generates invoice batch.
2. Parent pays via Stripe.
3. Webhook confirms payment.
4. Ledger events recorded.
5. Refund/credit/write-off processed if needed.
6. Reconciliation job verifies totals daily.

---

## 5) State Machines (Authoritative)

### 5.1 Invoice
States: `draft -> issued -> partially_paid -> paid -> refunded_partial/refunded_full -> voided -> written_off`
- Allowed transitions + actor + side effects must be enumerated before build.

### 5.2 Payment
States: `pending -> succeeded -> failed -> disputed -> reversed`
- Webhook retries must be idempotent.

### 5.3 Meet
States: `draft -> published -> commitment_open -> commitment_closed -> entry_finalized -> completed -> archived`

### 5.4 Commitment
States: `invited -> committed_yes/committed_no -> coach_reviewed -> locked`

### 5.5 Entry
States: `draft -> submitted -> approved -> exported -> updated_from_results -> archived`

### 5.6 File Import
States: `uploaded -> scanning -> parsing -> matching -> conflicts_pending -> applied -> failed -> archived`

### 5.7 Conflict Resolution
States: `open -> assigned -> resolved_auto/resolved_manual -> rejected -> closed`

### 5.8 Compliance Document
States: `not_submitted -> submitted -> verified -> expiring_soon -> expired -> waived`

---

## 6) Data Schema (Build-Ready Specification)

Include final SQL in `/db/migrations`. This section is mandatory before coding.

### 6.1 Required conventions
1. UUID PKs.
2. `created_at`, `updated_at`, `deleted_at` soft delete where needed.
3. `created_by`, `updated_by` for privileged entities.
4. Tenant field decision:
- If single-club now: include `org_id` anyway (future-proof).
5. All PII columns tagged for encryption policy.

### 6.2 Minimum tables (v1)
1. `users`, `roles`, `user_roles`, `sessions`
2. `households`, `contacts`, `swimmers`, `swimmer_groups`, `groups`
3. `meets`, `meet_sessions`, `meet_events`
4. `commitments`, `entries`, `entry_legs`
5. `results`, `splits`, `best_times`, `time_standards`
6. `invoices`, `invoice_items`, `payments`, `refunds`, `ledger_events`
7. `file_imports`, `file_import_items`, `file_import_errors`, `file_exports`, `match_conflicts`
8. `announcements`, `email_campaigns`, `email_deliveries`
9. `audit_logs`, `webhook_events`

### 6.3 Index and constraints requirements
1. Unique swimmer identifiers per org.
2. Unique invoice number per org.
3. Idempotency key unique for webhook events.
4. Composite indexes for reporting filters (date + group + status).
5. FK constraints with explicit cascade policy documented.

---

## 7) Data Ownership and Conflict Rules

Define per field:
- source of truth
- editable by
- import behavior
- override policy
- conflict winner

Examples:
1. Best time:
- Imported result is default winner unless manual lock exists.
2. Name mismatch:
- Household profile canonical; import alias stored for mapping.
3. DOB edits after results:
- Allowed only admin; rematch impacted records queued.

---

## 8) Interoperability Specs and Fixture Pack

### 8.1 Required docs
1. `docs/interop/sd3.md`
2. `docs/interop/cl2.md`
3. `docs/interop/sdif.md`
4. `docs/interop/hy3.md`
Each must contain:
- Record map
- Field normalization rules
- Required/optional fields
- Known edge cases
- Error codes

### 8.2 Fixture library
1. Valid files (real anonymized).
2. Malformed files.
3. Large files.
4. Ambiguous identity samples.
5. ZIP mixed validity samples.

### 8.3 Expected output fixtures
1. Parsed JSON snapshots.
2. Expected export files.
3. Roundtrip assertions.

---

## 9) Migration Packet

### 9.1 Inputs needed
1. Roster exports.
2. Household/contact exports.
3. Billing/invoice/payment exports.
4. Meet/event/entry/results history.
5. Known bad data samples.

### 9.2 Mapping matrix columns
- Source system
- Source field
- Target table.field
- Transform
- Validation
- Default
- Owner

### 9.3 Migration phases
1. Dry-run in staging.
2. Reconciliation report.
3. Remediation.
4. Final cutover migration.
5. Post-cut verification.

### 9.4 Rollback
- Snapshot taken + tested restore + decision authority named.

---

## 10) UX/Wireframes and Navigation

### 10.1 Required screens list
1. Login/MFA
2. Parent dashboard
3. Coach dashboard
4. Admin dashboard
5. Household/member management
6. Group management
7. Meet setup
8. Commitments
9. Entries
10. Import center + conflict queue
11. Billing center
12. Comms center
13. Reports
14. Settings + audit viewer
15. Parent portal home (alerts, tasks, and household shortcuts)
16. Parent meet commitments center
17. Parent billing/invoices/receipts center
18. Parent document/compliance center
19. Optional swimmer portal home
20. Optional swimmer performance center

### 10.2 Required states for each screen
- loading, empty, error, success, partial failure, permission denied

### 10.3 Design system decisions
- typography, spacing, form behavior, destructive actions, notification patterns

---

## 11) API Contract (OpenAPI-First)

### 11.1 Required standards
1. OpenAPI file checked into repo.
2. Error envelope standardized.
3. Pagination/filter/sort conventions standardized.
4. Auth scope documented per endpoint.
5. Idempotency header required for critical writes.

### 11.2 Core modules
1. `/auth`
2. `/members`
3. `/groups`
4. `/meets`
5. `/commitments`
6. `/entries`
7. `/results/import`
8. `/exports/sd3`
9. `/billing`
10. `/comms`
11. `/admin/audit`

---

## 12) Background Jobs (Spec Per Job)

Template (repeat for each job):
- Job name
- Trigger
- Input schema
- Idempotency key
- Timeout
- Retry policy
- DLQ behavior
- Alerting threshold
- User-visible status updates
- Audit log event emitted

Must-spec jobs:
1. File import parse/match/apply.
2. SD3/SDIF exports.
3. Email/SMS send + bounce handling.
4. Billing reconciliation daily.
5. Reminder jobs (compliance/invoice/commitment).
6. Nightly integrity checks.

---

## 13) Error Handling Rules (Product-Level)

Define what user sees and system does for:
1. Partial parse success.
2. Ambiguous swimmer match.
3. Duplicate Stripe webhook.
4. Provider outage (email/SMS).
5. Mixed-valid ZIP/HY3 import.
6. Roster changed during import.
7. Fee conflicts post-import.
8. Payment after invoice adjustment.

For each:
- user message
- retriable?
- auto-retry?
- manual action required?
- audit/alert required?

---

## 14) Reporting Definitions (Exact)

For each report specify:
1. Purpose.
2. Columns.
3. Filters.
4. Grouping/aggregation.
5. Date logic.
6. Access permissions.
7. Export formats.
8. Expected runtime.

Minimum v1 reports:
1. Membership snapshot.
2. Attendance summary.
3. Meet fee report.
4. Invoice aging.
5. Payment reconciliation.
6. Import error summary.
7. Swimmer aging-out roster.

---

## 15) Billing Rules (Authoritative)

Must be fully defined:
1. Invoice numbering format.
2. Due dates and grace periods.
3. Household vs swimmer charge ownership.
4. Partial payment handling.
5. Credit vs refund vs write-off semantics.
6. Voids and reversals.
7. Meet fee calculation rules.
8. Coupon/discount precedence.
9. Failed payment retries.
10. Ledger event taxonomy:
- `invoice_created`, `payment_received`, `refund_issued`, etc.

---

## 16) Communications Rules

Define:
1. Who can send which message types.
2. Recipient selection logic.
3. Template ownership and approvals.
4. Suppression and unsubscribe rules.
5. Attachment policy.
6. Bounce/failure retry behavior.
7. Read/open tracking policies.
8. Audit visibility rules.

---

## 17) Compliance Document Workflows

Track:
1. Document types (waivers, medical, volunteer checks).
2. Required fields.
3. Verification steps.
4. Expiration rules.
5. Reminder schedule.
6. Role-based visibility.
7. Retention/deletion policy.

---

## 18) Environment, Deployment, and Ops

### 18.1 Environments
1. `dev`
2. `staging` (prod-like)
3. `prod`

### 18.2 Deployment rules
1. Branch strategy.
2. CI gates.
3. Preview deploys.
4. Secrets manager.
5. Rollback strategy.
6. Backup schedule + restore test cadence.

### 18.3 Domain plan
- `www` public, `admin` portal, `api` backend, status page domain.

---

## 19) Local Development Bootstrap

Must include:
1. Tool versions.
2. Docker compose for Postgres/Redis.
3. `.env.example`.
4. Seed scripts + test users.
5. Migration commands.
6. Worker startup commands.
7. Stripe local webhook instructions.
8. Email sandbox setup.

---

## 20) Test Strategy (Execution-Grade)

### 20.1 Required test types
1. Unit tests (parser, billing logic, permissions).
2. Integration tests (API + DB + queue).
3. Contract tests (OpenAPI compatibility).
4. Fixture tests (SD3/CL2/SDIF/HY3).
5. Migration tests (dry-run assertions).
6. E2E tests (critical user journeys).
7. Accessibility tests.

### 20.2 Release gates
1. All P0 tests pass.
2. No critical security findings open.
3. Reconciliation checks pass.
4. Smoke test pass in staging.

---

## 21) Decision Log (Resolve Before Build)

Track unresolved choices with deadline and owner:
1. Auth0 vs Keycloak.
2. Single-club vs multi-tenant now.
3. Admin and parent portal combined or separate.
4. Email single provider vs fallback.
5. SMS/push launch scope.
6. PWA only vs native mobile timeline.
7. Public site migration scope.

Template:
| Decision | Options | Chosen | Owner | Due Date | Rationale |
|---|---|---|---|---|---|

---

## 22) Cutover Ownership and Runbook

Name owners:
1. Data migration lead.
2. Billing reconciliation lead.
3. Meet-file certification lead.
4. Support lead.
5. Training lead.
6. Go-live approver.
7. Rollback authority.

Cutover checklist:
1. Data freeze window communicated.
2. Final migration executed.
3. Parallel validation done.
4. Billing totals checked.
5. Import/export certification run.
6. Go-live signoff recorded.

---

## 23) Content and Policy Assets

Required before launch:
1. Email templates.
2. Announcement templates.
3. Waiver and consent text.
4. Privacy policy and terms.
5. Help docs and onboarding.
6. Admin SOPs.
7. Support macros.

---

## 24) Post-Launch Support Model

Define:
1. On-call schedule.
2. Incident severities.
3. First response targets.
4. Hotfix criteria.
5. Data correction procedure.
6. Weekly operations review cadence.

---

## 25) Module-by-Module Signoff Criteria

### Auth
- MFA, token rotation, role enforcement tested.

### Billing
- Ledger/invoice/payment/refund workflows reconcile exactly.

### Interop
- SD3/CL2/SDIF/HY3 fixtures pass; export accepted by target software.

### Migration
- Dry-run + prod run reconciliation signed.

### Reporting
- All required reports match source-of-truth queries.

### Security
- Audit logs, alerting, backup restore, vuln scans complete.

---

## 26) Time, Budget, and Capacity Constraints

Fill these now:
1. Target launch date: `TBD`
2. Max budget: `TBD`
3. Team size/FTE: `TBD`
4. Acceptable manual ops at launch: `TBD`
5. Must-build vs buy list: `TBD`

---

## 27) Legacy System Coexistence Plan

Define:
1. What remains external during v1.
2. Temporary syncs needed.
3. Permanent external dependencies.
4. Decommission date and read-only archive plan.

---

## 28) Dependency-Ordered Build Plan

1. Auth + RBAC + audit foundation.
2. Members/households/groups.
3. Meets/commitments/entries.
4. Interop export engine.
5. Import + matching + conflict queue.
6. Best-times and standards.
7. Billing ledger and Stripe.
8. Comms and reporting.
9. Migration tooling and cutover rehearsals.
10. Parallel run and launch.

---

## 29) CA/LA Operational Considerations (Technical)
1. Timezone default: `America/Los_Angeles`.
2. Date handling in UI/API standardized to ISO + explicit TZ.
3. Incident notification templates prebuilt for CA workflows.
4. Staff compliance status fields and reminder automation included.
5. Regional outage fallback procedures documented.

---

## 30) Readiness Gate (No-Code-Until Checklist)

Mark each `DONE` before coding:
1. Scope locked.
2. Feature registry locked.
3. RBAC matrix approved.
4. Workflows approved.
5. State machines approved.
6. Schema v1 frozen.
7. OpenAPI v1 frozen.
8. Interop fixtures ready.
9. Migration samples ready.
10. Billing rules approved.
11. Job specs approved.
12. Wireframes baseline approved.
13. Deployment runbook drafted.
14. Cutover owners assigned.

---

## Appendix A) Suggested Repo Structure

- `/docs/implementation/IMPLEMENTATION_PACKET_MASTER.md` (this file)
- `/docs/interop/*`
- `/docs/billing/*`
- `/docs/workflows/*`
- `/docs/security/*`
- `/db/migrations/*`
- `/api/openapi.yaml`
- `/fixtures/interop/*`
- `/fixtures/migration/*`

---

## Appendix B) Ticket Template (Use For Every Story)

Title:
Description:
Business rule:
Dependencies:
API/schema impact:
Acceptance criteria:
Test cases:
Rollback considerations:
Owner:
Estimate:
Status:

---

## Appendix C) Definition of Done (Global)

A ticket is done only when:
1. Acceptance criteria pass.
2. Tests added and passing.
3. Audit/security considerations addressed.
4. Documentation updated.
5. Monitoring/alerting added (if applicable).
6. Feature flag/rollback path defined (if high risk).
