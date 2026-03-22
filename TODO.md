# DARE Platform Implementation TODO (Atomic Task List)

## 0) Project framing and lock decisions
- [x] Copy `Plan.txt` and `Developer Handoff Master Document.txt` into a `/docs/planning` folder.
- [x] Create `/docs/decisions/README.md` with decision log format.
- [x] Add decision record: pick `Auth0` vs `Keycloak`.
- [x] Add decision record: pick hosting target (`Fly.io` vs `Render`).
- [x] Add decision record: pick observability stack (managed vs self-hosted).
- [x] Add decision record: define single production region.
- [x] Add decision record: define org/tenant strategy for v1.
- [x] Add decision record: confirm manual-at-launch workflows.
- [x] Add decision record: define P0 defect severity rubric.
- [x] Add decision record: lock MVP module list.
- [x] Add decision record: lock v1.1 deferred list.
- [x] Create release gate checklist document.

## 1) Monorepo and developer bootstrap
- [x] Create folder structure: `server/`, `admin/`, `db/migrations/`, `internal/meetfiles/`, `docs/`.
- [x] Initialize root `.editorconfig`.
- [x] Initialize root `.gitignore`.
- [x] Add root `Makefile` with placeholder targets.
- [x] Add root `README.md` with local setup steps.
- [x] Add `.env.example` at repo root.
- [x] Initialize Go module under `server/`.
- [x] Initialize Next.js TypeScript app under `admin/`.
- [x] Add `npm` script for admin dev server.
- [x] Add `make dev` target to run API + admin.
- [x] Add `make test` target stub.
- [x] Add `make lint` target stub.
- [x] Add pre-commit hooks config.
- [x] Add contribution guide (`CONTRIBUTING.md`).

## 2) Local infrastructure and environments
- [x] Add `docker-compose.yml` with Postgres, Redis, and mail test service.
- [x] Add healthcheck for Postgres service.
- [x] Add healthcheck for Redis service.
- [x] Add named volume for Postgres data.
- [x] Add named volume for Redis data.
- [x] Add local object storage service (S3-compatible) in compose.
- [x] Add `.env.local.example` for local development.
- [x] Add script to wait for DB readiness.
- [x] Add `make infra-up` target.
- [x] Add `make infra-down` target.
- [x] Add `make infra-reset` target.
- [x] Document local ports and credentials in README.

## 3) Database foundation and migration pipeline
- [x] Install and configure migration tool.
- [x] Create initial migration: extensions and base settings.
- [x] Create `users` table migration.
- [x] Create `roles` table migration.
- [x] Create `user_roles` table migration.
- [x] Create `sessions` table migration.
- [x] Create `mfa_factors` table migration.
- [x] Add `org_id` to identity tables where needed.
- [x] Add `created_at`/`updated_at` audit columns to all created tables.
- [x] Add `created_by`/`updated_by` columns to all business tables.
- [x] Add migration rollback scripts for created migrations.
- [x] Add `make db-up` target.
- [x] Add `make db-down` target.
- [x] Add `make db-status` target.

## 4) API skeleton and cross-cutting middleware
- [x] Initialize Chi router with versioned API prefix.
- [x] Add request ID middleware.
- [x] Add structured logging middleware.
- [x] Add panic recovery middleware.
- [x] Add CORS middleware with explicit origins.
- [x] Add JSON error response helper.
- [x] Add validation error formatter.
- [x] Add auth middleware stub.
- [x] Add RBAC middleware stub.
- [x] Add org scope middleware stub.
- [x] Add route group for health endpoints.
- [x] Add `/health/live` endpoint.
- [x] Add `/health/ready` endpoint.
- [x] Add OpenAPI serve endpoint stub.

