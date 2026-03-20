# DARE Platform Threat Model v1

Date: 2026-03-18
Status: Draft v1
Scope: Launch-blocking platform surfaces through STEP 22

## System summary

DARE Platform is a swim club operations application with:

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
