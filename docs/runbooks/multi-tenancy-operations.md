# Multi-tenancy operations runbook

Last updated: 2026-11-03. Tracker: `docs/multi-tenancy-implementation-tracker.md`. Decisions: `docs/decisions/2026-11-03-multi-tenancy.md`.

This is for platform operators and on-call engineers. A "club" is one organization (`org`). Everything a club owns carries its `org_id`; clubs never share data, payments, sign-ins or email identity.

## 1. One-time platform setup

1. **Choose the platform org id** (a UUID) and set `PLATFORM_ORG_ID` on the API. At startup the API registers it in the `orgs` table as an active org with slug `platform`. Without this variable the operator plane does not exist.
2. **Keycloak**: create the realm role `platform_admin`. Create each operator as a normal user with attribute `org_id` = the platform org id, require MFA, and assign `platform_admin`. Org admins cannot grant this role (it is outside the assignable role tiers), and the role only works for members of the platform org.
   **Keycloak user profile (every realm, including production):** `org_id` must be a declared attribute with `edit: [admin]`, and `unmanagedAttributePolicy` must be `ADMIN_EDIT` (not `ENABLED`), otherwise any signed-in user can edit undeclared attributes such as `registration_status` and `requested_role`, which the approval queue reads. Check with `GET /admin/realms/<realm>/users/profile`; fix with `PUT` of the same document. The repo's realm import already has this (`TestRealmKeepsTenantAttributesAdminOnly`); a realm created before 2026-11-03 may still say `ENABLED`.
3. **Stripe**: enable Connect on the platform Stripe account. Add a second webhook endpoint for *events on connected accounts* pointing at `/api/v1/webhooks/stripe`, and set its signing secret as `STRIPE_CONNECT_WEBHOOK_SECRET`. Subscribe it to `payment_intent.*`, `charge.refunded` and `account.updated`.
4. **Database role**: the API's `DATABASE_URL` user must **not** be a superuser and must not have `BYPASSRLS`, and should not be the role that runs migrations. Row-level security (currently on the six `volunteer_*` tables, tracker 4.4) is silently skipped for such roles, so a superuser connection gives no database-level isolation at all. Verify with `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user;` (both must be false).

## 2. Creating a club

Operator console `/platform` → *Add a club* (or `POST /api/v1/platform/orgs`). Provide the club name, a web handle (3–40 chars: a–z, 0–9, hyphen), and the first administrator.

What happens: the org is registered (`provisioning`), its settings are created, the administrator's Keycloak user is created in that org with the `admin` role and gets a verification and a password-setup email, then the org becomes `active`.

If it fails halfway the org stays `provisioning`. **Submit the same details again**; the run resumes without creating a second org. A slug already used by another club (as its registry handle *or* its public website handle) is refused, and so is an administrator email that already belongs to another club (one email, one club).

After creation the club's admin should: connect Stripe (Settings → Online payments), set the email sender (Settings), create sign-up invites, and optionally enable the public site.

## 3. Pausing and resuming

*Pause club* sets `suspended`. Within 15 seconds on every API instance (immediately on the one that handled the click) all signed-in users of that club get 403 `org_unavailable`, sign-in is refused, the public site and sign-up disappear, and scheduled jobs stop. Stripe and Postmark webhooks are still acknowledged and recorded. *Resume club* reverses it. The platform org cannot be paused.

## 3a. Supporting a club's people

Open a club from the operator console (`/platform`, click its name) for the support page, or call the routes under `/api/v1/platform/orgs/{id}`. It shows the club's handle, plan, web addresses, Stripe Connect status, people per role and a list of the club's accounts. It reads account details only (name, email, roles, enabled, email verified, MFA on), never swimmers, invoices or households.