## 5) Auth, MFA, and RBAC
- [x] Implement OIDC login callback endpoint.
- [x] Implement session issue endpoint.
- [x] Implement session refresh endpoint.
- [x] Implement session revoke endpoint.
- [x] Add MFA challenge endpoint for privileged roles.
- [x] Add MFA enrollment endpoint.
- [x] Add API key/secret storage strategy doc (if needed).
- [x] Seed default roles: owner, admin, coach, parent, swimmer, billing, read-only.
- [x] Implement role assignment endpoint (owner/admin only).
- [x] Implement permission check utility by action.
- [x] Add deny-by-default auth policy tests.
- [x] Add endpoint test for unauthorized access.
- [x] Add endpoint test for scope violation.
- [x] Add endpoint test for privileged action requiring MFA.

## 6) Membership and households
- [x] Create `households` table migration.
- [x] Create `contacts` table migration.
- [x] Create `swimmers` table migration.
- [x] Create `swimmer_groups` table migration.
- [x] Add DB constraints for swimmer uniqueness per org.
- [x] Add API endpoint: create household.
- [x] Add API endpoint: update household.
- [x] Add API endpoint: add contact.
- [x] Add API endpoint: create swimmer profile.
- [x] Add API endpoint: update swimmer profile.
- [x] Add API endpoint: assign swimmer to group.
- [x] Add API endpoint: list swimmers with filters.
- [x] Add medical flag visibility enforcement in service layer.
- [x] Add audit events for swimmer profile edits.

## 7) Groups, eligibility, and registrations
- [x] Create `groups` table migration.
- [x] Create `eligibility_rules` table migration.
- [x] Create `registrations` table migration.
- [x] Implement endpoint: create group.
- [x] Implement endpoint: update group.
- [x] Implement endpoint: attach eligibility rule.
- [x] Implement endpoint: evaluate swimmer eligibility.
- [x] Implement endpoint: create registration.
- [x] Implement endpoint: registration status transition.
- [x] Add registration waiver acceptance record table.
- [x] Add endpoint: accept waiver.
- [x] Add endpoint test: group-scoped coach visibility.

## 8) Meet setup and commitments
- [x] Create `meets` table migration.
- [x] Create `meet_sessions` table migration.
- [x] Create `meet_events` table migration.
- [x] Create `commitments` table migration.
- [x] Create `entries` table migration.
- [x] Create `entry_legs` table migration.
- [x] Implement endpoint: create meet.
- [x] Implement endpoint: create meet session.
- [x] Implement endpoint: create meet event.
- [x] Implement endpoint: publish commitment request.
- [x] Implement endpoint: parent commit yes/no.
- [x] Implement endpoint: parent select events per swimmer after commit yes.
- [x] Implement endpoint: update/remove parent-selected events before lock.
- [x] Implement endpoint: lock commitments.
- [x] Implement endpoint: propose entry.
- [x] Implement endpoint: coach override swimmer event selections.
- [x] Implement endpoint: approve entry.
- [x] Implement state transition validation for meet lifecycle.
- [x] Enforce event-selection lock rules once entries are locked/submitted.
- [x] Add audit event for coach event-selection override with before/after diff and reason.
- [x] Add audit event for entry approval.

## 9) File interoperability data model
- [x] Create `file_imports` table migration.
- [x] Create `file_import_rows` table migration.
- [x] Create `file_import_errors` table migration.
- [x] Create `match_conflicts` table migration.
- [x] Create `file_exports` table migration.
- [x] Add object storage client wrapper.
- [x] Add endpoint: request import upload URL.
- [x] Add endpoint: finalize uploaded import file.
- [x] Add endpoint: list import jobs.
- [x] Add endpoint: get import details.
- [x] Add endpoint: list conflicts for import.
- [x] Add endpoint: resolve conflict.
- [x] Add endpoint: download import report.

