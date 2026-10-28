# DARE Platform Implementation TODO (Atomic Task List)

---

## 24) Migration prep and dry-runs (blocked on staging environment)
- [ ] Run dry-run migration in staging with anonymized data.
- [ ] Compare row counts with source system.
- [ ] Compare billing totals with source system.
- [ ] Log all manual remediation items.

---

## 29C) Payments and email providers
- [x] Replace the Stripe stub runtime with the real Stripe SDK client and production payment/checkout configuration.
- [ ] Validate real Stripe webhook, reconciliation, refund, and balance-fetch flows in staging with live credentials.
- [ ] Configure a real Postmark client for production sends and verify outbound delivery against sandbox/prod streams.
- [x] Implement inbound email delivery/bounce/suppression webhook handling for production delivery-state accuracy.

---

## 29F) Parent and swimmer portal — remaining
- [x] Add end-to-end regression coverage for parent and swimmer portal critical paths using live backend state.
- [x] Remove the swimmer portal feature flag and "Coming Soon" screen; `/swim` is always enabled for the `swimmer` role.

---

## 29N) Real data migration and launch proof
- [ ] Finish Step 24 real-data migration work: anonymized staging dry-run, source row-count comparison, billing total comparison, and remediation log.
- [ ] Complete two full parallel meet cycles with no manual data patching.
- [ ] Complete Step 26 real-world cutover execution and evidence capture.
- [ ] Complete Step 27 post-launch stabilization tasks and publish the first stability reports.

---

## 30) Keycloak identity and user management integration

This step builds full in-app user lifecycle management backed by the Keycloak
Admin REST API. The goal: admins can invite, configure, and remove users entirely
from the DARE dashboard without ever touching the Keycloak UI.

### 30A) Keycloak Admin API client (Go backend)

- [x] Add `server/internal/keycloak` package with Admin REST API client.
- [x] Add Keycloak Admin credentials to env config: `KEYCLOAK_ADMIN_URL`, `KEYCLOAK_ADMIN_CLIENT_ID`, `KEYCLOAK_ADMIN_CLIENT_SECRET`, `KEYCLOAK_REALM`.
- [x] Implement client-credentials token fetch and auto-refresh for admin API calls.
- [x] Implement `ListUsers(orgID, search, limit, offset)` — query realm users filtered by `org_id` attribute.
- [x] Implement `GetUser(userID)` — fetch user by Keycloak ID.
- [x] Implement `CreateUser(email, firstName, lastName, orgID)` — provision new Keycloak user with `org_id` custom attribute and `UNVERIFIED` status.
- [x] Implement `SendVerificationEmail(userID)` — trigger Keycloak email verification / welcome email.
- [x] Implement `UpdateUser(userID, fields)` — patch display name, email, enabled state.
- [x] Implement `DeleteUser(userID)` — hard delete from Keycloak (admin/owner only, with confirmation guard).
- [x] Implement `DisableUser(userID)` / `EnableUser(userID)` — set `enabled` flag in Keycloak.
- [x] Implement `GetUserRealmRoles(userID)` — list assigned realm roles.
- [x] Implement `AssignRealmRole(userID, roleName)` — add realm role by name.
- [x] Implement `RemoveRealmRole(userID, roleName)` — remove realm role by name.
- [x] Implement `EnsureRealmRolesExist()` — idempotently create `owner`, `admin`, `coach`, `parent`, `swimmer`, `billing`, `read-only` realm roles on startup.
- [x] Implement `SendPasswordResetEmail(userID)` — trigger Keycloak password reset action email.
- [x] Add `org_id` protocol mapper in Keycloak realm config doc: user attribute `org_id` → ID token claim `org_id` (instead of relying solely on state param fallback).
- [x] Implement proper OIDC end_session logout: store the `id_token` from the OIDC token exchange in the session record, and on `POST /api/v1/auth/sessions/logout` redirect through Keycloak's `end_session_endpoint` with `id_token_hint` and `post_logout_redirect_uri`. This terminates the Keycloak SSO session server-side so a subsequent SSO authorize request cannot silently re-authenticate the previous user. (Current workaround: `prompt=login` is set on every authorize request to force re-auth, but the Keycloak SSO session is never actually invalidated.)
- [x] Add integration tests for Keycloak client with a test realm or mock HTTP server.

### 30B) User management API endpoints (Go)