| Club asks | Do this | Route |
| --- | --- | --- |
| "I can't log in" / forgot password | **Send password reset** | `POST .../users/{userID}/password-reset` |
| "I never got the verification email" | **Resend verification** | `POST .../users/{userID}/resend-verification` |
| "I lost my phone" | **Reset MFA** (deletes authenticator and backup codes, ends sessions; they enrol again at next sign-in) | `POST .../users/{userID}/reset-mfa` |
| "Someone left, lock their account" | **Disable account** (also ends sessions); **Enable account** reverses it | `POST .../users/{userID}/disable` / `enable` |
| "Sign them out everywhere" | **Sign out everywhere** | `POST .../users/{userID}/sign-out` |
| "Our only admin left" | **Add an administrator** (promotes an existing user of the club, or creates and invites a new one) | `POST .../admins` |
| "We renamed" | **Edit details** (display name and plan; the handle is fixed) | `PATCH .../orgs/{id}` |

Rules every action follows:
- A `reason` of 3-500 characters is mandatory. It is stored in the audit log, and the entry is written to both the platform log and the club's own log, so the club's admins can see what support did and why. `GET .../activity` lists the latest operator entries (`platform.*`) for a club.
- The target must belong to the club in the URL (otherwise 404). Operator accounts and the platform org cannot be targeted. An email that belongs to another club cannot be added (409 `admin_email_in_use`).
- Works while a club is suspended or closing (it may need to pull its export); refused once the club is deleted.
- There is no impersonation. Recovery is always done by emailing the user or resetting their factor.
- Dev: the login page's **Operator** button signs in as the stub operator; set `PLATFORM_ORG_ID=00000000-0000-0000-0000-00000000a0ff` first.

## 4. Closing a club and deleting its data

1. **Start closing** (`offboarding`): access stops now; the cooling-off clock starts (`PLATFORM_OFFBOARDING_COOLING_OFF`, default 14 days). *Keep the club* cancels it (the org returns to `suspended`).
2. **Create data export** at any time (`POST /platform/orgs/{id}/export`): a zip with one NDJSON file per table plus a manifest of stored files with 7-day links. Credentials are never included (sessions, MFA factors, feed tokens, invite hashes, integration secrets, payment account link, raw webhook payloads). Give the club its export before deleting.
3. **Delete permanently** (`POST /platform/orgs/{id}/purge`) needs: status `offboarding`, the cooling-off period passed, and the club's handle typed exactly. It deletes the club's Keycloak users, stored files and database rows. **Kept on purpose:** the audit trail and the `orgs` row (as a tombstone: status `deleted`, slug released, id never reused). **Not touched:** the club's own Stripe account, which belongs to the club.

Everything above is audited as `platform.org.*` in the platform org's audit log.

## 5. Recovering one club (single-org restore)

A database-wide point-in-time restore rolls back *every* club, so recover one club from its export instead:

1. Make sure the club has an export (`export` step above; keep one before risky operations). The key looks like `offboarding/{org_id}/export-<timestamp>.zip`. It sits outside the prefixes a purge deletes, so it survives the purge and remains the restore source (delete it explicitly if the data must be erased for compliance).
2. `POST /platform/orgs/{id}/restore` with `{"export_key": "..."}`. Only rows whose `org_id` is that club are inserted; rows that exist are left alone; rows of any other org are dropped; the call is safe to repeat. The org comes back **suspended**.
3. Check the data, then **resume**. People sign in again and re-enrol MFA (credentials are not in exports). Stored files are not in the archive: restore them from object-storage backups using the manifest.

Tested by `TestExportThenPurgeThenRestoreRecoversOnlyThatOrg` and `TestRestoreNeverInsertsRowsOfAnotherOrg`.

## 6. Production migration rehearsal (existing single club to multi-tenant)

Do this on a snapshot of production first and record timings. The migrations are `20260801000100`–`20260801000600`.

**Pre-flight (must all return zero rows):**

```sql
-- non-UUID org ids that cannot be converted (audit_events is append-only: if it has any, rewrite them
-- with the trigger disabled by a DBA before migrating)
SELECT 'audit_events', org_id FROM audit_events WHERE org_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' AND org_id <> 'org-demo' LIMIT 5;
-- repeat for every table whose org_id is still TEXT (see \d <table>) before 20260801000200
-- children pointing at a parent in another org (would fail 20260801000300 validation)
SELECT count(*) FROM swimmers s JOIN households h ON h.id = s.household_id WHERE h.org_id <> s.org_id;
```