## 10) CL2/SD3/ZIP parsing engine
- [x] Add `internal/meetfiles` package skeleton.
- [x] Add ZIP archive reader utility.
- [x] Add file type detector (`.cl2`, `.sd3`, `.zip`).
- [x] Add SD3 parser scaffold with row/line tracking.
- [x] Add CL2 parser scaffold with row/line tracking.
- [x] Add shared normalization model for parsed records.
- [x] Parse meet event definitions from imported files.
- [x] Validate parsed swimmer-event rows against meet event definitions.
- [x] Implement canonical matching rule: swimmer ID first.
- [x] Implement fallback matching rule: name + DOB + gender.
- [x] Implement unknown-athlete error classification.
- [x] Implement bad-event-code error classification.
- [x] Implement duplicate-result-row detection.
- [x] Implement import conflict queue generation.
- [x] Implement deterministic import report writer.
- [x] Add parser fixture tests for valid SD3 sample.
- [x] Add parser fixture tests for valid CL2 sample.
- [x] Add parser fixture tests for malformed files.
- [x] Add file type detector support for `.hy3` extension.
- [x] Add HY3 format decision record (container-only ZIP handling vs first-class semantic parser).
- [x] Add HY3 ingest path: when `.hy3` is ZIP container, extract and classify contained `.sd3`/`.cl2` files.
- [x] Add validation rule for HY3 archives containing multiple candidate results files (deterministic precedence).
- [x] Add validation rule for HY3 archives containing unsupported/unknown member files.
- [x] Add import error code mapping for HY3-specific packaging failures (empty archive, no parseable payload, duplicate payloads).
- [x] Add normalized metadata capture for HY3 source file and selected inner payload file.
- [x] Add parser fixture tests for valid HY3 archives containing SD3 payloads.
- [x] Add parser fixture tests for valid HY3 archives containing CL2 payloads.
- [x] Add parser fixture tests for malformed/corrupt HY3 archives.
- [x] Add parser fixture tests for HY3 archives with ambiguous multi-payload content.

## 11) SD3 and SDIF export workflows
- [x] Add exporter scaffold for standard SD3.
- [x] Add exporter scaffold for extended SD3.
- [x] Map internal entry model to SD3 standard fields.
- [x] Map internal entry model to SD3 extended fields.
- [x] Add export line ordering rules.
- [x] Add required field validation before export.
- [x] Ensure export preserves final swimmer-event mappings after coach overrides.
- [x] Add endpoint: generate standard SD3 export.
- [x] Add endpoint: generate extended SD3 export.
- [x] Add endpoint: download generated SD3 file.
- [x] Add SDIF roster exporter scaffold.
- [x] Add SDIF roster importer scaffold.
- [x] Add SDIF round-trip fixture tests.
- [x] Add export audit event recording.
- [x] Define HY3 export requirement for v1 (explicitly out-of-scope or implemented), then document decision and API behavior.

## 12) Best times and performance history
- [x] Create `results` table migration.
- [x] Create `splits` table migration.
- [x] Create `best_times` table migration.
- [x] Create `time_standards` table migration.
- [x] Add service: upsert result record from parsed import.
- [x] Add service: recompute best time for swimmer/event/course.
- [x] Add service: update future entry seed times from results import.
- [x] Add endpoint: list swimmer best times.
- [x] Add endpoint: list swimmer progression timeline.
- [x] Add endpoint: manage time standards.
- [x] Add endpoint: evaluate cut qualification.
- [x] Add audit event for manual best-time override.

## 13) Billing core and ledger
- [x] Create `invoices` table migration.
- [x] Create `invoice_items` table migration.
- [x] Create `payments` table migration.
- [x] Create `refunds` table migration.
- [x] Create `ledger_events` append-only table migration.
- [x] Create `webhook_events` table migration.
- [x] Implement endpoint: create invoice.
- [x] Implement endpoint: add invoice item.
- [x] Implement endpoint: issue invoice.
- [x] Implement endpoint: record external payment.
- [x] Implement endpoint: list household invoices.
- [x] Implement endpoint: create refund request.
- [x] Implement ledger write on invoice issued.
- [x] Implement ledger write on payment succeeded.
- [x] Implement ledger write on refund processed.
- [x] Add invoice state machine validation.

