# DARE Platform — Domain and DNS Plan

## Overview

The DARE platform uses `dare.club` as its primary domain, fronted entirely by Cloudflare (proxied). All traffic terminates TLS at Cloudflare and is re-encrypted to the origin VPS using a Cloudflare Origin Certificate installed on the VPS nginx instance.

---

## Domain Structure

| Subdomain | Purpose | Origin Target |
|---|---|---|
| `dare.club` | Root — 301 redirect to `www.dare.club` | Cloudflare redirect rule (no origin hit) |
| `www.dare.club` | Marketing / landing page (static) | VPS nginx, port 80 (internal) |
| `admin.dare.club` | Next.js admin portal | VPS nginx → container port 3000 |
| `api.dare.club` | Go API backend | VPS nginx → container port 8080 |
| `status.dare.club` | Uptime Kuma status page | VPS nginx → container port 3001 |
| `staging-admin.dare.club` | Staging admin portal | VPS nginx → container port 3100 |
| `staging-api.dare.club` | Staging API backend | VPS nginx → container port 8180 |

---

## DNS Records

All records use Cloudflare proxy (orange cloud). Replace `203.0.113.10` with the actual VPS public IP.

```
# Production
dare.club           A       203.0.113.10    Proxied
www.dare.club       A       203.0.113.10    Proxied
admin.dare.club     A       203.0.113.10    Proxied
api.dare.club       A       203.0.113.10    Proxied
status.dare.club    A       203.0.113.10    Proxied

# Staging
staging-admin.dare.club    A    203.0.113.10    Proxied
staging-api.dare.club      A    203.0.113.10    Proxied

# Mail (if sending from dare.club domain — adjust to Postmark values)
# Postmark will provide SPF, DKIM, and DMARC values
# Example:
pm-bounces.dare.club       CNAME   pm.mtasv.net    DNS-only (not proxied)
```

> **Note:** MX and mail-related records must NOT be proxied through Cloudflare — set these to DNS-only.

---

## Cloudflare SSL/TLS Configuration

### Mode
Set **SSL/TLS encryption mode** to **Full (strict)** in the Cloudflare dashboard.

- Cloudflare terminates the public TLS connection (browser → Cloudflare).
- Cloudflare re-encrypts the connection to the origin VPS (Cloudflare → VPS).
- The origin VPS must present a valid certificate — use a Cloudflare Origin Certificate.

### Origin Certificate

Generate a Cloudflare-issued origin certificate covering `dare.club` and `*.dare.club` (15-year validity is fine for a self-managed VPS).

Install the certificate on the VPS:

```
/etc/ssl/certs/dare-club-cloudflare-origin.pem    # certificate
/etc/ssl/private/dare-club-cloudflare-origin.key  # private key
```

Reference these paths in the nginx `ssl_certificate` and `ssl_certificate_key` directives. Because the Cloudflare Origin CA is only trusted by Cloudflare's edge, do NOT rely on it for non-proxied traffic.

### HSTS

Enable via **SSL/TLS → Edge Certificates → HTTP Strict Transport Security (HSTS)**:

```
max-age=31536000
includeSubDomains: yes
preload: yes
```

After enabling preload, submit `dare.club` to https://hstspreload.org once DNS is stable.

### Always Use HTTPS

Enable **Always Use HTTPS** under Edge Certificates. This forces any HTTP request to redirect to HTTPS at the Cloudflare edge before it ever reaches the origin.

### Minimum TLS Version

Set **Minimum TLS Version** to **TLS 1.2** under SSL/TLS → Edge Certificates. TLS 1.0 and 1.1 should be disabled.

### Cipher Suites

Leave at Cloudflare's managed defaults (which follow Mozilla Modern compatibility). Do not manually restrict ciphers unless a compliance requirement demands it.

---

## nginx Origin Configuration