**Order and locks**

| Migration | What | Locks / risk |
|---|---|---|
| 000100 | `orgs`, `org_domains`, backfill | brief; additive |
| 000200 | 22 TEXT `org_id` columns → UUID | `ACCESS EXCLUSIVE` per table while it rewrites; run in a maintenance window; aborts with a sample value if any non-UUID, non-`org-demo` value exists |
| 000300 | message child `org_id`, FK to `orgs`, composite FKs | `NOT VALID` then `VALIDATE` (no long write lock); fails loudly if a child points at another org's parent |
| 000400 / 000500 / 000600 | invites, Stripe Connect, offboarding clock | additive |

**Rollback:** 000100, 000400, 000500, 000600 and the FK part of 000300 have working `Down` sections. **000200 is irreversible** (the `org-demo` literal cannot be recovered from a UUID): take a snapshot first and treat it as the point of no return.

**After migrating:** the existing club's org is registered by the backfill. Confirm `SELECT id, slug, status FROM orgs`, set the club's handle if it has none, then set `PLATFORM_ORG_ID`, create the operators, and have the club's admin connect Stripe. Until then, set `STRIPE_CONNECT_REQUIRED=false` for the single existing club so its online payments keep using the platform account; payments taken that way keep refunding correctly because each payment remembers its account.

## 7. If a tenant-isolation bug is suspected

1. Pause the affected club(s) if live data may be exposed (section 3). Pausing blocks API access immediately.
2. `make test-tenancy DATABASE_URL=…` against a snapshot reproduces the isolation checks; the cross-org matrix test (`TestOrgBCannotReachOrgAThroughAnyRoute`) walks every route.
3. Check the audit log for the actor (`/api/v1/audit-events`; operators see the platform org's log).
4. File it as P0 per `docs/decisions/2026-03-15-p0-severity-rubric.md`.

## 8. Known limits (read before promising anything to a club)

- **Row-level security covers every tenant table (81, including the change-counter table), with two precautions.** The application's database role must not be a superuser or have BYPASSRLS, or RLS does nothing. A new tenant table fails `TestEveryTenantTableHasRowLevelSecurity` until its repository binds the org (`internal/tenantdb`) and a migration enables RLS. A handful of lookups happen before a club is known and bind a credential instead (session token hash, calendar feed token, public slug, verified domain, Stripe account, Keycloak user, webhook event key); each is a narrow SELECT/UPDATE policy listed in the migrations from `20260801000900` on.
- **Caches load per club on demand and are bounded.** Every domain (members, groups, meets, exports, billing, comms, documents, performance) loads a club the first time it is served and evicts a club idle for `CACHE_ORG_IDLE_TTL` (30m), keeping at most `CACHE_MAX_ORGS` (200) per process, so memory is capped by that setting. Reloads read Postgres before taking the service lock (a slow club does not stall others); the Stripe payment/refund path deliberately re-reads inside its own database transaction (a payment must apply to the invoice exactly as the database holds it). Billing and performance skip the reload while the per-club change counters (`org_data_versions`, bumped by triggers at commit) show no write from another process, and still do a full reload every 5 minutes; performance checks a cached club every 5s, so another instance's writes appear within about 5s. Writes made outside the API (psql, migrations) count as foreign and force one reload. If the counters ever look wrong, dropping the `org_data_version` triggers makes every call reload again (slower, still correct). Each API/worker restart adds one `org_data_versions` row per club and domain (the writer id is per process); at startup the process folds rows of writers idle for 7 days into one `~retired` row (`prune_org_data_versions`), so the table stays bounded without a job. `splits` has no trigger because performance writes it through and never reads it back.
- **A suspension takes up to 15 seconds to reach every API instance** (status cache).
- **One email, one club.** The same person cannot be in two clubs with the same email.
- **Per-club storage and email quotas are not implemented.** Request volume is limited per club (`SECURITY_RATE_LIMIT_ORG_*`) and visible per club in `aquify_http_org_requests_total{org,class}` and `aquify_job_org_executions_total{org,status}` (bounded to 200 orgs; the rest report as `other`).
