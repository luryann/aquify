#!/usr/bin/env python3
"""Render the production Keycloak realm from the local development realm.

    render_realm.py --app-env /opt/aquify/.env.app --out /opt/aquify/keycloak/production-realm.json

The local realm (realm-aquify-local.json) stays the single source of the realm's
shape (roles, clients, mappers, user profile, themes, brute-force settings). This
script derives the production file from it so the two cannot drift, and refuses to
write anything that still looks like the development realm:

  * every user is removed except the service account for the admin client (which
    Keycloak needs for realm-management); none carries credentials
  * client secrets come from .env.app (OIDC_CLIENT_SECRET, KEYCLOAK_ADMIN_CLIENT_SECRET),
    so the realm and the application cannot disagree
  * redirect URIs, web origins and post-logout URIs are the real https origin
  * TLS is required, e-mail verification is on, SMTP is Postmark's

The first human administrator is created by hand in the Keycloak admin console
(deploy/README.md); no credential is ever stored in this file. Standard library only.
"""
import argparse
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
DEV_MARKERS = ("localhost", "127.0.0.1", "aquify.local", "dev-secret", "local-admin-secret", "password123")


def read_env(path):
    """KEY=VALUE lines of an env file (never executed)."""
    env = {}
    for raw in Path(path).read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        env[key.strip()] = value
    return env


def first_origin(env):
    origins = [o.strip() for o in env.get("API_ALLOWED_ORIGINS", "").split(",") if o.strip()]
    if not origins:
        raise SystemExit("API_ALLOWED_ORIGINS is empty in the app env file")
    return origins[0]


def render(base, env):
    realm = json.loads(json.dumps(base))  # deep copy
    origin = first_origin(env)
    secrets = {
        "aquify-id": env.get("OIDC_CLIENT_SECRET", ""),
        "aquify-admin": env.get("KEYCLOAK_ADMIN_CLIENT_SECRET", ""),
    }
    for name, value in secrets.items():
        if not value:
            raise SystemExit(f"{name}: client secret missing from the app env file")

    for client in realm["clients"]:
        cid = client["clientId"]
        if cid in secrets:
            client["secret"] = secrets[cid]
        if cid == "aquify-id":
            client["redirectUris"] = [origin + "/auth/callback"]
            client["webOrigins"] = [origin]
            attrs = client.setdefault("attributes", {})
            attrs["post.logout.redirect.uris"] = origin + "/*"

    # Seed people and passwords never ship; only the admin client's service account stays.
    realm["users"] = [
        {k: v for k, v in u.items() if k != "credentials"}
        for u in realm.get("users", [])
        if u.get("serviceAccountClientId") == "aquify-admin"
    ]

    realm["sslRequired"] = "external"
    realm["verifyEmail"] = True
    realm["registrationAllowed"] = False
    token = env.get("POSTMARK_SERVER_TOKEN", "")
    if not token:
        raise SystemExit("POSTMARK_SERVER_TOKEN missing: Keycloak needs SMTP for verification and password reset")
    realm["smtpServer"] = {
        "host": "smtp.postmarkapp.com",
        "port": "587",
        "from": env.get("POSTMARK_FROM_EMAIL", ""),
        "fromDisplayName": env.get("POSTMARK_FROM_NAME", "Aquify"),
        "auth": "true",
        "user": token,        # Postmark SMTP uses the server token as both user and password
        "password": token,
        "starttls": "true",
        "ssl": "false",
    }
    return realm


def validate(realm, env=None):
    """Return a list of problems; empty means the realm is safe to import."""
    problems = []
    text = json.dumps(realm)
    for marker in DEV_MARKERS:
        if marker in text:
            problems.append(f"contains development value '{marker}'")
    users = realm.get("users", [])
    for u in users:
        if u.get("serviceAccountClientId") != "aquify-admin":
            problems.append(f"unexpected user {u.get('username')!r}")
        if u.get("credentials"):
            problems.append(f"user {u.get('username')!r} carries credentials")
    if realm.get("sslRequired") != "external":
        problems.append("sslRequired must be 'external'")
    if realm.get("registrationAllowed"):
        problems.append("self-registration must be off")
    if not realm.get("verifyEmail"):
        problems.append("verifyEmail must be on")
    if not realm.get("bruteForceProtected"):
        problems.append("brute-force protection must be on")
    for c in realm.get("clients", []):
        if c["clientId"] in ("aquify-id", "aquify-admin"):
            secret = c.get("secret", "")
            if len(secret) < 16 or not re.fullmatch(r"[A-Za-z0-9_-]+", secret):
                problems.append(f"client {c['clientId']}: secret missing or weak")
        for uri in c.get("redirectUris", []) + c.get("webOrigins", []):
            if not uri.startswith("https://"):
                problems.append(f"client {c['clientId']}: non-https URI {uri}")
    if not realm.get("smtpServer", {}).get("host"):
        problems.append("smtpServer is not configured")
    if env is not None:
        by = {c["clientId"]: c.get("secret") for c in realm.get("clients", [])}
        if by.get("aquify-id") != env.get("OIDC_CLIENT_SECRET"):
            problems.append("aquify-id secret differs from OIDC_CLIENT_SECRET")
        if by.get("aquify-admin") != env.get("KEYCLOAK_ADMIN_CLIENT_SECRET"):
            problems.append("aquify-admin secret differs from KEYCLOAK_ADMIN_CLIENT_SECRET")
    return problems


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default=str(HERE / "realm-aquify-local.json"))
    ap.add_argument("--app-env", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--check-only", action="store_true", help="validate an existing --out file instead of rendering")
    args = ap.parse_args(argv)

    env = read_env(args.app_env)
    if args.check_only:
        realm = json.loads(Path(args.out).read_text())
    else:
        realm = render(json.loads(Path(args.base).read_text()), env)
    problems = validate(realm, env)
    if problems:
        for p in problems:
            print("realm check failed:", p, file=sys.stderr)
        return 1
    if not args.check_only:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(realm, indent=2) + "\n")
        out.chmod(0o640)
        print(f"wrote {out} ({len(realm['clients'])} clients, {len(realm['users'])} user(s))")
    else:
        print("realm file passes the production checks")
    return 0


if __name__ == "__main__":
    sys.exit(main())