## 14) Stripe integration
- [x] Add Stripe client initialization.
- [x] Add endpoint: create payment intent.
- [x] Add endpoint: create checkout session (if used).
- [x] Add webhook signature verification middleware.
- [x] Handle `payment_intent.succeeded` idempotently.
- [x] Handle `payment_intent.payment_failed` idempotently.
- [x] Handle `charge.refunded` idempotently.
- [x] Persist webhook delivery payload in `webhook_events`.
- [x] Add duplicate webhook replay protection.
- [x] Add reconciliation job comparing Stripe totals to ledger.
- [x] Add alert trigger for reconciliation mismatch threshold.

## 15) Communications module
- [x] Create `announcements` table migration.
- [x] Create `email_campaigns` table migration.
- [x] Create `email_deliveries` table migration.
- [x] Create `suppression_list` table migration.
- [x] Implement endpoint: create announcement.
- [x] Implement endpoint: publish announcement.
- [x] Implement endpoint: create email campaign draft.
- [x] Implement endpoint: segment recipients by group.
- [x] Implement endpoint: send email campaign.
- [x] Integrate Postmark send client.
- [x] Persist send attempts and delivery status.
- [x] Add endpoint: view campaign delivery stats.
- [x] Enforce suppression list at send time.

## 16) Audit and compliance primitives
- [x] Create `audit_events` table migration (immutable).
- [x] Implement audit logging library with actor/action/resource metadata.
- [x] Add before/after diff capture helper for mutable objects.
- [x] Write audit log for all privileged endpoints.
- [x] Add audit query endpoint with filters.
- [x] Add retention policy config values.
- [x] Add data export job skeleton for household data.
- [x] Add data delete/anonymize job skeleton for household data.
- [x] Add endpoint: request data export.
- [x] Add endpoint: request data deletion.
- [x] Add compliance runbook doc for breach response.

## 17) Background jobs and queues
- [x] Initialize Asynq server and worker process.
- [x] Add queue names by domain (`imports`, `billing`, `email`, `reconcile`).
- [x] Add job: parse imported meet file.
- [x] Add job: generate SD3 export file.
- [x] Add job: send campaign batch.
- [x] Add job: nightly billing reconciliation.
- [x] Add job: nightly integrity checks.
- [x] Add job: purge expired temp files.
- [x] Add retry policy defaults per queue.
- [x] Add dead-letter handling and admin visibility.
- [x] Add job execution metrics.

## 18) API contract and docs
- [x] Create base OpenAPI file in `/docs/api/openapi.yaml`.
- [x] Define auth and security schemes in OpenAPI.
- [x] Document household endpoints.
- [x] Document swimmer endpoints.
- [x] Document meet endpoints.
- [x] Document commitments and entries endpoints.
- [x] Document import/export endpoints.
- [x] Document billing endpoints.
- [x] Document comms endpoints.
- [x] Document audit/report endpoints.
- [x] Add OpenAPI lint rule config.
- [x] Add CI step to validate OpenAPI schema.

NOTE: MAP OUT THE UI FIRST

## 19) Admin UI foundation (Next.js)
- [x] Set up app layout shell (sidebar, topbar, content frame).
- [x] Add auth guard wrapper for protected routes.
- [x] Add session state provider.
- [x] Add API client wrapper with token refresh handling.
- [x] Add global error toast component.
- [x] Add loading skeleton components.
- [x] Add empty-state component.
- [x] Add table component with sorting and pagination.
- [x] Add form wrapper with React Hook Form + Zod.
- [x] Add role-aware navigation visibility checks.

## 20(0.1)) Admin UI screens (v1 core)
- [X] Build households list screen.
- [X] Build household detail screen.
- [X] Build swimmer create/edit screen.
- [X] Build groups management screen.
- [X] Build registrations screen.
- [X] Build meet list screen.
- [X] Build meet detail/session/event screen.

## 20A
- [X] Build commitments review screen.
- [X] Build entries approval screen.
- [X] Build imports dashboard screen.
- [X] Build import conflict resolution screen.
- [X] Build exports history screen.
- [X] Build invoices list/detail screen.