- [x] Add `GET /api/v1/users` — list users in org, with `search`, `role`, `status` filters. Owner/admin only.
- [x] Add `GET /api/v1/users/{id}` — get user detail (Keycloak profile + DARE roles + linked household).
- [x] Add `POST /api/v1/users/invite` — provision Keycloak user + send welcome email + optionally create linked household. Owner/admin only.
- [x] Add `PUT /api/v1/users/{id}` — update display name / email. Owner/admin only.
- [x] Add `POST /api/v1/users/{id}/disable` — disable Keycloak account. Owner/admin only.
- [x] Add `POST /api/v1/users/{id}/enable` — re-enable Keycloak account. Owner/admin only.
- [x] Add `DELETE /api/v1/users/{id}` — delete user from Keycloak and anonymize DARE records. Owner only, requires MFA.
- [x] Add `GET /api/v1/users/{id}/roles` — list assigned roles.
- [x] Add `POST /api/v1/users/{id}/roles` — assign role (`{role}` body). Owner/admin only.
- [x] Add `DELETE /api/v1/users/{id}/roles/{role}` — remove role. Owner/admin only. Cannot remove last `owner` role.
- [x] Add `POST /api/v1/users/{id}/password-reset` — send password reset email. Owner/admin only.
- [x] Enforce: no actor can assign/remove roles above their own tier (admin cannot promote to owner).
- [x] Enforce: org isolation — all user management calls validate `org_id` attribute matches request principal's org.
- [x] Emit audit event for every user management action (invite, role change, disable, delete).
- [x] Add OpenAPI docs for all new user management endpoints.
- [x] Add integration tests for invite, role assign/remove, disable/enable, and delete flows.
- [x] Add RBAC matrix tests for user management route authorization.

### 30C) `org_id` claim hardening

- [x] Document Keycloak protocol mapper setup: `User Attribute → org_id → ID token + access token` (in `docs/ops/keycloak-setup.md`).
- [x] Update `verifyAndParseIDToken` to log a warning when `org_id` falls back to the state parameter (so missing mappers are visible in logs).
- [x] Add startup validation: if `KEYCLOAK_REALM` is set, verify JWKS endpoint is reachable and realm roles exist.
- [x] Add `keycloak-setup.md` operator runbook covering: realm creation, client config, valid redirect URIs, protocol mappers, realm roles, and first-admin bootstrap.

### 30D) User management admin UI

- [x] Add **Users** page at `/users` in admin sidebar (owner/admin role only).
- [x] Build user list table: email, name, role badges, status (active/disabled), last-login (if available), household link.
- [x] Add search/filter bar: by name/email, by role, by status.
- [x] Add **Invite User** slide-over:
  - Fields: first name, last name, email, role (dropdown), optionally create linked household (yes/no toggle).
  - On submit: calls `POST /api/v1/users/invite`, shows success/error.
- [x] Build **User Detail** page at `/users/[id]`:
  - Shows Keycloak profile fields (name, email, status).
  - Shows current role(s) with add/remove controls.
  - Shows linked household (if any) with link to household detail.
  - Shows audit trail for this user's management events.
  - Action buttons: Reset Password, Disable/Enable, Delete (owner only, confirm dialog).
- [x] Add role assignment controls on user detail: role chip list with `+` (add) and `×` (remove per chip).
- [x] Enforce UI-level tier guard: admin cannot see the owner role option; only owner can assign/remove owner.
- [x] Add confirmation modal for destructive actions (disable, delete).
- [x] Add delete confirmation with typed-email verification for irreversible deletes.
- [x] Wire all mutations to live API with loading/error states.
- [x] Add `/users` and `/users/[id]` to nav visibility rules (owner/admin only).

### 30E) Household-to-Keycloak user linkage

- [x] Add `keycloak_user_id` column to the `households` table (migration).
- [x] On invite with "create household" option: auto-create household record and set `keycloak_user_id`.
- [x] On OIDC callback: if household with matching `keycloak_user_id` exists, attach `household_id` to the session principal.
- [x] Add `GET /api/v1/users/{id}/household` — return linked household summary.
- [x] Add `POST /api/v1/users/{id}/household/link` — link existing household to user. Owner/admin only.
- [x] Add `POST /api/v1/users/{id}/household/unlink` — unlink. Owner/admin only.
- [x] Add migration + backfill script for any existing households whose contact email matches a Keycloak user email.

### 30F) Self-service flows

