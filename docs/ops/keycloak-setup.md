# Keycloak Setup Operator Runbook

This runbook covers Keycloak configuration for the DARE platform — from local
development bootstrapping to production realm setup.

---

## Table of Contents

1. [Local Development (Automated Bootstrap)](#1-local-development-automated-bootstrap)
2. [Production / Staging Realm Setup](#2-production--staging-realm-setup)
3. [Protocol Mappers — `org_id` Claim Hardening](#3-protocol-mappers--org_id-claim-hardening)
4. [Realm Roles](#4-realm-roles)
5. [Client Configuration](#5-client-configuration)
6. [Valid Redirect URIs](#6-valid-redirect-uris)
7. [First-Admin Bootstrap](#7-first-admin-bootstrap)
8. [Required App Environment Variables](#8-required-app-environment-variables)
9. [Security Defaults Checklist](#9-security-defaults-checklist)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. Local Development (Automated Bootstrap)

The repository includes a reproducible local Keycloak bootstrap that does not
require any manual UI configuration.

### What is provisioned

From `keycloak/realm-aquify-local.json`:

| Resource | Value |
|---|---|
| Realm | `aquify` |
| OIDC client | `aquify-id` (confidential) |
| Client secret | `aquify-local-dev-secret` |
| Redirect URIs | `http://localhost:3000/auth/callback`, `http://127.0.0.1:3000/auth/callback` |
| Protocol mappers | realm roles → `realm_access.roles`; user attribute `org_id` → token claim `org_id` |
| Realm roles | `owner`, `admin`, `coach`, `parent`, `swimmer`, `billing`, `read_only_staff` |

Seed users (password: `ChangeMe123!`):
- `owner@dare.local`, `admin@dare.local`, `coach@dare.local`
- `parent@dare.local`, `swimmer@dare.local`, `billing@dare.local`, `readonly@dare.local`

### Bootstrap commands

```sh
# Start local infrastructure (Postgres, Redis, MinIO, Keycloak, Mailpit):
make infra-up

# Re-run only Keycloak bootstrap if needed (idempotent):
make keycloak-bootstrap
```

The bootstrap is implemented by `scripts/keycloak-bootstrap.sh` and re-creates
the `aquify` realm on each run (safe for local dev; never run against production).

### Required local env vars

```env
AUTH_ENABLE_DEV_STUB_TOKENS=false
AUTH_ALLOW_DEV_SESSION_ISSUE=false
OIDC_AUTHORIZE_URL=http://localhost:8180/realms/aquify/protocol/openid-connect/auth
OIDC_TOKEN_URL=http://localhost:8180/realms/aquify/protocol/openid-connect/token
OIDC_JWKS_URL=http://localhost:8180/realms/aquify/protocol/openid-connect/certs
OIDC_END_SESSION_URL=http://localhost:8180/realms/aquify/protocol/openid-connect/logout
OIDC_REDIRECT_URL=http://localhost:3000/auth/callback
OIDC_POST_LOGOUT_REDIRECT_URL=http://localhost:3000/login
OIDC_CLIENT_ID=aquify-id
OIDC_CLIENT_SECRET=aquify-local-dev-secret
OIDC_EXPECTED_ISSUER=http://localhost:8180/realms/aquify
KEYCLOAK_ADMIN_URL=http://localhost:8180
KEYCLOAK_REALM=aquify
KEYCLOAK_ADMIN_CLIENT_ID=aquify-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<admin-service-account-secret>
```

---

## 2. Production / Staging Realm Setup

For production and staging, create dedicated realms rather than reusing the
local `aquify` realm. Never export local realm configs to production — they
contain dev secrets and permissive settings.

### Step-by-step realm creation

1. **Log in** to your Keycloak admin console (e.g. `https://auth.yourdomain.com/admin`).

2. **Create realm**:
   - Top-left dropdown → **Add realm**
   - Name: `dare` (or `dare-staging` for staging)
   - Enabled: `ON`
   - Click **Create**

3. **Realm settings**:
   - **General** tab:
     - Display name: `DARE Swim Club`
     - Frontend URL: `https://auth.yourdomain.com` (must match public DNS)
   - **Login** tab:
     - User registration: `OFF`
     - Forgot password: `ON`
     - Remember Me: `ON`
     - Email as username: `ON`
     - Login with email: `ON`
   - **Email** tab: configure SMTP server for password reset and verification emails
   - **Security Defenses** tab:
     - Brute force detection: `ON`
     - Max login failures: `10`
     - Wait increment: `60` seconds

4. Continue to **Section 4** (realm roles) and **Section 5** (client config).

---

## 3. Protocol Mappers — `org_id` Claim Hardening

**This is a required step.** Without the `org_id` protocol mapper, the API
falls back to resolving `org_id` from the OIDC state parameter, which produces
a warning in server logs:

```
level=WARN msg="oidc: org_id absent from ID token claims — falling back to state parameter; verify Keycloak protocol mapper (User Attribute 'org_id' → ID token claim 'org_id') is configured"
```

If you see this warning in production, the protocol mapper is not configured
correctly. Follow these steps to fix it.

### Create the `org_id` protocol mapper

In the Keycloak admin console for your realm:

1. Navigate to **Clients** → select your DARE client (e.g. `dare-api`)
2. Click the **Client scopes** tab
3. Click the dedicated scope for your client (e.g. `dare-api-dedicated`)
4. Click **Add mapper** → **By configuration** → **User Attribute**
5. Configure as follows:

| Field | Value |
|---|---|
| **Name** | `org_id` |
| **User Attribute** | `org_id` |
| **Token Claim Name** | `org_id` |
| **Claim JSON Type** | `String` |
| **Add to ID token** | `ON` |
| **Add to access token** | `ON` |
| **Add to userinfo** | `ON` |
| **Multivalued** | `OFF` |
| **Aggregate attribute values** | `OFF` |

6. Click **Save**

### Set `org_id` on users

Every Keycloak user in the DARE realm must have the `org_id` custom attribute
set to the organization's ID. When the DARE API invites a user (via
`POST /api/v1/users/invite`), it sets this attribute automatically.

To set it manually in the Keycloak UI:
1. Navigate to **Users** → select user
2. **Attributes** tab → click **Add attribute**
3. Key: `org_id`, Value: `<your-org-id>` (e.g. `org-dare-aquatics`)
4. Click **Save**

### Realm-level mapper (alternative: via realm scope)

If you want `org_id` included in tokens for all clients in the realm:

1. Navigate to **Client scopes** (top-level, not inside a client)
2. Select `profile` (or create a dedicated `org` scope)
3. **Mappers** tab → **Add mapper** → **By configuration** → **User Attribute**
4. Use the same configuration as above

---

## 4. Realm Roles

The DARE platform requires the following realm roles to exist. The API creates
them automatically on startup via `EnsureRealmRolesExist` when Keycloak admin
credentials are configured.

| Role | Description |
|---|---|
| `owner` | Full platform access. Can manage all users, billing, and configuration. |
| `admin` | Administrative access. Cannot assign/remove the `owner` role. |
| `coach` | Meet management, entry approval, group assignment. |
| `billing` | Invoice and payment management. |
| `parent` | Parent portal access. Scoped to household. |
| `swimmer` | Swimmer portal access. Scoped to swimmer record. |
| `read_only_staff` | Read-only access to admin views. No write operations. |

### Create realm roles manually (if needed)

In the Keycloak admin console:

1. Navigate to **Realm roles** → **Add role**
2. Create each role from the table above
3. For `owner`, add a **Description**: "Full platform owner — assign sparingly"

---

## 5. Client Configuration

### Create the DARE API client

1. Navigate to **Clients** → **Create client**
2. **General Settings**:
   - Client type: `OpenID Connect`
   - Client ID: `dare-api` (use `dare-api-staging` for staging)
   - Name: `DARE API`
   - Click **Next**
3. **Capability config**:
   - Client authentication: `ON` (confidential client)
   - Authorization: `OFF`
   - Authentication flow: enable **Standard flow** only
   - Click **Next**
4. **Login settings** (see Section 6 for redirect URIs)
   - Click **Save**

### Get the client secret

1. Open the client → **Credentials** tab
2. Copy the **Client secret** → store in your secrets manager
3. Set as `OIDC_CLIENT_SECRET` in the API environment

### Create the admin service account client

For the DARE Admin REST API client (used by `KEYCLOAK_ADMIN_CLIENT_ID`):

1. Navigate to **Clients** → **Create client**
2. Client ID: `dare-admin` (use `dare-admin-staging` for staging)
3. Client authentication: `ON`
4. Authentication flow: enable **Service accounts roles** only (no standard flow)
5. Click **Save**
6. **Service account roles** tab → **Assign role** → add:
   - `manage-users` (from `realm-management` client roles)
   - `manage-realm` (for role creation via `EnsureRealmRolesExist`)
   - `view-users`
   - `query-users`

---

## 6. Valid Redirect URIs

Configure redirect URIs on the DARE API client (**Settings** tab → **Valid redirect URIs**):

### Production
```
https://app.yourdomain.com/auth/callback
```

### Staging
```
https://staging.yourdomain.com/auth/callback
```

### Local development
```
http://localhost:3000/auth/callback
http://127.0.0.1:3000/auth/callback
```

**Web origins** (for CORS):

| Environment | Value |
|---|---|
| Production | `https://app.yourdomain.com` |
| Staging | `https://staging.yourdomain.com` |
| Local | `http://localhost:3000` |

> **Important**: Do not use `*` as a wildcard for redirect URIs in production.
> Exact URI matching prevents open redirect vulnerabilities.

---

## 7. First-Admin Bootstrap

After realm creation, provision the first owner account:

### Option A: Via Keycloak UI

1. Navigate to **Users** → **Add user**
2. Username: first owner's email address
3. **Credentials** tab → set a temporary password
4. **Attributes** tab → add `org_id` = `<your-org-id>`
5. **Role mappings** tab → assign realm role `owner`
6. Send a password reset email or provide the temp password out-of-band

### Option B: Via DARE invite API

Once the Keycloak admin client is configured and the API is running:

```sh
curl -X POST https://api.yourdomain.com/api/v1/users/invite \
  -H "Authorization: Bearer <owner-token>" \
  -H "X-Org-Id: <org-id>" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "owner@yourdomain.com",
    "first_name": "Admin",
    "last_name": "Owner",
    "role": "owner",
    "create_household": false
  }'
```

The API will provision the Keycloak user, set `org_id`, assign the `owner`
role, and send a welcome/verification email.

---

## 8. Required App Environment Variables

Full set of Keycloak-related environment variables:

```env
# OIDC (authentication)
OIDC_AUTHORIZE_URL=https://auth.yourdomain.com/realms/dare/protocol/openid-connect/auth
OIDC_TOKEN_URL=https://auth.yourdomain.com/realms/dare/protocol/openid-connect/token
OIDC_JWKS_URL=https://auth.yourdomain.com/realms/dare/protocol/openid-connect/certs
OIDC_END_SESSION_URL=https://auth.yourdomain.com/realms/dare/protocol/openid-connect/logout
OIDC_REDIRECT_URL=https://app.yourdomain.com/auth/callback
OIDC_POST_LOGOUT_REDIRECT_URL=https://app.yourdomain.com/login
OIDC_CLIENT_ID=dare-api
OIDC_CLIENT_SECRET=<client-secret-from-keycloak>
OIDC_EXPECTED_ISSUER=https://auth.yourdomain.com/realms/dare
OIDC_STATE_SIGNING_KEY=<random-32-byte-hex>

# Keycloak Admin API (user management)
KEYCLOAK_ADMIN_URL=https://auth.yourdomain.com
KEYCLOAK_REALM=dare
KEYCLOAK_ADMIN_CLIENT_ID=dare-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<admin-client-secret-from-keycloak>

# Security
AUTH_ENABLE_DEV_STUB_TOKENS=false
AUTH_ALLOW_DEV_SESSION_ISSUE=false
APP_ENV=production
```

---

## 9. Security Defaults Checklist

Before going live, verify the following in the Keycloak admin console:

- [ ] **Brute-force protection** is enabled (Realm settings → Security Defenses)
- [ ] **Public registration** is disabled (Realm settings → Login)
- [ ] **Duplicate emails** policy is set to `Deny` (prevents account takeover via email reuse)
- [ ] **SSL required** is set to `All requests` (Realm settings → General)
- [ ] **Admin console** is not publicly accessible (firewall rule or Cloudflare Access)
- [ ] **Session timeouts** are configured (Realm settings → Sessions → SSO Session Idle: 8h, Max: 24h)
- [ ] **`org_id` protocol mapper** is configured on the client (see Section 3)
- [ ] **Redirect URIs** do not contain wildcards in production (see Section 6)
- [ ] **Admin service account** has only the minimum required roles (see Section 5)
- [ ] **Local dev secrets** (`aquify-local-dev-secret`) are NOT used in staging/production
- [ ] **`AUTH_ENABLE_DEV_STUB_TOKENS=false`** is set in all non-local environments

---

## 10. Troubleshooting

### `org_id` warning in server logs

```
level=WARN msg="oidc: org_id absent from ID token claims — falling back to state parameter"
```

**Cause**: The `org_id` protocol mapper is not configured or the user does not
have the `org_id` attribute set.

**Fix**:
1. Verify the protocol mapper exists (see Section 3)
2. Verify the user has `org_id` set in their Keycloak attributes
3. Force a new login (existing tokens will not include the new claim until re-issued)

---

### Startup error: JWKS endpoint unreachable

```
keycloak startup: JWKS endpoint unreachable at "https://auth.yourdomain.com/..."
```

**Cause**: `KEYCLOAK_REALM` is set but the JWKS endpoint is not reachable from
the API server.

**Fix**:
1. Verify `OIDC_JWKS_URL` is correct
2. Verify Keycloak is running and healthy: `curl -s <OIDC_JWKS_URL> | jq .`
3. Check network/firewall rules between the API server and Keycloak
4. Check Keycloak logs for errors

---

### Startup warning: realm roles cannot be verified

```
level=WARN msg="keycloak startup: could not verify/create realm roles — role assignment features may be degraded"
```

**Cause**: `KEYCLOAK_ADMIN_CLIENT_ID` / `KEYCLOAK_ADMIN_CLIENT_SECRET` are not
set or the admin service account lacks `manage-realm` permission.

**Fix**:
1. Verify Keycloak admin env vars are set (see Section 8)
2. Verify the admin service account has `manage-realm` and `manage-users` roles
   assigned (see Section 5)
3. Verify the admin client is enabled in Keycloak

---

### `invalid_oidc_callback: id_token issuer mismatch`

**Cause**: `OIDC_EXPECTED_ISSUER` does not match the `iss` claim in the token.

**Fix**: Set `OIDC_EXPECTED_ISSUER` to exactly the value Keycloak uses for the
realm issuer. You can find it at:
```
curl https://auth.yourdomain.com/realms/dare/.well-known/openid-configuration | jq .issuer
```

---

### Token verification fails after Keycloak key rotation

Keycloak rotates signing keys periodically. The DARE API caches JWKS for 5
minutes. If tokens fail to verify immediately after a key rotation:
1. Wait up to 5 minutes for the JWKS cache to expire
2. Or restart the API server to force an immediate JWKS refresh
