# Production-readiness tracker

Merged, deduplicated list from two independent assessments (Claude Code, Codex), each
re-verified against the tree before work started. Status vocabulary:

- **fixed + tested**: a test that was actually executed here covers the change.
- **fixed, logic-tested only**: covered by a sandbox test with stubbed Docker/gpg/mc/rclone/curl.
  This proves script logic, not that the real tool accepts the commands.
- **changed, untested**: edited, but nothing executed here exercises it.
- **open**: not done. **unverified**: a hypothesis nobody has checked.

Gate columns: **S** = needed for a private staging install with synthetic data;
**P** = needed before real customer data or live payments.

Environment used for verification: macOS arm64, Go 1.26 from `.tools/`, and **Docker 29.9 + Compose 5.7 +
buildx via Colima (Apple Virtualization VM, 4 CPU / 8 GB), installed under `.tools/docker`** (`. .tools/docker/env.sh`;
the VM must be started with `colima start --vm-type vz`). Quirks found while setting it up: the Lima socket path limit
(104 chars) forces `COLIMA_HOME=~/.colima-aquify`; Colima forwards the Docker socket to `~/.colima/default/docker.sock`
regardless; Colima shares only `$HOME` with the VM, so a stack staged under `/tmp` gets empty bind mounts.
Everything below that says **rehearsed locally** ran on that VM with synthetic secrets, `*.localtest.me` names and
Caddy's internal CA (`deploy/scripts/rehearse-local.sh`), **arm64 images only**. Scripts that expect a Linux host
(`backup.sh`, `restore.sh`, `promote.sh`, `rollback.sh`) ran unmodified inside an Alpine container with real
gpg, rclone, mc, python3 and the docker CLI against the VM's daemon. A real `psql` 16 (from the postgres image,
through a shim) replaced the earlier pgx stand-in for the Go suite. Not available: a public name (no ACME), amd64, real
Stripe/Postmark/Mailpit mail, a browser sign-in (the realm ships no users and the service-account token carries no org).

A snapshot of the working tree taken before any change is at `refs/backup/pre-hardening`
(tracked changes) plus an untracked-files tarball kept outside the repo.

## 1. Issue tracker

