# STEPS 1–18 Implementation Audit Failure

**Date:** 2026-03-16  
**Scope reviewed:** `design-language-guide.html`, `docs/planning/Developer Handoff Master Document.md`, `docs/planning/Plan.md`, `CLAUDE.md`, `TODO.md`, plus representative code and tests under `server/`, `db/`, `internal/meetfiles/`, `docs/`, `scripts/`.

## Verdict

**FAILED** — `TODO.md` marks all items in STEPS 1–18 as complete, but at least **two STEP 1–18 checklist items are not actually implemented** as written:

1. **STEP 12:** “Add service: update future entry seed times from results import” is present only as a **no-op stub**.
2. **STEP 18:** “Add CI step to validate OpenAPI schema” is marked done, but there is **no CI configuration** in the repo.

This report focuses on mismatches where **TODO says done but the codebase does not satisfy the statement**.

## What Looks Implemented (High Confidence)

These are not exhaustive proofs, but they have concrete artifacts in the repo consistent with STEPS 1–11 and large parts of 13–18:

- **STEP 1–2 (Bootstrap + infra):** `.editorconfig`, `.gitignore`, `.env.example`, `.env.local.example`, `Makefile`, `docker-compose.yml`, `scripts/wait-for-db.sh`, `server/go.mod`, and the Next.js `admin/` app exist and are wired in `Makefile`.
- **STEP 3 (Migrations):** `db/migrations/*.sql` exist with Goose `-- +goose Up/Down` blocks; Make targets (`db-up`, `db-down`, `db-status`) run Goose via `go run`.
- **STEP 4–11 (API + domains + meetfiles):** `server/internal/app/server.go` wires middleware and routes; `internal/meetfiles/` contains SDIF/SD3 parsing/export and tests (including SDIF roster round-trip tests).
- **STEP 14, 17, 18 (Stripe, jobs, OpenAPI):** Stripe is implemented as an interface + stub by design; background workers exist under `server/internal/workers/`; OpenAPI files exist under `docs/api/` and are served by `server/internal/app/handlers/openapi.go`.

Even where implementations are in-memory scaffolds, many endpoints and tests exist and appear coherent.

## Failure 1 (STEP 12) — Seed-Time Update Service Is Not Implemented

### What went wrong

`TODO.md` (STEP 12) says:
- [x] “Add service: update future entry seed times from results import”

But the actual code is explicitly a stub:
- `server/internal/app/performance/service.go` defines `UpdateFutureEntrySeedTimes(...)` as a **no-op** with a production TODO and returns `nil` without updating any entries.

This means that importing/upserting results cannot actually update any future meet entry seed times, which is a primary interoperability workflow called out in both:
- `docs/planning/Plan.md` (update-times workflow), and
- `docs/planning/Developer Handoff Master Document.md` (Primary interoperability requirement #4).

### How to fix

Implement a real seed-time update path that actually mutates future entries. Any of the following is acceptable; pick one and document it:

1. **Direct synchronous integration (scaffold-friendly):**
   - Introduce an interface in the performance domain, e.g. `type EntrySeedTimeUpdater interface { UpdateSeedTimesFromBestTimes(orgID, swimmerID string) error }`.
   - Have `meets.Service` implement it (in-memory for now).
   - Inject that dependency when wiring services in `server/internal/app/server.go`.
   - In `performance.Service.UpsertResult`, call the updater after recomputing best times.

2. **Background job approach (aligned with STEP 17):**
   - Enqueue a task (e.g. `performance:update_seed_times`) to the worker server from `UpsertResult`.
   - Add a job handler that calls into meets/entries to update seed times.
   - Ensure the task is visible and retryable via `/api/v1/jobs`.

3. **Database-first approach (future):**
   - If/when migrating to DB-backed services, implement the update via SQL transactions updating entry seed time columns based on new best times.

### What counts as passing

- A deterministic, test-covered behavior exists such that:
  - After a result is upserted for a swimmer (and best times are recomputed), **future draft/pending entries** for that swimmer have seed times updated to reflect the new best time (or defined policy).
- Add at least one server-level test showing this end-to-end. A minimal pass is a test that:
  - Creates a swimmer and a meet entry with an initial seed time,
  - Upserts a faster result for that swimmer,
  - Verifies the entry seed time is updated accordingly.

## Failure 2 (STEP 18) — “Add CI Step” Is Marked Done But No CI Exists

### What went wrong

`TODO.md` (STEP 18) says:
- [x] “Add CI step to validate OpenAPI schema”

The repo contains:
- `docs/api/openapi.yaml`
- `docs/api/.spectral.yaml`
- `Makefile` target `lint-openapi` that runs Spectral if installed

But there is **no CI pipeline definition** in this repository to actually run it:
- No `.github/workflows/*`
- No other CI config files were found at typical locations.

As written, “CI step” implies a committed automation that runs on push/PR (or at least a documented CI integration file), not just a Make target.

### How to fix

Add a CI workflow that runs at minimum:
- `make lint-openapi`
- Optionally `make test` and/or `make lint`

If GitHub Actions is the intended CI, create a workflow file like:
- `.github/workflows/ci.yml`

Ensure Spectral is installed in the CI environment (either via `npm install -g @stoplight/spectral-cli` or `npx @stoplight/spectral-cli ...`).

### What counts as passing

- A CI config exists in-repo (for the chosen CI provider) that runs OpenAPI validation on every PR/push.
- A failing OpenAPI spec causes CI to fail (proving it is gated).

## Additional Notable Risks (Not Counted As “TODO Done But Not Done”)

These are important gaps relative to the Developer Handoff requirements, but they are not necessarily explicit TODO checklist statements:

- `server/internal/app/performance/service.go` defers **parent household scoping** for read endpoints (“TODO(production)”), which conflicts with the portal privacy requirements in `docs/planning/Developer Handoff Master Document.md` section 1.1A/1.1B/1.1C.

