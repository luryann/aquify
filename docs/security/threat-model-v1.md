# aquify Platform Threat Model v1

Date: 2026-03-18
Status: Draft v1
Scope: Launch-blocking platform surfaces through STEP 22

## System summary

aquify Platform is a swim club operations application with:

- Go API at `/api/v1`
- Next.js admin and portal clients
- PostgreSQL for operational data
- Redis-backed background jobs
- S3-compatible object storage for import/export artifacts
- OIDC authentication plus platform-issued sessions
- Stripe and Postmark integrations

Primary sensitive domains:

- household and swimmer PII
- medical and emergency contact data
- billing and refund workflows
- imported meet result files
- privileged admin/coach actions

## Trust boundaries

1. Browser and mobile clients to API
2. API to OIDC provider
3. API to Stripe/Postmark/object storage
4. API to worker queue and background job execution
5. Tenant/org boundary between households, staff, and swimmers

## Key assets

- access and refresh session tokens
- CSRF tokens
- user roles and MFA state
- household financial history
- swimmer identifiers and results history
- imported meet files and generated reports
- audit trail integrity
- Stripe webhook authenticity

## Primary threats and controls

### Session theft or misuse

Threats:

- stolen bearer tokens
- cookie replay over plaintext transport
- CSRF against cookie-backed session refresh/revoke flows

Controls:

- Secure, HttpOnly, SameSite=Strict access and refresh cookies
- CSRF double-submit token enforcement for cookie-backed auth session mutations
- development/test-only direct session issuance path disabled outside approved local flows
- deny unknown or expired session tokens
- MFA step-up for privileged actions

### Privilege escalation

Threats:

- role assignment without MFA
- billing-sensitive actions without step-up
- future impersonation capability being enabled unsafely

Controls:

- centralized permission rules requiring MFA for role assignment, refunds, voids, and write-offs
- impersonation headers rejected by default
- if impersonation is ever enabled, owner role plus MFA plus reason are required before any implementation path is allowed

### Cross-tenant data exposure

Threats:

- using a valid token with a mismatched `X-Org-Id`
- cross-household reads by parent/swimmer roles

Controls:

- org-scope enforcement on protected routes
- role- and household-aware service checks
- dedicated authz tests for parent/swimmer boundaries

### Malicious file upload and parser abuse

Threats:

- oversized uploads
- unsupported or misleading MIME/type combinations
- path traversal in filenames
- malware or hostile payloads reaching downstream parsing jobs

Controls:

- filename basename validation
- explicit size cap on import upload requests
- upload MIME allowlist
- malware-scan hook invoked before import finalization proceeds
- required scan mode rejects missing or `stub` scanner integrations
- import failure if the scan reports a non-clean result

Residual risk:

- current presigned upload flow validates declared metadata rather than independently inspecting stored object bytes
- future hardening should add storage-side size/content enforcement and signed upload policies

### Automated abuse and brute-force traffic

Threats:

- repeated session endpoint attempts
- generalized request floods

Controls:

- global in-memory rate limiting
- stricter auth-route rate limiting

Residual risk:

- current rate limiter is single-process memory only
- production multi-instance deployment should move limits to shared infrastructure or edge enforcement

### Client-side injection and browser abuse

Threats:

- clickjacking
- content sniffing
- unsafe embedding

Controls:

- baseline security headers middleware:
  - `Content-Security-Policy`
  - `X-Frame-Options: DENY`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: no-referrer`
  - `Permissions-Policy`
  - `Strict-Transport-Security` on HTTPS requests

## Assumptions

- TLS termination is correctly configured in non-local environments.
- OIDC provider signs ID tokens with trusted keys and rotates JWKS normally.
- Stripe and Postmark credentials are stored outside source control.
- Dev stub bearer tokens remain disabled outside local development.

## Deferred follow-ups

- shared/distributed rate limiting for multi-instance deployments
- storage-enforced upload policies and object metadata verification
- full impersonation design with session lineage and dedicated audit model
- WAF/CDN-level request filtering and bot mitigation validation
- periodic threat model review after migration/cutover features land

## Multi-tenancy threats (added 2026-11-03)

Scope: several clubs on one deployment. Controls and the tests that prove them are listed in `docs/multi-tenancy-implementation-tracker.md`.

| Threat | Controls | Residual risk |
|---|---|---|
| **Cross-org read/write by guessing ids (IDOR)** | org taken from the authenticated session (`X-Org-Id` is only an assertion); services filter by org; composite FKs make the database refuse a child that points at another org's parent; SQL guard test fails on by-id statements without `org_id`; the cross-org matrix walks every route with another org's ids | No row-level security: a new query that forgets `org_id` is caught by tests, not by the database (tracker 4.4) |
| **Tenant confusion on public surfaces** (sign-up, public site, custom domains) | the browser never supplies an org id: slug or verified custom domain is resolved server-side; one slug cannot address two clubs (cross-namespace check); unknown, unverified and inactive clubs return the same 404; DNS TXT proves domain ownership (a TXT record naming another org is rejected) | slug enumeration is possible for clubs with a public site (by design) |
| **Account re-homing / privilege crossing** | a user is bound to one org; a token claiming another org is refused (`org_mismatch`); platform operator access needs role + platform org membership + MFA; org admins cannot grant `platform_admin`; invite roles cannot be admin | Keycloak realm admins are trusted |
| **Forged or mis-routed webhooks** | Connect events are attributed by the connected account (never metadata alone); metadata naming a different org is a permanent rejection; account rows are one-to-one with orgs; Stripe idempotency keys are namespaced per org | Postmark webhooks still authenticate with one shared secret and carry `org_id` in metadata |
| **Confused deputy in background jobs** | jobs carry `org_id`; scheduled fan-out only targets active orgs; offboarding purges by `org_id` only and refuses the platform org | queued jobs for a suspended org are not re-checked at execution time |
| **One club sending as another** (shared mail account) | campaign From address is pinned to the org's configured sender outside development | Postmark sender-signature verification is not automated |
| **Stored secret transplant** | integration secrets are AES-GCM-sealed with the org id as associated data | single global encryption key |
| **Shared-device leakage in the browser** | roll-call rosters/queues are keyed by org+user and wiped at sign-out | other localStorage values hold no tenant data |
| **Noisy neighbour / exhaustion** | per-IP and per-org API rate limits; per-org metrics with bounded label cardinality | no storage/email quotas; shared in-memory caches |
| **Support access abuse** | no impersonation (fails closed); platform actions are audited under the platform org | operators can pause/close any club by design |
| **Offboarding data loss or retention** | cooling-off period, typed confirmation, export before delete, audit trail retained, single-org restore from export | stored files are not part of the archive (manifest only) |