- [x] Add `/register` page (public): email collection form → calls `POST /api/v1/users/invite` with `role=parent` (or triggers a pending-approval flow).
- [x] Add pending-approval state for self-registered users: `invited` → `pending_approval` → `active`.
- [x] Add admin **Approvals** queue page at `/members/approvals` to review and approve pending registrations.
- [x] Add `POST /api/v1/users/{id}/approve` endpoint. Owner/admin only. Triggers welcome email and activates account.
- [x] Add `POST /api/v1/users/{id}/reject` endpoint with optional reason. Owner/admin only.
- [x] Emit audit event for approve/reject actions.
- [x] Add notification to admin when a new self-registration arrives (in-app alert or email).

---

## 31) Schema drift fixes (observed on startup)

These columns are referenced in repository code but missing from current migrations.
Each needs a migration to add the column (or the repository query needs to be corrected).

- [x] `eligibility_rules.rule_value` — add column migration or fix repository query.
- [x] `meets.venue` — add column migration or fix repository query.
- [x] `file_exports.filename` — add column migration or fix repository query.
- [x] `announcements.deleted_at` — add soft-delete column migration.
- [x] `data_export_requests.deleted_at` — add soft-delete column migration.
- [x] `invoices` — fix `""` date parsing error (empty string stored for date field; add NOT NULL constraint or coerce empty to NULL in repository).
- [x] Run `make db-up` after all migrations are applied and verify server starts with zero repository errors.
- [x] Add a startup schema-check that logs a clear error (not a panic) if expected columns are missing.

---

## 32) Additional production hardening gaps (code/application only)

### 32A) Startup and runtime fail-closed behavior
- [x] In non-development environments, fail startup if database connection cannot be established (no in-memory fallback).
- [x] In non-development environments, fail startup if object storage initialization fails (no stub storage fallback).
- [x] Add explicit startup checks that validate all required runtime dependencies and emit actionable errors.

### 32B) Durable background job processing
- [x] Replace the in-memory worker queue/server with a durable shared backend (Asynq/Redis) for production execution.
- [x] Ensure DLQ/retry/metrics endpoints read from the durable backend rather than process-local memory.
- [x] Add restart/recovery tests proving queued jobs survive API restarts.

### 32C) Health/readiness correctness
- [x] Replace stub readiness values with real dependency checks (database, queue backend, storage, provider clients as applicable).
- [x] Return non-ready status when critical dependencies are unavailable.
- [x] Remove all stubs with real prod ready items
- [x] Add tests for readiness failure modes.

### 32D) Remove remaining UI stub flows
- [x] Replace portal document upload stub flow with real upload + attachment flow (presigned upload/object key validation).
- [x] Replace meet creation “simulated parse/mock sessions” flow with real backend parsing and derived sessions.
- [x] Add regression coverage for both flows against live backend state.

### 32E) Stateful service consistency and persistence hardening
- [x] Eliminate process-local state as source of truth for core domains; ensure DB-backed consistency across multiple API instances.
- [x] Add concurrency and multi-instance consistency tests for write paths in members/groups/training/other stateful modules.
- [x] Validate cache/state reload correctness across restart and rolling update scenarios.

### 32F) Auth/session durability and scale safety
- [x] Replace in-memory session/refresh-token store with persistent/shared session storage suitable for multi-instance deployments.
- [x] Add tests for session refresh/revoke behavior across process restart and cross-instance routing.
- [x] Add secure session lifecycle controls (rotation/invalidation/TTL validation) tied to persistent storage.

### 32G) E2E test harness implementation in repo
- [x] Implement real E2E suite (Playwright or Cypress) for critical parent/admin/swimmer flows documented in testing plans.
- [x] Add E2E test scripts to `admin/package.json` and wire into CI quality gates.
- [x] Add deterministic seeded test fixtures for E2E environments.

### 32H) Org scope middleware hardening
- [x] Replace `OrgScopeStub` naming/implementation with hardened production org-scope middleware.
- [x] Centralize org-scope enforcement to reduce per-handler duplication and drift risk.
- [x] Add contract tests to ensure org-scope invariants hold across all protected routes.

### 32I) Frontend build resilience
- [x] Remove hard dependency on external Google Fonts fetch during production build (self-host or bundled font assets).
- [x] Add build-mode checks to ensure offline/restricted-network builds remain reproducible.

### 32J) OpenAPI serving robustness
- [x] Replace runtime file-path dependency for `/api/v1/openapi` with an embedded or packaged spec artifact strategy.
- [x] Add startup/check tests ensuring OpenAPI endpoint is always available in production runtime layouts.

