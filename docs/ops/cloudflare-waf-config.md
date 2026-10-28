# DARE Platform — Cloudflare WAF and Security Configuration

## Overview

All traffic to `dare.club` and its subdomains passes through Cloudflare. This document defines the WAF rules, rate limiting, caching, and security settings that form the platform's edge defense layer. These settings complement—but do not replace—the in-application rate limiter and security middleware described in `CLAUDE.md`.

---

## WAF Managed Rules

### OWASP Core Rule Set

Enable the **Cloudflare OWASP Core Ruleset** under Security → WAF → Managed Rules.

| Setting | Value |
|---|---|
| Status | Enabled |
| Paranoia Level | PL1 (Level 1) for API routes — less false positives |
| Score threshold | 60 (block at High sensitivity) |
| Action | Block |

> PL1 is the right starting point for a JSON API. If false positives appear on file imports or large POST bodies, add a WAF skip rule for `/api/v1/imports/*` targeting specific rule IDs rather than raising the global threshold.

### Cloudflare Managed Ruleset

Enable the **Cloudflare Managed Ruleset** under WAF → Managed Rules.

Ensure the following rule groups are set to **Block** (not just Log):

| Rule Group | Action |
|---|---|
| Log4j / Log4Shell (CVE-2021-44228) | Block |
| Spring4Shell (CVE-2022-22963) | Block |
| Shellshock | Block |
| PHP exploits | Block |
| WordPress exploits | N/A — skip (not used) |
| Exposed credentials | Block |

### Exposed Credentials Check

Enable **Credential Stuffing Protection** under Security → WAF → Managed Rules → Exposed Credentials Check. Point it at `POST /api/v1/auth/sessions` and `POST /api/v1/auth/sessions/issue`.

---

## Custom WAF Rules

Navigate to **Security → WAF → Custom Rules**. Create the rules below in the listed priority order (lower number = evaluated first).

### Rule 1 — Block Empty User-Agent on API

```
Expression:
  (http.host eq "api.dare.club") and (http.user_agent eq "")

Action: Block
Priority: 1
Description: Reject API requests with no User-Agent header — characteristic of automated scanners
```

### Rule 2 — Block Empty User-Agent on Staging API

```
Expression:
  (http.host eq "staging-api.dare.club") and (http.user_agent eq "")

Action: Block
Priority: 2
Description: Same empty-UA block for staging API
```

### Rule 3 — Admin Routes: Challenge Non-Browser Clients

```
Expression:
  (http.host eq "admin.dare.club") and
  not (http.user_agent contains "Mozilla") and
  not (cf.client.bot_score lt 30)

Action: Managed Challenge (Turnstile)
Priority: 10
Description: Require browser challenge for non-browser clients hitting the admin portal
```

### Rule 4 — Block Direct VPS IP Access

```
Expression:
  not (http.host in {"dare.club" "www.dare.club" "admin.dare.club" "api.dare.club"
                      "status.dare.club" "staging-admin.dare.club" "staging-api.dare.club"})
  and ip.src ne 0.0.0.0/0

Action: Block
Priority: 20
Description: Block requests that do not match known hostnames (host header spoofing / direct IP scans)
```

### Rule 5 — Block Suspicious Path Patterns

```
Expression:
  (http.request.uri.path contains "wp-admin") or
  (http.request.uri.path contains "wp-login") or
  (http.request.uri.path contains ".php") or
  (http.request.uri.path contains "/.env") or
  (http.request.uri.path contains "/etc/passwd") or
  (http.request.uri.path contains "xmlrpc")

Action: Block
Priority: 30
Description: Block common scanner probes for WordPress, PHP, and system file paths
```

---

## Rate Limiting Rules

Navigate to **Security → WAF → Rate Limiting Rules**.

> **Important:** These edge rate limits mirror the in-application rate limiter. Both layers should be consistent. The application-level rate limiter (in-memory) is the authoritative source; these Cloudflare rules are a defense-in-depth first line.

### Rate Limit 1 — Auth Endpoints

```
Name: Auth Rate Limit
Expression: (http.request.uri.path wildcard "/api/v1/auth/*")
Characteristics: IP address
Requests per period: 20
Period: 1 minute
Action: Block (429 response)
Description: Matches APP rate limit config — SECURITY_RATE_LIMIT_AUTH_REQUESTS=20/1m
```

### Rate Limit 2 — Import Uploads

```
Name: Import Upload Rate Limit
Expression: (http.request.uri.path wildcard "/api/v1/orgs/*/imports*") and (http.request.method eq "POST")
Characteristics: IP address
Requests per period: 10
Period: 1 minute
Action: Block (429 response)
Description: Limit import job submission to prevent queue flooding
```

### Rate Limit 3 — Global API Limit

```
Name: Global API Rate Limit
Expression: (http.host eq "api.dare.club") or (http.host eq "staging-api.dare.club")
Characteristics: IP address
Requests per period: 1200
Period: 1 minute
Action: Block (429 response)
Description: Matches APP global rate limit — SECURITY_RATE_LIMIT_GLOBAL_REQUESTS=1200/1m (sized for a club's shared pool/office NAT, where every coach and parent device presents one IP and each portal page issues ~10 API reads)
```

---

## Bot Fight Mode