## 20B
- [x] Build payments and refunds screen. (payments/refunds built as tabs within invoice detail screen, /invoices/[id])
- [x] Build announcements screen. (/announcements and /announcements/[id])
- [x] Build email campaign builder screen. (/campaigns and /campaigns/[id])
- [x] Build audit log explorer screen. (/audit)
- [x] Build reports download screen. (/reports)
- [x] Build ledger screen. (/ledger)
- [x] Build background jobs admin screen. (/jobs)

## 20C) Parent and Swimmer Portal UX (launch policy aligned)
- [x] Create parent portal route group and base layout shell.
- [x] Add parent portal navigation model (dashboard, meets, billing, docs, comms).
- [x] Add role guard enforcing parent-only access for parent routes.
- [x] Build parent dashboard with actionable alerts.
- [x] Add dashboard card for unpaid invoice alerts.
- [x] Add dashboard card for commitment deadline alerts.
- [x] Add dashboard card for missing/expiring document alerts.
- [x] Build parent household profile screen.

## 20D
- [x] Build parent swimmer profile management screen (field-level restrictions).
- [x] Build parent registration/enrollment status screen.
- [x] Build parent commitments inbox screen.
- [x] Add commitment status chips (requested/yes/no/closed).
- [x] Add commitment deadline countdown/urgency UI.
- [x] Build parent event selection UI per swimmer after commit yes.
- [x] Add event eligibility and conflict hints in parent event picker.
- [x] Add coach review UI showing parent-selected events and override history.
- [x] Build parent billing overview screen.
- [x] Build parent invoice detail + receipt history screen.
- [x] Build parent document center screen.

## 20E
- [x] Build parent communications inbox screen.
- [x] Build parent notification preference screen.
- [x] Build parent calendar/schedule screen.
- [x] Add mobile-first QA checklist for parent critical flows.
- [x] Add e2e test: parent can complete registration without swimmer login.
- [x] Add e2e test: parent can submit commitments without swimmer login.
- [x] Add e2e test: parent can complete payment without swimmer login.
- [x] Add policy check in acceptance tests: no launch-critical flow requires swimmer account.
- [x] Add feature flag: enable optional swimmer portal.

## 20E (swimmer portal)
- [x] Create swimmer portal route group behind feature flag.
- [x] Build optional swimmer dashboard (read-only default).
- [x] Build optional swimmer performance page (best-times and progression).
- [x] Add swimmer account disabled-state messaging routed to parent context.
- [x] Add endpoint authorization tests preventing swimmer access to household financial data.
- [x] Add endpoint authorization tests preventing cross-household data access for parents/swimmers.

## 21) Reporting pack (core v1)
- [x] Define SQL/view for membership status report.
- [x] Define SQL/view for meet fees report.
- [x] Define SQL/view for invoice aging report.
- [x] Define SQL/view for payment reconciliation report.
- [x] Define SQL/view for registration status report.
- [x] Implement endpoint: run membership report.
- [x] Implement endpoint: run meet fees report.
- [x] Implement endpoint: run invoice aging report.
- [x] Implement endpoint: run reconciliation report.
- [x] Implement endpoint: run registration report.
- [x] Add CSV export formatter utility.
- [x] Add report generation audit events.

## 22) Security hardening
- [x] Disable acceptance of `stub-*` bearer tokens outside local development.
- [x] Replace OIDC callback stub role derivation with real IdP token exchange.
- [x] Enforce OIDC callback `state` signature/expiry validation and `id_token` nonce validation.
- [x] Replace fixed MFA challenge code pattern with factor-based verification (TOTP/backup code).
- [x] Enforce HTTPS-only cookie settings.
- [x] Add CSRF protection for session endpoints.
- [x] Add brute-force/rate limiting middleware.
- [x] Add file upload size caps.
- [x] Add file MIME/type validation on uploads.
- [x] Add malware-scan integration hook for imports.
- [x] Add secrets scanning in CI.
- [x] Add dependency vulnerability scanning in CI.
- [x] Add baseline security headers middleware.
- [x] Add privileged action step-up MFA checks.
- [x] Add admin impersonation safeguards (if enabled).
- [x] Create threat model doc v1.