### 32K) Static analysis and lint hygiene
- [x] Resolve current React Hooks dependency lint warning on approvals page.
- [x] Enforce zero-warning lint threshold for production branch gates (or document explicit waivers).

---

## 33) Additional pre-production code hardening gaps (newly identified)

### 33A) Auth/session token exposure hardening
- [x] Stop returning access/refresh session tokens in JSON response bodies for cookie-auth flows (`oidcCallback`, `issueSession`, `refreshSession`); rely on secure HttpOnly cookies and return only non-sensitive session metadata.
- [x] Add regression tests proving auth endpoints do not leak token values in response JSON while preserving cookie issuance/refresh behavior.

### 33B) CSRF and CORS contract alignment
- [x] Add `X-CSRF-Token` to CORS `AllowedHeaders` for browser mutation flows using cookie auth.
- [x] Add integration tests that exercise cross-origin preflight + cookie-auth mutation requests to validate CSRF header acceptance.

### 33C) Environment fail-safe defaults
- [x] Treat unset `APP_ENV` as non-development for security-sensitive defaults (or require explicit environment value at startup).
- [x] Add startup validation that fails with actionable error when `APP_ENV` is missing/invalid in non-local runtime modes.

### 33D) Request body parsing consistency and DoS hardening
- [x] Standardize all JSON handlers to use bounded body reads, `DisallowUnknownFields`, and single-object decode enforcement.
- [x] Add shared helper/middleware for decode+validate to prevent per-handler drift.
- [x] Add contract tests for oversized bodies, unknown fields, and trailing JSON payload rejection on representative routes.

### 33E) Stripe idempotency for payment creation flows
- [x] Add idempotency key support for `POST /stripe/payment-intents` and `POST /stripe/checkout-sessions` request paths.
- [x] Wire idempotency keys through Stripe SDK calls and add duplicate-request tests to verify exactly-once semantics under retries/timeouts.

### 33F) Metrics endpoint code-level access controls
- [x] Add app-level controls for `/metrics` exposure (bind-address segregation, auth middleware, or explicit feature gate) so metrics visibility is not solely infrastructure-dependent.
- [x] Add startup/logging checks that loudly report insecure metrics exposure in non-development environments.

### 33G) Comms recipient safety completion
- [x] Replace env-flagged arbitrary recipient acceptance with server-side org-member recipient resolution and eligibility enforcement.
- [x] Add validation/audit tests preventing campaigns from sending to recipients outside scoped org membership.

### 33H) Pagination and result-size guardrails
- [x] Add pagination/cursor support with sensible defaults and max limits on list endpoints that currently return unbounded datasets.
- [x] Add API contract tests ensuring limit ceilings are enforced and response metadata is stable.

### 33I) CSRF token generation failure behavior
- [x] Replace timestamp-based CSRF token fallback with fail-closed cryptographic handling (or explicit startup/runtime error path) when secure randomness is unavailable.
- [x] Add tests covering RNG failure path to guarantee no predictable token material is ever emitted.

---

## 34) Additional frontend/backend parity and account-flow gaps (newly identified)

### 34A) Parent/swimmer role parity and endpoint alignment
- [x] Resolve parent portal dependency on org-wide list endpoints that currently deny parent role access (`/members/swimmers`, `/members/households`, `/registrations`) by either:
  - [x] adding parent-scoped endpoints, or
  - [x] extending existing handlers with strict parent household scoping.
- [x] Add contract/integration tests proving parent accounts can load all parent portal pages without forbidden errors while preserving org/household isolation.
- [x] Resolve swimmer portal API access mismatches (swimmer account routed to `/swim` but swimmer role lacks required read access on key endpoints used by swimmer pages).
- [x] Add explicit role matrix tests for `parent` and `swimmer` across members/meets/performance/training endpoints consumed by portal UIs.

### 34B) Real registration and approval lifecycle completion
- [x] Replace public `/register` simulated submit flow with a real backend registration request endpoint and persistence.
- [x] Implement backend access-request lifecycle (create, review, approve, reject) tied to user identity/account state rather than household status toggles.
- [x] Refactor admin `/members/approvals` to use real request/user approval APIs (not `household.status` mutation as a stand-in).
- [x] Emit audit events and notifications for registration submitted/approved/rejected transitions.
- [x] Add end-to-end tests from public registration through admin approval/rejection to account activation.