| ID | Sev | Issue | Root cause | Resolution / evidence | Gate | Status |
|----|-----|-------|------------|-----------------------|------|--------|
| T-01 | P0 | No way to apply migrations in a deployment | `main.go` handled only `healthcheck`/`worker`; image is distroless; README step ran a nonexistent command | `aquify-api migrate up\|status` (`cmd/api/migrate.go`, `internal/dbmigrate`); SQL embedded at image build from `db/migrations` (`server/Dockerfile`, `make embed-migrations`); owner DSN via `MIGRATE_DATABASE_URL`, never the app role. **Tests run:** fresh apply of all 133, idempotent re-run, 3 concurrent runners apply each exactly once, failing migration rolls back and is not recorded, empty embed is a hard error, DSN never echoed. Built binary run as `APP_ENV=production` against an empty database: 133 applied, second run 0. | S,P | fixed + tested. **Rehearsed locally:** `docker build -f server/Dockerfile .` succeeds; the built image run as `APP_ENV=production` applied 133 against an empty PG 16 (`migrate up`, then `status`). |
| T-02 | P0 | Duplicate YAML merge keys in compose | `<<: *restart` and `<<: *logging` both present on 4 services | merged into one sequence; strict yaml.v3 parse test for all compose + workflow files; reproduced the exact 4 reported errors on the old file | S,P | fixed + tested |
| T-03 | P0 | Compose `${VARS}` not loaded outside systemd | `env_file` does not feed interpolation; scripts ran `docker compose -f` bare | `scripts/lib.sh` `compose()` always passes `.env.production` + `release.env`; systemd unit does the same; `IMAGE_TAG` now required | S,P | fixed + tested. Real `docker compose config` accepts production (via `preflight.sh`, which now passes) and staging with the generated env files; `promote.sh` / `rollback.sh` ran end to end (see section 4) |
| T-04 | P0 | `.env.production` root-owned yet rewritten by promote | one file held secrets and the mutable tag | tag moved to `release.env`; secrets files are read-only to scripts | S,P | fixed + tested (promote rewrote only `release.env`; the secrets files were read-only to it) |
| T-05 | P0 | No public HTTPS ingress | nothing listened on 80/443 | `caddy` service + `deploy/caddy/Caddyfile` (app/api/auth/files hostnames, Keycloak admin/master/metrics blocked, `files.` alias for presigned URLs). Test: only caddy publishes non-loopback ports, MinIO console unpublished | S,P | fixed + tested. **Found by running it:** `caddy` waited on a healthy API and the API validates Keycloak's JWKS *through caddy* at startup, so a cold `compose up` deadlocked; the `depends_on` was removed. **Rehearsed locally:** app/api/auth/files routing, `/admin`, `/realms/master`, `/metrics`, `/health` on `auth.` all 404 (since changed to an allow-list: only `/realms/aquify/*`, `/resources/*`, `/js/*` are proxied, everything else 404; `caddy validate` passes, the full rehearsal was not re-run), `/api/v1/*` proxied by admin. **Not run:** ACME certificates on a public name. |
| T-06 | P0 | App connected as the Postgres superuser (RLS bypassed) | only the bootstrap superuser existed; no startup check | `postgres/init/roles.sql.in` + `10-bootstrap.sh`: owner / app (NOSUPERUSER NOBYPASSRLS) / backup (BYPASSRLS, read-only) / keycloak (own DB); startup refuses a superuser/BYPASSRLS role. **Tests run against real PG:** all 133 migrations run as the non-superuser owner; app role sees 0 rows unbound, only its club when bound, cannot write another club's rows, cannot CREATE/DROP; backup role reads all, cannot write; keycloak role cannot connect; script idempotent; mutation (app given BYPASSRLS) is caught. Wrapper input validation sandbox-tested. | S,P | fixed + tested. **Rehearsed locally:** the real postgres image ran `10-bootstrap.sh` on an empty volume and created the four roles with the intended `rolsuper`/`rolbypassrls` flags; migrations ran as the owner. |
| T-07 | P0 | Production Keycloak realm not in repo; local realm has seed users | host-provisioned file, empty dir if missing | `keycloak/render_realm.py` derives it from the local realm: only the service-account user, secrets from `.env.app`, https URIs, TLS required, verifyEmail, Postmark SMTP; validator rejects dev values. **Tests run:** render, refusal of bad input, rejection of 4 tampered files | S,P | fixed + tested. **Found by running real Keycloak 26.5:** the import was mounted as `production-realm.json`, which Keycloak rejects (`File name / realm name mismatch`); now mounted as `aquify-realm.json`, with a policy test. **Rehearsed locally:** the rendered realm imports, the OIDC discovery and JWKS endpoints answer through caddy, the API's startup JWKS check passes, the `aquify-admin` service account obtains a token. **Not run:** browser login, MFA enrolment, mail. First admin is a manual console step |
| T-08 | P1 | Redis could evict Asynq data | `allkeys-lru` | `noeviction`, 512 MB, password via env not argv; test forbids any eviction policy in either compose file | S,P | fixed + tested. **Rehearsed locally:** `noeviction` + AOF reported by the running Redis; a queued list survived a restart; with `maxmemory` lowered to 3 MB, writes were rejected (`OOM command not allowed`) at key 79, nothing was evicted and the queue was intact. Asynq itself was not driven |
| T-09 | P1 | Backup archive path corrupted | `log()` wrote to stdout inside `$(create_archive)` | logs to stderr. **Also found by the new test and fixed:** `record_failure` inside `$(...)` was lost in the subshell, so a failed encryption exited 0 | S,P | fixed + tested (real gpg and rclone in the rehearsal; archive path clean, failures propagate) |
| T-10 | P1 | Backup: optional encryption, unreachable DB host, mismatched var names, silent skips, no retention | host `pg_dump` against `postgres:5432`; separate naming | dump runs in the container as the backup role (verified with `pg_restore --list`); gpg mandatory (`ALLOW_UNENCRYPTED_BACKUP=1` only for rehearsal, logged loudly); plaintext snapshot deleted; every failed step fails the run; offsite retention; manifest | S,P | fixed + tested. **Rehearsed locally with real tools:** pg_dump as the backup role, mc mirror, Keycloak realm export, gpg encryption, rclone upload, plaintext removed. **Two defects found and fixed:** the Keycloak token body ended in a newline (heredoc) so Keycloak answered `unsupported_grant_type` and every run failed; offsite retention pruning exited 3 for the not-yet-created weekly/monthly folders and reported two false errors every day (stub test now mirrors both). **Not covered: Keycloak users (separate DB), Redis, env files, DB/object snapshot consistency, key escrow** |
| T-11 | P1 | Restore tolerated partial failure and live writers | grep-for-ERROR, skipped steps, only api/worker stopped | verify first; stop api/worker/admin/**keycloak**; `pg_restore --single-transaction --exit-on-error` as bootstrap superuser (RLS is forced); manifest check; objects and migrate mandatory; plaintext work dir removed | P | fixed + tested. **Rehearsed locally:** the encrypted archive was restored onto emptied Postgres and MinIO volumes (club row and an object came back, manifest matched), then again over a populated database. **Defect found and fixed:** it started only `keycloak`, not the ingress the API needs at startup, and `up` of api+admin failed on admin's "api healthy" wait; now keycloak+caddy, then api+worker, then admin after readiness. **Not run:** a second physical host |
| T-12 | P1 | Staging-built admin image promoted to production | `NEXT_PUBLIC_*` baked at build | staging workflow also builds `aquify-admin-production` (`NEXT_PUBLIC_APP_ENV=production`); production compose and workflow use it | P | changed, untested (GitHub Actions not run). The admin image itself builds (`NEXT_PUBLIC_APP_ENV=production`) and ran behind caddy |
| T-13 | P1 | DB init could fall back to in-memory | `openValidatedDBPool` returned nil on error | outside dev/test a failing pool/schema/role check ends the process; dev keeps the fallback. Test covers 7 env/failure combinations | S,P | fixed + tested |
| T-14 | P1 | Env docs/templates wrong; no production template | dev `.env.example`; unused `SESSION_*` keys documented | `deploy/env/{production,app}.env.example`, `scripts/preflight.sh` (17+4 sandbox cases), tests tying templates to compose interpolation and to variables the server reads | S,P | fixed + tested |
| T-15 | P1 | Every container received all secrets | `env_file: .env.production` on 7 services | only api/worker get `.env.app`; others use explicit `environment`; migrate service gets only the owner DSN; test enforces | P | fixed + tested (config) |
| T-16 | P1 | OIDC/Stripe/provider startup validation incomplete | partial-config checks only | preflight covers https, key format, secret strength, domain consistency, Stripe test key; **the server itself still accepts incomplete OIDC / any Stripe key** | P | fixed + tested. `ValidateProviderConfig` now also refuses, outside development, a missing or non-https OIDC endpoint, missing client secret / issuer / state key, a state key under 32 characters, and webhook secrets that are not `whsec_`; Stripe key format and Postmark were already checked. Table-driven tests in `startup_test.go`; development still starts without OIDC |
| T-17 | P1 | Trusted-proxy CIDRs undocumented | absent from deploy files | template sets it; test requires it to equal the compose network subnet | S,P | fixed + tested (config) |
| T-18 | P1 | Monitoring files not shipped; Loki driver not installed | mounts pointed outside the bundle | mounts now `./docs/observability/...`, test requires every bind-mount source to exist; README installs the plugin | S,P | fixed + tested. **Two defects found by running it:** the Loki driver runs in the host's Docker daemon, which cannot resolve `loki`, so no log ever arrived and stopping/recreating a container whose driver cannot reach Loki hung `compose up`; `loki-url` is now `http://127.0.0.1:3100/...` (the published loopback port), with a policy test. Grafana 13 refused to start because `GF_INSTALL_PLUGINS=grafana-loki-datasource` 404s (Loki is built in); removed. **Rehearsed locally:** plugin `grafana/loki-docker-driver:3.3.2-arm64`, caddy's logs queried back from Loki, Grafana provisions the Prometheus and Loki datasources, Prometheus scrapes the api and worker |
| T-19 | P1 | Weak release gate; mutable images; no deploy lock | liveness-only gate, `:latest` defaults | readiness + worker gate, deploy lock, `IMAGE_TAG` required, no pull on boot. **`minio`, `prometheus`, `grafana`, `loki` still `:latest`** | P | fixed + tested. minio, prometheus, grafana, loki, mailpit and uptime-kuma pinned (digests of the multi-arch index, readable version tags), as are postgres, redis, caddy, keycloak; `TestEveryImageIsPinned` fails on a floating tag. **Found:** `minio/minio` no longer exists on Docker Hub or Quay (see T-29), so the image is built from pinned source (`deploy/minio/Dockerfile`). Readiness + worker gate and deploy lock ran in the rehearsal |
| T-20 | P2 | Presigned URLs must be browser-reachable | URLs signed with `STORAGE_ENDPOINT` | `files.<domain>` via ingress, API reaches it through a network alias | P | fixed + tested for the ingress path: a presigned POST upload and a presigned GET through `https://files.<domain>` succeeded (Host header and signature survive caddy). **Not run:** URLs signed by the API itself (needs a signed-in session) |
| T-21 | P2 | Malware-scan flag has no implementation | config only | none | P | **deferred by owner for launch** (needs a ClamAV service, a scan state machine for uploads and a decision on quarantine; `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK` is still read by nothing) |
| T-22 | P2 | TOTP secrets stored plaintext | authn | none | P | fixed + tested. TOTP secrets are AES-256-GCM sealed with `INTEGRATIONS_ENCRYPTION_KEY` (org id as associated data, `v2:` prefix) on write; plaintext rows are still readable and are re-sealed when first loaded; a sealed row with no key is skipped, not misread. DB test covers write, restart, legacy re-seal and the no-key case. Backup codes were already stored only as hashes. |
| T-23 | P2 | No `admin/.dockerignore` | `COPY . .` copied host `node_modules`/env | added; test | S | fixed + tested |
| T-24 | P2 | Email pending-before-send ambiguity | comms | none | P | fixed + tested. **Decision (owner): duplicate over drop.** `RunCampaignSend` is at-least-once: a delivery row left `pending` by a crash is sent again on retry and reused (one record per recipient); sent/suppressed rows are never re-mailed. `TestRunCampaignSendResendsPendingButNotSent` |
| T-25 | P1 | DB-backed suite had 37 failing events | see section 3 | 31 environmental + 6 real test defects, all resolved | S,P | fixed + tested |
| T-26 | P2 | Legacy snapshot bridge needs `psql` | `runtimepersist` shells out; image has none, so it silently does nothing | none | P | **partly verified**: all seven snapshot-bridge domains (comms, groups, compliance, meets, members, performance, billing) have a Postgres repository with `NewServiceWithPool`, and the DB-backed Go suite (restart/reload round trips) passes with real `psql`. A live create-restart-read through the running API was **not** done (no signed-in session) |
| T-27 | P3 | Flaky in-memory test on macOS | `time.Now()` has microsecond resolution there; strict `After` comparison can tie | seen once in a full run, 0/300 in isolation; not changed | -- | open (pre-existing) |
| T-28 | P1 | Staging does not use the new flow | `deploy-staging.yml` and `docker-compose.staging.yml` deploy without migrations, still use one env file | only the Redis eviction fix and merge-key fix were applied to staging | S | fixed + tested for config. Staging compose now has the role split and init mount, `aquify-migrate`, `.env.staging.app` for api/worker only, explicit env for admin/minio/grafana, Keycloak on its own database with `KC_PROXY_HEADERS` (`KC_PROXY=edge` was removed in KC 26) and a correctly named realm import, required `IMAGE_TAG`; `deploy-staging.yml` runs `promote.sh`. `docker compose config` accepts it. **Not run:** a staging host deploy or the GitHub workflow; `keycloak/staging-realm.json` must be rendered on the host (it was never in the repo) |

| T-29 | P1 | MinIO is no longer distributed | `minio/minio` is gone from Docker Hub and Quay; `dl.min.io` returns 410; the upstream OSS repo is archived | `deploy/minio/Dockerfile` builds `minio` and `mc` from pinned source revisions (compose `build:`); built and run healthy in the rehearsal. **Residual risk: no upstream security fixes.** Plan a move to another S3-compatible store | P | fixed + tested (build, health, presigned I/O); **risk accepted by owner: keep MinIO for now** |
| T-30 | P2 | Production compose could not cold-start | caddy/api circular dependency (see T-05) | see T-05 | S,P | fixed + tested (rehearsed) |

## 2. Dependency order that was followed

T-02 -> T-03/T-04 (deployable compose) -> T-01 (migrate service needs both) -> T-06 (roles precede
migrations) -> T-13 (fail-closed needs the role check) -> T-14/T-15 (templates and env split) ->
T-09..T-11 (backup/restore on the new compose helpers) -> T-07/T-05 (realm, ingress). T-25 ran in parallel.

## 3. The 37 failing DB-backed test events

Reproduced on a fresh PG 16.4 with all migrations (`go test -p 1 ./...`, baseline: 37 `--- FAIL`).

| Count | Cause | Resolution |
|-------|-------|------------|
| 31 | `psql` is not installed on this machine; the harness and the legacy snapshot bridge shell out to it and fail loudly by design | Environmental. Ran them with a throwaway pgx-based stand-in for `psql` (not in the repo): all 31 pass. CI has real `psql` |
| 4 | `billing` `stripeTestPool` replayed migrations by hand and did not register the org, so the `households_org_fk` foreign key failed | helper now delegates to `testutil.IsolatedSchemaPool` |
| 1 | `performance` `TestOverridePersistsAcrossReload`: a fixture used meet id `"m"` in a UUID column, then lacked a swimmer row | fixture fixed (no meet, seeded household + swimmer) |
| 1 | `TestDBRepoTrainingWorkouts_RoundTrip` expected 1900/1500 but the code correctly converts a metres workout to yards (`ToYards`, 1900 m = 2078 yd; docs agree) | test now computes expected values with `ToYards`; production code unchanged |

Result after the changes: `go test -count=1 -p 1 ./...` with a throwaway PostgreSQL 16.15 (Docker, all 133
migrations) and a **real `psql` 16** (no stand-in): exit 0, 48 packages ok. That run predates the T-16 / T-22 /
deploycheck additions; the final run is recorded in the last section of this file.

## 4. Acceptance run

The plan asked for a disposable Linux VM with public DNS. What was run instead is a **local rehearsal** on a
Colima VM (arm64, internal CA, synthetic secrets). Each step is marked with what it did and did not cover.

| # | Step | Result |
|---|------|--------|
| 1 | Fresh volumes, exact images, templates, `preflight.sh` | **Rehearsed.** Preflight passes (including real `docker compose config`). Images were local builds tagged `rehearsal`/`relN`, arm64 |
| 2 | Roles bootstrap by the real postgres image; `aquify-migrate`; services healthy; `/health/ready` | **Rehearsed.** Roles and 133 migrations fine; api, worker, admin, keycloak, caddy, redis, minio healthy. `/health/ready` returns **503** only because Stripe and Postmark reject the synthetic keys (their checks are live calls, which also proves they work); database, keycloak, queue and storage report ready |
| 3 | Render and import the realm; synthetic admin; sign in, MFA, sign out | **Partly.** Render + import + JWKS + service-account token. **Not run:** admin user, browser sign-in, MFA (no user can be created without the Keycloak console and an org) |
| 4 | Synthetic club and swimmer; upload/download through `files.<domain>` | **Partly.** A club row was inserted directly; presigned upload/download through `files.` worked with a service-account-signed URL. Creating a swimmer and the API's own presigning need a session: **not run** |
| 5 | Restart api/worker and confirm data persists (T-26) | **Not run** through the API. See T-26 |
| 6 | Redis restart with a queued job; worker drain; fill toward the cap | **Rehearsed** (T-08). Worker drain on stop and a real Asynq job were not exercised |
| 7 | Encrypted backup, restore on a second empty VM | **Rehearsed on one host:** backup with real gpg/rclone/mc/pg_dump, restore onto emptied volumes. Keycloak users are not in the archive (by design); **not run:** a second host |
| 8 | Schema-changing release, failing migration, rollback | **Rehearsed** with `promote.sh`/`rollback.sh`: rel1 -> rel2 (additive migration, applied once) succeeded; rel3 (migration `SELECT 1/0`) aborted before any container changed and kept rel2 serving; `rollback.sh` returned to rel1 and the old release ran against the expanded schema. The gate used the liveness URL, because readiness is 503 with synthetic provider keys. Note: every promote overwrites `release.env.prev` with the then-current good release, so after a failed promote the rollback target is the current release, not the one before it |
| 9 | Cross-club isolation as `aquify_app`; Stripe test mode; Postmark sandbox | **Isolation: tested at the database** (T-06: unbound sees 0 rows, cannot write another club). **Not run:** API-level cross-club checks, Stripe test mode, Postmark sandbox |

## 5. Status

Production readiness is **not** claimed.

Closed in this pass: T-16 (app-level OIDC/webhook validation), T-19 (image pins), T-22 (TOTP sealed at rest), T-28
(staging flow, config-level), plus defects found only by running real containers (T-05 deadlock, T-07 realm file
name, T-18 Loki URL and Grafana plugin, T-10/T-11 script bugs, T-29 MinIO distribution).

Still open or unrun, by name:
- **T-21** malware scanning (no implementation): deferred by the owner. **T-27** the macOS timing flake (not reproduced, unchanged). T-24 was decided (at-least-once) and fixed.
- **T-29** MinIO is unmaintained upstream: owner accepted the risk and will keep it for now; revisit for a maintained S3 store.
- Not run anywhere: ACME on a public name, amd64 images, GitHub Actions (including the production admin image build and the staging workflow), a staging host deploy, Stripe and Postmark with real accounts (so readiness 200 and the promote gate with real providers), browser sign-in/MFA, a live create-restart-read through the API (T-26), API-level cross-club isolation, and a restore on a second host.

## 6. Final verification and known design debt

After the last edits: `go vet ./...` clean; `go test -count=1 -p 1 ./...` with real `psql`: exit 0 (run before the final
small simplifications; `./internal/app` and `./internal/app/authn` re-run afterwards and pass); the four deploy shell
tests pass.

Design debt noted by the cleanup review and deliberately **not** changed here: the API fetches Keycloak's JWKS through
the public ingress at startup (an internal JWKS URL would remove the caddy/Keycloak start-order coupling); the
integrations key also seals TOTP secrets and has no key id, so it cannot be rotated; TOTP sealing is applied at three
call sites rather than one row codec, and sealing is skipped when no key is configured (non-development startup
refuses to run without the key); legacy reseal runs under the load lock on first boot; staging and production compose
files duplicate hardening and will drift; backup.sh double-compresses and runs steps serially; production needs a
separate admin image because `NEXT_PUBLIC_*` is baked at build time.
