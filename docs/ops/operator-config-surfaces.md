# DARE Platform — Operator Configuration Surfaces

This document describes which configuration settings can be changed and by whom, without requiring infrastructure access or a service restart.

---

## Tier 1 — Safe to Change In-App (Admin Panel Runtime Settings)

These settings are stored in the database and take effect immediately without a deployment or service restart. They are accessible to users with the `owner` or `admin` role via the Admin Panel → System → Runtime Settings page.

### Email Notification Templates

| Setting | Description | Notes |
|---|---|---|
| Welcome email body | HTML template for new household registration | Supports `{{swimmer_name}}`, `{{org_name}}` tokens |
| Meet commitment invite template | Body for commitment invitation emails | |
| Invoice issued template | Body for invoice created notification | |
| Payment receipt template | Body for successful payment confirmation | |
| Registration confirmation template | Body for completed registration | |

Changes take effect for all emails sent after the save. In-flight queued messages use the template version that was active when they were enqueued.

### Announcement Content

| Setting | Description |
|---|---|
| Active announcements | Create, edit, publish, archive announcements via the Announcements page — no restart required |
| Email campaign drafts | Draft and schedule campaigns; scheduling writes a queued job at save time |
| Suppression list management | Add/remove email suppression entries in real time |

### Meet Registration Deadlines

| Setting | Description |
|---|---|
| Default commitment window (days before meet) | How many days before meet date the commitment window opens |
| Default entry deadline (hours before meet) | Hours before meet start when entries lock |
| Grace period for late entries (minutes) | Coached override window after lock |

These defaults populate new meet forms but can be overridden per-meet.

### Time Standard Thresholds

| Setting | Description |
|---|---|
| Time standard sets | Create and manage named time standard sets (AAAA/AAA/AA/A/BB/B) per age group and stroke |
| Qualifying time update frequency | How often best-time qualification checks re-run (nightly by default) |

Time standards take effect on the next eligibility check job run. They do not retroactively modify existing entry approvals.

### Invoice and Billing Settings

| Setting | Description |
|---|---|
| Invoice grace period (days) | Days after due date before a late fee is applied |
| Late fee amount (dollars) | Flat fee added after grace period |
| Late fee percentage | Percentage of unpaid balance added after grace period |
| Invoice notes template | Default footer text on generated invoices |
| Autopay reminder days | How many days before due date the autopay reminder email fires |

These take effect for invoices generated after the change. Existing invoices are not retroactively updated.

### Rate Limit Overrides (Auth Window)

The `auth_rate_limit_window` runtime setting allows the on-call engineer to temporarily widen the auth rate limit window without a restart — for example, during a large team import that generates bulk account creation requests.

> **Warning:** Widening the auth rate limit reduces protection against credential stuffing. Log a reason and revert promptly. The Cloudflare WAF rate limit (20 req/min) operates independently and is unaffected by this in-app override.

---

## Tier 2 — Require Environment Variable Change + Restart

These settings are controlled by environment variables in `/opt/dare/.env.production` (or `.env.staging`). A change requires:

1. SSH to VPS, update the `.env` file.
2. Restart the affected container(s): `docker compose restart dare-api dare-worker` (or `dare-admin` for frontend vars).
3. Verify health: `curl https://api.dare.club/healthz`.

### Auth and Session

| Variable | Description |
|---|---|
| `AUTH_ENABLE_DEV_STUB_TOKENS` | Must be `false` in production. Setting `true` re-enables dev bearer tokens — never do this in production |
| `AUTH_ALLOW_DEV_SESSION_ISSUE` | Must be `false` in production. Same risk as above |
| `OIDC_AUTHORIZE_URL` | Keycloak authorization endpoint — change if realm URL changes |
| `OIDC_TOKEN_URL` | Keycloak token endpoint |
| `OIDC_JWKS_URL` | Keycloak JWKS endpoint |
| `OIDC_CLIENT_ID` | OIDC client identifier |
| `OIDC_CLIENT_SECRET` | OIDC client secret — rotate via Keycloak admin, then update here |
| `OIDC_STATE_SIGNING_KEY` | 32-byte hex key for signing OIDC state params — rotate with care (invalidates in-flight logins) |
| `OIDC_EXPECTED_ISSUER` | Expected issuer in JWT — must match Keycloak realm URL |

### Object Storage

| Variable | Description |
|---|---|
| `S3_ENDPOINT` | MinIO/S3 endpoint URL |
| `S3_ACCESS_KEY_ID` | Access key |
| `S3_SECRET_ACCESS_KEY` | Secret key |
| `S3_REGION` | Storage region |
| `MINIO_BUCKET` | Primary object storage bucket name |
| `STORAGE_BACKUP_BUCKET` | Backup artifact bucket name |

### Email (Postmark)