### 34C) Parent portal registration wizard backend wiring
- [x] Replace hardcoded `/portal/registrations/new` data (seasons/swimmers/waivers) with live backend APIs.
- [x] Wire registration submission to create real registration records and required waiver acceptances.
- [x] Ensure resulting registrations and waiver state appear correctly in `/portal/registrations` and admin views.
- [x] Add regression coverage for multi-swimmer household registration flows.

### 34D) Meet files parity for parent/swimmer experiences
- [x] Implement meet-files API contract (list/download) and data model integration for parent/swimmer meet detail pages.
- [x] Replace meet-files placeholder panels in portal/swimmer UIs with real file listings and access controls.
- [x] Add role/ownership tests ensuring only authorized household/swimmer users can access allowed meet files.

### 34E) Remaining placeholder UI workflows that need backend persistence
- [x] Implement swimmer bio persistence (enable save on swimmer profile and back it with API/storage).
- [x] Implement swimmer cut-qualification tab wiring to live time-standard qualification data where intended.
- [x] Persist parent communications notification preferences via backend API (replace local-only `savePrefs` behavior).
- [x] Implement real autopay management backend integration for parent billing autopay tab (replace local-only toggle/config state).
- [x] Implement swimmer household activity history feed backing API and replace placeholder history panel.
- [x] Add end-to-end coverage for each replaced placeholder flow.

### 33J) Frontend multi-role fidelity
- [x] Preserve and expose full backend role set in frontend session state instead of collapsing to a single primary role.
- [x] Refactor nav/feature guards to evaluate capability from complete role claims; add regression tests for mixed-role users.

### 33K) Direct test coverage expansion for high-risk packages
- [x] Add package-level unit/integration tests for currently under-covered core areas (handlers/compliance/documents/imports/meets/performance/reports/training/workers).
- [x] Define minimum coverage thresholds for critical backend packages and enforce in CI gates.

---

## 34) Additional frontend/backend completion gaps (newly identified)

### 34A) Tenant-aware auth initiation
- [x] Remove hardcoded `org_id=org-demo` from login initiation; derive tenant/org context from the selected club domain, user input, or server-side resolver.
- [x] Add tests proving OIDC authorize requests carry the correct org context per tenant and do not default to a shared demo org in production.

### 34B) Public registration flow backend integration
- [x] Replace `/register` simulated submission with a real backend flow (access request or invite pipeline) including validation, persistence, duplicate handling, and confirmation state.
- [x] Add API + UI regression tests covering successful registration, duplicate email, invalid payload, and approval handoff behavior.

### 34C) Portal registration wizard live data wiring
- [x] Replace static seasons/swimmers/waivers on `/portal/registrations/new` with live API-backed data and persistence.
- [x] Implement real registration submit flow (including waiver acceptance writes) and add end-to-end coverage for multi-swimmer registration.

### 34D) Practice scheduler page parity
- [x] Replace demo-only `/practices` admin page with live API wiring and persistence semantics that match training/practice backend behavior.
- [x] Remove any remaining illustrative-only / not persisted UX paths once live integration is complete.

### 34E) Autopay management implementation
- [x] Implement backend endpoints and frontend wiring for autopay enrollment/status management (list households, enroll/opt-out, method metadata, next charge preview).
- [x] Add role enforcement, audit events, and regression tests for autopay mutation and visibility rules.

### 34F) Portal meet artifacts/files integration
- [x] Add backend and frontend support for meet files surfaced in portal meet detail (`/portal/meets/[id]`) instead of placeholder panel.
- [x] Define authorization and object-link strategy for parent/swimmer-safe access to meet artifacts.

### 34G) Swimmer profile persistence completion
- [x] Implement backend persistence and API wiring for swimmer profile bio editing currently rendered as disabled “coming soon”.
- [x] Add validation/sanitization and regression tests for profile update/read behavior.

### 34H) Swimmer cut qualification UI hookup
- [x] Wire swimmer performance “Cut Qualifications” tab to existing cut-qualification API and time standards data.
- [x] Add empty/error/loading states and tests verifying expected behavior across qualified/not-qualified/no-standard scenarios.

### 34I) Portal registration detail waiver state integration
- [x] Replace registrations page waiver placeholder with live waiver/document status derived from registration/document APIs.
- [x] Add tests ensuring waiver status visibility is scoped to the household and consistent with backend transitions.

### 34J) Error visibility and fallback hardening in UI
- [x] Replace silent `catch`/fallback patterns in critical admin and portal flows with explicit error states, retry affordances, and telemetry hooks.
- [x] Add UI tests ensuring failed API calls do not present stale/demo state as if operations succeeded.