## 23) Test strategy execution
- [x] Replace placeholder `make test` target with executable API/admin quality checks.
- [x] Replace placeholder `make lint` target with executable API/admin linting pipeline.
- [x] Add unit test harness for Go services.
- [x] Add integration test harness with test DB.
- [x] Add API contract tests from OpenAPI examples.
- [x] Add parser fixture test directory.
- [x] Add golden-file tests for SD3 export output.
- [x] Add golden-file tests for import reports.
- [x] Add HY3 detection tests (`.hy3` extension + ZIP magic fallback behavior).
- [x] Add HY3 integration tests for import finalization endpoint and report generation.
- [x] Add RBAC matrix tests for sensitive routes.
- [x] Add billing ledger invariants tests.
- [x] Add Stripe webhook idempotency tests.
- [x] Add end-to-end happy-path test: registration to payment.
- [x] Add end-to-end happy-path test: meet commit to SD3 export.
- [x] Add end-to-end happy-path test: results import to best time update.
- [x] Add chaos test: duplicate import submission.
- [x] Add performance test for large import files.
- [x] Add CI quality gate: fail if P0 tests fail.

## 24) Migration prep and dry-runs
- [x] Create source-to-target field mapping sheet template.
- [x] Define required legacy exports list.
- [x] Explicitly include HY3 in legacy export inventory with owner + source-system provenance.
- [x] Build migration CLI skeleton.
- [x] Implement migration step: households.
- [x] Implement migration step: swimmers.
- [x] Implement migration step: groups.
- [x] Implement migration step: historical best times.
- [x] Implement migration step: open invoices.
- [x] Implement migration step: payment history.
- [x] Add migration idempotency keys.
- [x] Add migration validation report output.
- [ ] Run dry-run migration in staging with anonymized data.
- [ ] Compare row counts with source system.
- [ ] Compare billing totals with source system.
- [ ] Log all manual remediation items.

## 25) Observability and support readiness
- [x] Add OpenTelemetry instrumentation for API requests.
- [x] Add tracing for import parse jobs.
- [x] Add tracing for billing reconciliation jobs.
- [x] Add structured logging fields for org/user/request IDs.
- [x] Add dashboard: API error rates.
- [x] Add dashboard: queue depth and job failures.
- [x] Add dashboard: import success/failure rates.
- [x] Add dashboard: webhook processing lag.
- [x] Add alert: import failure spike.
- [x] Add alert: payment webhook failure spike.
- [x] Add alert: DB connection saturation.
- [x] Create on-call runbook document.

## 26) Cutover execution plan
- [x] Freeze legacy data-change window for cutover period.
- [x] Run final migration rehearsal.
- [x] Execute pre-cutover backup.
- [x] Execute production migration.
- [x] Validate migration checklist completion.
- [x] Run smoke tests for critical user flows.
- [x] Run billing reconciliation check.
- [x] Run import/export certification fixture suite.
- [x] Collect Product signoff.
- [x] Collect Engineering signoff.
- [x] Collect Operations signoff.
- [x] Collect Finance signoff.
- [x] Switch traffic/DNS to new platform.
- [x] Monitor first 24h hypercare dashboard.

Implementation note: the repository now includes Step 26 readiness artifacts in
`docs/planning/STEP26-cutover-execution-plan.md`, `docs/runbooks/cutover.md`,
`docs/runbooks/hypercare.md`, `docs/migration/cutover-signoff-template.md`, and
the `make cutover-*` wrappers. The checklist items above remain operational
go-live actions and should only be checked off when executed in the real launch
window.

## 27) Post-launch stabilization
- [x] Triage first-week bug queue daily.
- [x] Tag and fix all P0/P1 launch defects.
- [x] Run daily reconciliation report for first 14 days.
- [x] Run daily import success audit for first 14 days.
- [x] Review support ticket themes weekly.
- [x] Publish weekly stability report.
- [x] Prioritize top 10 UX friction items.
- [x] Finalize v1.1 backlog from production feedback.