| Surface | Setting |
|---|---|
| `www.dare.club` | Super Bot Fight Mode: On — block definitely automated traffic |
| `admin.dare.club` | Super Bot Fight Mode: On |
| `api.dare.club` | Bot Fight Mode: Off — API clients may be non-browser scripts; rely on custom rules instead |
| `status.dare.club` | Super Bot Fight Mode: On |

Configure under **Security → Bots**.

---

## Page Rules / Cache Rules

Cloudflare Page Rules are deprecated in favour of **Cache Rules** (Rules → Cache Rules). Define the following:

### Cache Rule 1 — Next.js Static Assets (1-year cache)

```
Name: Admin Static Assets Cache
Expression: (http.host eq "admin.dare.club") and
            (http.request.uri.path wildcard "/_next/static/*")
Cache Eligibility: Eligible for cache
Edge TTL: Respect origin + override to 31536000 seconds
Browser TTL: 31536000 seconds
Cache Key: Default
```

### Cache Rule 2 — No Cache for API Responses

```
Name: API No Cache
Expression: (http.host eq "api.dare.club") or (http.host eq "staging-api.dare.club")
Cache Eligibility: Bypass cache
```

### Cache Rule 3 — No Cache for Admin HTML

```
Name: Admin HTML No Cache
Expression: (http.host eq "admin.dare.club") and
            not (http.request.uri.path wildcard "/_next/static/*") and
            not (http.request.uri.path wildcard "/_next/image*")
Cache Eligibility: Bypass cache
Description: Next.js app shell and data fetching should not be edge-cached
```

---

## Transform Rules — Force HTTPS Redirect

Cloudflare's **Always Use HTTPS** setting (under SSL/TLS → Edge Certificates) handles this globally. No additional Transform Rule is needed. Confirm the toggle is enabled.

For any subdomain that should NOT be forced to HTTPS (e.g., internal health-check endpoints — unlikely), you would add a Page Exception. For DARE, leave Always Use HTTPS on globally.

---

## Security Level

| Zone / Subdomain | Security Level |
|---|---|
| `api.dare.club` | Medium |
| `admin.dare.club` | High |
| `www.dare.club` | Medium |
| `status.dare.club` | Medium |
| Staging subdomains | Medium |

Configure via **Security → Settings → Security Level** per zone, or use a Configuration Rule scoped by hostname expression if the entire zone shares a single setting.

---

## DDoS Protection

Enable **DDoS Protection** (HTTP DDoS Attack Protection) under **Security → DDoS**:

```
Ruleset: Cloudflare HTTP DDoS Attack Protection
Sensitivity: High (default)
Action: Block
```

Enable **Network-layer DDoS Protection** (L3/L4) under Security → DDoS → Network-layer DDoS:

```
Sensitivity: High
Action: Block
```

Both are enabled by default on Pro and above plans — verify they are active.

---

## Cloudflare Turnstile (Login Page)

Turnstile provides a lightweight, privacy-preserving bot challenge for the login form. It is optional but recommended.

1. Create a Turnstile widget in the Cloudflare dashboard (Turnstile → Add Widget).
   - Hostname: `admin.dare.club`
   - Widget Mode: Managed
   - Note the **Site Key** and **Secret Key**.
2. In the admin Next.js app, add `@cloudflare/turnstile-web` to the login form.
3. In the Go API, verify the Turnstile token server-side at `POST /api/v1/auth/sessions` before issuing the session:

```go
// Verify Turnstile token
resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
    "secret":   {os.Getenv("TURNSTILE_SECRET_KEY")},
    "response": {turnstileToken},
    "remoteip": {clientIP},
})
```

4. Set `TURNSTILE_SECRET_KEY` and `NEXT_PUBLIC_TURNSTILE_SITE_KEY` in respective `.env` files.
5. Gate this behind `APP_ENV=production` to avoid friction in dev/staging.

---

## Cloudflare Access (Optional Hardening for Admin)

For an additional layer of identity protection on `admin.dare.club`, consider enabling **Cloudflare Access** (Zero Trust → Access → Applications):

- Require Cloudflare Access authentication before the request reaches the origin.
- Allow access only to users with `@dare.club` email addresses (or a specific identity provider group).
- This adds a pre-auth gate before the OIDC/Keycloak login.

This is optional and adds operational complexity. It is most valuable during early launch when the attack surface is large relative to the user base.

---

## Monitoring and Alerting

| Alert type | Where to configure |
|---|---|
| WAF rule triggered (> 100/min) | Cloudflare → Notifications → Security Events |
| DDoS attack detected | Cloudflare → Notifications → DDoS Attack |
| Origin health degraded | Cloudflare → Notifications → Origin Error Rate |
| Rate limit threshold reached | Cloudflare → Notifications → Advanced Rate Limiting |

Route all Cloudflare notifications to the same Slack channel used for ops alerts (`#dare-alerts`).

---

## Ongoing Maintenance

- **Quarterly:** Review Cloudflare managed ruleset changelog for new rule additions that may cause false positives against the API.
- **On any API schema change:** Audit WAF expression patterns (empty UA rule, path-based rules) for unintended matches against new routes.
- **On any new dependency CVE:** Check if Cloudflare managed rules cover it; if not, add a custom rule targeting the exploit signature until upstream patches ship.
- **Annually:** Rotate Turnstile secret key and update `TURNSTILE_SECRET_KEY` environment variable.
