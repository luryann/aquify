# API Key and Secret Storage Strategy (v1)

## Scope

This document defines how DARE Platform stores and uses sensitive credentials, including API keys, OAuth client secrets, webhook signing secrets, and encryption keys.

## Rules

1. Secrets are never committed to git, including encrypted blobs.
2. Local development uses `.env.local` only, sourced from `.env.local.example`.
3. Non-local environments store secrets in a managed secret store (cloud KMS/secret manager or equivalent).
4. Application process receives secrets at runtime through environment variables or secret mounts.
5. API keys that must be persisted are stored hashed (one-way) whenever plaintext retrieval is not required.
6. If reversible storage is required, secrets are encrypted using envelope encryption with a managed KMS key.
7. Secrets are scoped by environment (`dev`, `staging`, `prod`) and principle of least privilege.

## Rotation and Lifecycle

1. Rotate high-risk secrets (OIDC client secret, Stripe webhook secret, signing keys) at least every 90 days.
2. Rotate immediately after suspected exposure.
3. Keep dual-secret validation windows during planned rotation to avoid downtime.
4. Track last rotation date and owner in an ops runbook.

## Access Control and Audit

1. Secret read access is restricted to runtime service identity and minimum required operators.
2. Human access to production secrets requires break-glass process with ticket/reference.
3. All secret read/write actions must be auditable in the platform/infrastructure logs.

## Implementation Notes for This Repo

1. `.env.example` and `.env.local.example` define required keys with placeholders only.
2. Session refresh tokens are stored as hashes in database schema (`sessions.refresh_token_hash`), not plaintext.
3. Future API key tables should store:
   - `key_prefix` (display only)
   - `key_hash` (SHA-256 or stronger)
   - `last_used_at`
   - `revoked_at`
   - `created_by`

## Non-Goals

1. Building custom cryptographic primitives.
2. Storing long-lived root credentials inside application code.