| Variable | Description |
|---|---|
| `POSTMARK_SERVER_TOKEN` | Postmark API token — rotate via Postmark dashboard |
| `POSTMARK_FROM_EMAIL` | From address for transactional email |
| `POSTMARK_WEBHOOK_AUTH_HEADER_NAME` | Webhook auth header name |
| `POSTMARK_WEBHOOK_AUTH_HEADER_VALUE` | Webhook auth header value — rotate and update Postmark dashboard |
| `POSTMARK_WEBHOOK_BASIC_USER` | Webhook HTTP basic auth username (alternative to header auth) |
| `POSTMARK_WEBHOOK_BASIC_PASS` | Webhook HTTP basic auth password |

### Billing (Stripe)

| Variable | Description |
|---|---|
| `STRIPE_SECRET_KEY` | Stripe secret key — never rotate mid-processing; coordinate with billing team |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret — update after rotating in Stripe dashboard |

### Rate Limiting (Global)

| Variable | Description |
|---|---|
| `SECURITY_RATE_LIMIT_GLOBAL_REQUESTS` | Global rate limit request count |
| `SECURITY_RATE_LIMIT_GLOBAL_WINDOW` | Global rate limit window duration (e.g. `1m`) |
| `SECURITY_RATE_LIMIT_AUTH_REQUESTS` | Auth endpoint rate limit count |
| `SECURITY_RATE_LIMIT_AUTH_WINDOW` | Auth endpoint rate limit window |

### Upload Limits

| Variable | Description |
|---|---|
| `SECURITY_UPLOAD_MAX_BYTES` | Maximum import file size in bytes |
| `SECURITY_UPLOAD_ALLOWED_MIMES` | Comma-separated allowed MIME types for uploads |
| `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK` | Set `true` to require malware scan before import finalization |

### Observability

| Variable | Description |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP trace collector endpoint — set blank to disable tracing |
| `OTEL_TRACES_SAMPLER_ARG` | Trace sampling rate (0.0–1.0) |
| `LOG_LEVEL` | Logging verbosity (`debug`, `info`, `warn`, `error`) |

### Feature Flags

| Variable | Description |
|---|---|
| `SECURITY_IMPERSONATION_ENABLED` | Enable admin impersonation — `false` by default; requires owner MFA when enabled |
| `TURNSTILE_SECRET_KEY` | Cloudflare Turnstile server-side key for login bot protection |

---

## Tier 3 — Require Infrastructure Access

These changes require direct VPS, Cloudflare, or database access. They cannot be made via the admin panel or by updating environment variables alone.

### TLS Certificates

| Task | Where |
|---|---|
| Regenerate Cloudflare Origin Certificate | Cloudflare dashboard → SSL/TLS → Origin Server |
| Install updated certificate on VPS | SSH to VPS, update `/etc/ssl/certs/` and `/etc/ssl/private/`, then `nginx -s reload` |
| Let's Encrypt renewal | `certbot renew` runs via systemd timer; check `systemctl status certbot.timer` |

### Cloudflare WAF Rules

| Task | Where |
|---|---|
| Add/modify custom WAF rules | Cloudflare dashboard → Security → WAF → Custom Rules |
| Adjust rate limit thresholds at edge | Cloudflare dashboard → Security → WAF → Rate Limiting |
| Update Bot Fight Mode settings | Cloudflare dashboard → Security → Bots |
| Change Security Level per subdomain | Cloudflare dashboard → Security → Settings |

Changes here take effect at the Cloudflare edge within seconds but require Cloudflare account access (admin or security role on the CF account).

### Database

| Task | Who / Where |
|---|---|
| Run schema migrations (`goose up/down`) | On-call engineer via SSH + `make db-up` |
| Adjust PostgreSQL `max_connections` | Edit `postgresql.conf` on VPS + restart Postgres container |
| Resize connection pool | Update `DB_POOL_MAX_CONNS` env var + restart dare-api |
| Add database indexes | Write migration file + deploy |
| Run manual data repairs | DBA or engineering lead via `psql` — requires audit log entry |

### VPS Resource Allocation

| Task | Who / Where |
|---|---|
| Resize VPS (CPU/RAM) | VPS provider control panel (requires provider account access) |
| Mount additional block storage | VPS provider + SSH to mount and update fstab |
| Update Docker resource limits | Edit `docker-compose.yml` resource limits + `docker compose up -d` |
| Rotate SSH deploy keys | Update VPS `~/.ssh/authorized_keys` + update CI secrets |

### Backup Schedule

| Task | Who / Where |
|---|---|
| Change backup frequency | Edit `/etc/systemd/system/dare-backup.timer` on VPS + `systemctl daemon-reload` |
| Change backup retention policy | Edit `deploy/scripts/backup.sh` `RETENTION_*` variables + re-deploy script |
| Change offsite backup target | Update `RCLONE_REMOTE` in backup script + re-configure rclone |
| Test restore | Follow `restore-drill-checklist.md` — requires engineering lead sign-off |

---

## Configuration Change Log

Any Tier 2 or Tier 3 change should be logged in the team Slack channel `#dare-ops` with:

- Who made the change
- What was changed
- Why (brief rationale)
- Any rollback plan if the change causes issues

For Tier 3 changes to production infrastructure (WAF rules, database, VPS), open a brief change record in the team's issue tracker before making the change.