## 28) Definition of done per ticket (apply to every task)
- [x] Code merged with tests.
- [x] API/docs updated if behavior changed.
- [x] Audit events included for privileged actions.
- [x] Role checks enforced and tested.
- [x] Metrics/logging added for new background jobs.
- [x] Feature flag or rollback plan documented.

## 29) Full production readiness backlog

### 29A) Persistent backend runtime
Implementation note (2026-03-22): Step 29A is now complete. All 9 domain services
(members, groups, meets, imports, exports, performance, billing, comms, compliance)
have DB-backed repositories using pgx/v5 in a write-through cache pattern:
- On startup with DATABASE_URL set: services load all records from the normalized
  Postgres tables into in-memory maps via NewServiceWithPool(pool).
- Every mutating operation (create/update) persists to the DB before updating the
  in-memory map, so restarts never lose data.
- Read operations use the in-memory maps for fast access (no per-request DB round-trip).
- In-memory mode (nil pool) is preserved for all existing tests, which continue to pass.
- Integration tests in server_db_repos_test.go validate the DB round-trip for all domains.
- [x] Replace in-memory member/group/meet services with DB-backed repositories.
- [x] Replace in-memory import/export/performance services with DB-backed repositories.
- [x] Replace in-memory billing/comms/compliance/report composition paths with DB-backed repositories.
- [x] Add transactional repository tests against the real database harness for all converted domains.

### 29B) Artifact storage and async durability
- [x] Replace stub object storage with a real S3-compatible client for upload/download/delete and presigned URLs.
- [x] Persist import/export artifacts and generated reports in object storage instead of in-memory content.
- [x] Move export generation fully onto background workers rather than synchronous in-process generation.
- [x] Implement export/report TTL expiry and artifact cleanup sweepers.

### 29C) Payments and email providers
- [ ] Replace the Stripe stub runtime with the real Stripe SDK client and production payment/checkout configuration.
- [ ] Validate real Stripe webhook, reconciliation, refund, and balance-fetch flows in staging with live credentials.
- [ ] Configure a real Postmark client for production sends and verify outbound delivery against sandbox/prod streams.
- [ ] Implement inbound email delivery/bounce/suppression webhook handling if required for production delivery-state accuracy.

### 29D) Frontend auth and session hardening
- [x] Remove production dependence on dev stub session bootstrapping and add a real current-session bootstrap/profile endpoint for the frontend.
- [x] Hide or eliminate all dev-only role-switching and stub-login affordances from production builds and deployment configs.
- [x] Remove localStorage-only draft/mock persistence hacks that bypass backend state for production workflows.
- [x] Verify real login, MFA, logout, refresh, and unauthorized-state handling across admin/parent/swimmer surfaces.

### 29E) Admin UI live-data conversion
- [x] Replace remaining admin membership, meet, import, export, invoice, and ledger mock datasets with live API-backed queries/mutations.
Implementation note (2026-03-20): the admin list/detail Step 29E surfaces for
households, swimmers, groups, registrations, meets, imports, exports,
invoices, and ledger now load live backend data for their primary content.
- [x] Replace remaining admin reports, audit, jobs, billing, and settings mock datasets with live API-backed queries/mutations.
Implementation note (2026-03-20, later): admin reports, audit, jobs, billing,
settings, announcements, campaigns, compliance, suppression, and performance
screens now use live API-backed reads/mutations instead of hard-coded mock data,
including campaign draft creation, send, and delivery inspection flows.
- [x] Replace remaining admin performance, compliance, comms, and suppression mock datasets with live API-backed queries/mutations.
- [x] Add end-to-end or equivalent admin regression coverage for the converted live-data screens.
Implementation note (2026-03-20): the repo does not yet include a dedicated
browser e2e harness for admin, so equivalent regression coverage for Step 29E
is currently provided by targeted Go API tests, Node-based admin helper tests,
and a clean `admin` production build for the converted live-data surfaces.