Each subdomain has a corresponding nginx server block. The VPS only listens on port 443 (with the Cloudflare origin cert) and restricts traffic to Cloudflare IP ranges.

### Restrict Access to Cloudflare IPs

Add to the global nginx config (`/etc/nginx/conf.d/cloudflare-ips.conf`):

```nginx
# Cloudflare IPv4 ranges — regenerate from https://www.cloudflare.com/ips-v4
geo $cloudflare_ip {
    default 0;
    103.21.244.0/22    1;
    103.22.200.0/22    1;
    103.31.4.0/22      1;
    104.16.0.0/13      1;
    104.24.0.0/14      1;
    108.162.192.0/18   1;
    131.0.72.0/22      1;
    141.101.64.0/18    1;
    162.158.0.0/15     1;
    172.64.0.0/13      1;
    173.245.48.0/20    1;
    188.114.96.0/20    1;
    190.93.240.0/20    1;
    197.234.240.0/22   1;
    198.41.128.0/17    1;
}
```

In each server block, deny non-Cloudflare traffic:

```nginx
if ($cloudflare_ip = 0) {
    return 403;
}
```

### Example: api.dare.club

```nginx
server {
    listen 443 ssl http2;
    server_name api.dare.club;

    ssl_certificate     /etc/ssl/certs/dare-club-cloudflare-origin.pem;
    ssl_certificate_key /etc/ssl/private/dare-club-cloudflare-origin.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    # Restore real client IP from Cloudflare header
    real_ip_header     CF-Connecting-IP;
    set_real_ip_from   0.0.0.0/0;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

Apply the same pattern for `admin.dare.club` (→ 3000), `status.dare.club` (→ 3001), staging variants, etc.

---

## Root Domain Redirect

Configure a Cloudflare **Redirect Rule** (Rules → Redirect Rules):

```
When: Hostname equals dare.club
Then: Dynamic redirect → https://www.dare.club${uri}  (301)
```

This avoids a DNS round-trip and works without an origin A record for the apex.

---

## Staging Environment DNS

Staging subdomains resolve to the same VPS IP but are routed to different container ports by nginx. See `staging-environment-guide.md` for full staging configuration.

```
staging-api.dare.club   → VPS port 8180
staging-admin.dare.club → VPS port 3100
```

Staging records should be proxied through Cloudflare (orange cloud) so they also benefit from WAF protection.

---

## Certificate Renewal

| Certificate type | Renewal method | Renewal cadence |
|---|---|---|
| Cloudflare Origin Certificate | Manual regeneration in CF dashboard | Every 15 years (or shorter validity if preferred) |
| Let's Encrypt (fallback / non-CF-proxied) | `certbot renew` via systemd timer | Every 60 days automatically |

For the current architecture (full Cloudflare proxy), the Cloudflare Origin Certificate is sufficient. Let's Encrypt is only needed if you ever expose the VPS directly (e.g., for Uptime Kuma internal checks from an external prober).

Certbot setup if needed:

```bash
certbot certonly --nginx -d dare.club -d '*.dare.club' \
    --dns-cloudflare --dns-cloudflare-credentials ~/.secrets/cloudflare.ini
```

---

## Checklist: Going Live

- [ ] Register `dare.club` and delegate nameservers to Cloudflare
- [ ] Set all A records as proxied (orange cloud)
- [ ] Generate Cloudflare Origin Certificate for `*.dare.club`
- [ ] Install origin cert on VPS nginx
- [ ] Set SSL/TLS mode to Full (strict)
- [ ] Enable Always Use HTTPS
- [ ] Enable HSTS with includeSubDomains + preload
- [ ] Set minimum TLS version to 1.2
- [ ] Configure Cloudflare IP restriction on nginx
- [ ] Configure root domain redirect rule in Cloudflare
- [ ] Verify `api.dare.club/healthz` returns 200
- [ ] Verify `admin.dare.club` loads correctly
- [ ] Submit `dare.club` to HSTS preload list