### 34K) Impersonation implementation completion
- [x] Either fully implement impersonation flow behind existing safeguards (owner-only + MFA + reason + audit/session controls) or remove/disable the feature path to avoid exposed 501 behavior in enabled environments.
- [x] Add contract tests validating impersonation authorization boundaries and audit requirements.

---

## 35) Additional frontend/backend contract and wiring gaps (newly identified)

### 35A) Broken meet write-route contracts in frontend
- [x] Replace frontend calls to `/api/v1/orgs/{orgId}/meets/...` with the actual backend route shape `/api/v1/meets/{meetId}/...` for entry creation and commitment response flows.
- [x] Add integration/regression tests proving admin entry creation and portal commitment response hit live routes successfully (no 404 contract drift).

### 35B) Admin entries mutation error handling correctness
- [x] Remove per-request error swallowing in admin entry creation (`Promise.all(...catch(...))`) so failed entry writes are surfaced to the user and block success state.
- [x] Add UI tests verifying partial/complete entry-create failures produce visible errors and do not show false-positive “saved” state.

### 35C) Portal commitment event-selection persistence hookup
- [x] Wire portal meet detail event selection UI to existing commitment event-selection endpoints (create/list/delete) so selected events are persisted and reloaded.
- [x] Add end-to-end coverage confirming event selections survive refresh and respect commitment/window constraints.

### 35D) Swimmer meet detail write-path completion
- [x] Implement real mutation wiring for swimmer meet detail commitment/event selection interactions (currently local-only state changes).
- [x] Add tests verifying swimmer-side updates persist and reconcile with backend commitment records.

### 35E) Meet files panel completion in portal and swimmer surfaces
- [x] Replace placeholder “Meet Files” panels in parent/swimmer meet detail pages with live file listing/download integration.
- [x] Add authorization tests ensuring household/swimmer users only access permitted meet artifacts.

### 35F) Swimmer profile bio persistence
- [x] Implement enabled save flow for swimmer profile bio (API route + persistence + frontend mutation wiring) to replace current disabled “coming soon” control.
- [x] Add validation and regression tests for bio create/update/read behavior.

### 35G) Swimmer cut-qualification tab implementation
- [x] Replace “Cut qualifications coming soon” placeholder with live cut-qualification data rendering from backend APIs/time standards.
- [x] Add UI tests for qualified/not-qualified/no-data/error states on swimmer performance cuts tab.

### 35H) Autopay production implementation parity
- [x] Replace `/billing/autopay` demo-only household state with live backend-backed autopay enrollment/status and payment-method metadata.
- [x] Add backend API coverage and role/audit tests for autopay list and enroll/opt-out mutations.

### 35I) Duplicate practices UX parity decision
- [x] Decide and execute a single source of truth between `/training` (live API-backed) and `/practices` (demo-only) to prevent operator confusion and split behavior. NOTE: MUST BE LIVE API
- [x] If `/practices` is retained, wire it to live training/practices APIs; if not retained, remove it from nav/routes and add redirect coverage.
---

## 36) Additional production readiness gaps (newly identified)

### 36A) Portal invoice checkout wiring
- [x] Replace simulated portal invoice payment flow (`alert('Stripe Checkout would launch here in production.')`) with real checkout session creation + redirect handling.
- [x] Add UI/integration tests covering successful checkout initiation, API failure handling, and disabled states during redirect.

### 36B) Portal communications preference persistence
- [x] Replace local-only notification preference save on `/portal/communications` with real backend persistence and retrieval (email/push channel settings per user/household).
- [x] Add API + UI regression tests ensuring preference changes survive refresh/session restart and respect org/household scoping.

### 36C) Import pipeline implementation completion
- [x] Replace synthetic import finalize behavior (`conflicts_pending` + synthetic rows) with real parse/match/report generation from uploaded artifacts.
- [x] Replace scaffolded `TypeParseImport` worker handler simulation with production parser invocation and durable status/report writes.
- [x] Add integration tests that validate real end-to-end import behavior: upload -> finalize -> parse outcomes -> conflict/report correctness.

### 36D) Runtime environment file loading safety
- [x] Restrict `.env.local` auto-loading in `server/cmd/api/main.go` to explicit local/development modes so non-local runtimes do not silently consume local override files.
- [x] Add startup tests/validation to prove production/staging modes do not load accidental local env files and produce deterministic config resolution.