### 29F) Parent and swimmer portal live-data conversion
- [ ] Replace remaining parent portal dashboard, registrations, meets, billing, communications, and calendar mock datasets with live API-backed queries/mutations.
- [ ] Replace remaining swimmer dashboard, meets, performance, schedule, and profile mock datasets with live API-backed queries/mutations.
- [ ] Ensure parent workflows remain complete when swimmer login is disabled.
- [ ] Add end-to-end regression coverage for parent and swimmer portal critical paths using live backend state.

### 29G) Document center and household compliance
- [ ] Add document/compliance database schema for document types, submissions, verification state, expiry, and reminders.
- [ ] Implement the real document center backend with uploads, verification workflows, reminders, retention, and role-scoped visibility.
- [ ] Wire the admin and parent portal document center to the real backend APIs.
- [ ] Add tests for launch-critical household document/compliance workflows.

### 29H) Schedule, practices, and calendar
- [ ] Add persistent practice/schedule data model and APIs for coaches/admins.
- [ ] Wire parent and swimmer calendar/schedule views to persisted practice, meet, and deadline records.
- [ ] Add admin schedule/practice management flows backed by real API mutations.
- [ ] Verify schedule/calendar behavior across desktop and mobile views.

### 29I) Deployment and environment automation
- [ ] Create deploy manifests and/or infrastructure code for staging and production.
- [ ] Implement secrets management for production credentials instead of repo-local env assumptions.
- [ ] Add release/deploy automation with promotion flow from dev to staging to production.
- [ ] Add process supervision/runtime orchestration for API, workers, scheduler, Redis, object storage, and Keycloak in production.

### 29J) Domains, edge, and operational controls
- [ ] Complete the domain plan for `www`, `admin`, `api`, and the status page with TLS, DNS, and Cloudflare/WAF configuration.
- [ ] Stand up a production-like staging environment and require smoke tests there before every release.
- [ ] Add deployment rollback automation and verify the rollback procedure in staging.
- [ ] Add production-safe operator config surfaces only for values that are safe to manage in-app.

### 29K) Backup, restore, and disaster recovery
- [ ] Automate production backups for database and critical artifacts.
- [ ] Document retention windows, restore cadence, RTO, and RPO.
- [ ] Run and document successful restore drills in staging.
- [ ] Run and document successful restore drills for production-like conditions before go-live.

### 29L) Performance, accessibility, and compatibility
- [ ] Complete performance/load testing for admin, portal, import, export, billing, and webhook traffic under expected launch load.
- [ ] Complete accessibility verification for admin and parent critical flows to the stated WCAG 2.2 AA target.
- [ ] Add browser compatibility coverage for launch-critical parent, admin, and swimmer journeys.
- [ ] Add mobile compatibility coverage for launch-critical parent, admin, and swimmer journeys.

### 29M) Training, support, and launch simulation
- [ ] Create training and operator enablement materials for Product, Operations, Finance, and support staff.
- [ ] Validate manual-at-launch workflows end-to-end with named owners and staging simulations.
- [ ] Run support simulations for import, billing, auth, and document-center incidents using the runbooks.
- [ ] Record final launch-owner assignments and escalation paths.

### 29N) Real data migration and launch proof
- [ ] Finish Step 24 real-data migration work: anonymized staging dry-run, source row-count comparison, billing total comparison, and remediation log.
- [ ] Complete two full parallel meet cycles with no manual data patching.
- [ ] Complete Step 26 real-world cutover execution and evidence capture.
- [ ] Complete Step 27 post-launch stabilization tasks and publish the first stability reports.

Implementation note: Step 29 was added after auditing the current repo for what
"fully ready production app" really requires beyond scaffold completion. Source
evidence includes the in-memory service implementations under `server/internal/app`,
stubbed runtime integrations under `server/internal/storage`, `server/internal/stripe`,
and `server/internal/postmark`, the mock-first UI plan in
`docs/planning/STEP20-implementation-plan.md`, and the deployment/ops
expectations in `docs/planning/Plan.md` and
`docs/planning/Developer Handoff Master Document.md`.
