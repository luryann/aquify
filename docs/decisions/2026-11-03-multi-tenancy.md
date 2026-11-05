# ADR: Multi-Tenancy (supersedes the v1 single-club tenant strategy)

- Status: **Accepted** (2026-11-03). D3, D5 decided by the product owner; D7 accepted on the owner's requirement that each club operate independently.
- Date: 2026-11-03
- Supersedes (when accepted): `2026-03-15-org-tenant-strategy.md`
- Tracker: `docs/multi-tenancy-implementation-tracker.md`

## Context
v1 shipped single-club with `org_id` on business tables. An audit on 2026-11-03 found no tenant registry, mixed `org_id` types, no database-enforced isolation, single-org identity, a client-trusted org on public registration, no provisioning path and global Stripe/Postmark credentials. See the tracker for evidence.

## Decisions
- **D1 Isolation:** shared database and schema, `org_id` on every tenant row. App-level filtering is primary; composite FKs enforce parent/child org consistency in the database. Postgres RLS is the backstop and is enabled (FORCE) on every tenant table (tracker 4.4, closed); a guard test fails a new tenant table without it.
- **D2 Tenant key:** `orgs.id UUID`; slug is a separate unique public handle.
- **D3 Identity (decided):** one person belongs to exactly one club; `users.org_id` is the membership. No memberships table or org switching. The same email cannot join two clubs.
- **D4 Org derivation:** from the session, server-side; `X-Org-Id` is only an assertion. The session org is immutable.
- **D5 Keycloak (decided):** single realm; the `org_id` claim must match `users.org_id` or the session is rejected.
- **D6 Public tenant resolution:** slug or verified custom domain resolved server-side. No client-supplied `org_id` in public flows (landed for registration, 2026-11-03).
- **D7 Payments:** Stripe Connect, one connected account per org (Standard). Accepted: the owner requires each club to operate independently.
- **D8 Email:** per-org sender identity on a shared platform Postmark account initially; per-org suppression.
- **D9 Caches:** every cache-resident domain loads a club on demand and nothing loads eagerly at startup; idle clubs are evicted (`CACHE_ORG_IDLE_TTL`, `CACHE_MAX_ORGS`); reloads run without holding the service lock; billing and performance skip reloads while per-club change counters show no write by another process (tracker 4.3).
- **D10 Platform operator:** separate `platform_admin` plane, MFA-required, audited, no ambient data access.

## Consequences
Schema migrations are phased (registry, type unification, FKs, RLS). Single-club deployments keep working throughout. Each phase has exit checks in the tracker.
