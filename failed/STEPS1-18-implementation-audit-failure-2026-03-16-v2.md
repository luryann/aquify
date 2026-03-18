# STEPS 1–18 Implementation Audit Failure (v2)

**Date:** 2026-03-16  
**Repo:** `/Users/ryan/Documents/aquify`  
**Primary checklist:** `/Users/ryan/Documents/aquify/TODO.md`  
**Other required references reviewed:**  
- `/Users/ryan/Documents/aquify/design-language-guide.html`  
- `/Users/ryan/Documents/aquify/docs/planning/Developer Handoff Master Document.md`  
- `/Users/ryan/Documents/aquify/docs/planning/Plan.md`  
- `/Users/ryan/Documents/aquify/CLAUDE.md`  
- `/Users/ryan/Documents/aquify/docs/planning/STEP12-implementation-plan.md`  
- `/Users/ryan/Documents/aquify/docs/planning/STEP18-implementation-plan.md`

## Verdict

**FAILED** — `TODO.md` marks all STEP 1–18 items as complete (`[x]`), but at least **two checklist items** are not implemented as written:

1) **STEP 12:** “Service: update future entry seed times from results import” is still a **no-op stub**.  
2) **STEP 18:** “Add CI step to validate OpenAPI schema” is checked done, but there is **no CI configuration** in-repo.

In addition, there are documentation consistency issues (not “code missing” per se) that increase the risk of false confidence:
- `/Users/ryan/Documents/aquify/CLAUDE.md:32` states STEP 0–18 are complete, which is not true given the two gaps above.
- `/Users/ryan/Documents/aquify/docs/planning/STEP18-implementation-plan.md:165` shows STEP 18 implementation status checkboxes as unchecked even though artifacts exist (openapi files + handler + Makefile target).

---

## Failure 1 (STEP 12) — Seed-time update service is not implemented

### What went wrong

`/Users/ryan/Documents/aquify/TODO.md:218` is checked `[x]`:
- “Add service: update future entry seed times from results import”

But `/Users/ryan/Documents/aquify/server/internal/app/performance/service.go:518` is explicitly a stub:
- `UpdateFutureEntrySeedTimes(...)` returns `nil` and does not mutate any entries.

This is also called out as a core interoperability requirement in:
- `/Users/ryan/Documents/aquify/docs/planning/Plan.md` (update-times workflow), and
- `/Users/ryan/Documents/aquify/docs/planning/Developer Handoff Master Document.md:20` (primary interoperability requirement #4).

### How to fix

Implement a real seed-time update path that actually updates future meet entries for the swimmer after results import updates best times. Any of these approaches can pass; pick one and document the policy:

1) **Direct synchronous path (fastest to ship in current in-memory architecture):**
   - Add an interface owned by performance domain (or injected into it) that can update entries for a swimmer.
   - Implement it in the meets/entries domain (current in-memory service).
   - Call it from `UpsertResult` after best-time recompute (outside locks if non-trivial).

2) **Job-based path (aligned with STEP 17 queue semantics):**
   - Enqueue a `performance:update_seed_times` task from the results upsert flow.
   - Add a job handler that pulls best times and updates draft/pending entries.
   - Add at least one test that proves the queue path updates entries deterministically.

3) **DB-backed path (future):**
   - When entries are persisted in DB, implement a SQL transaction that updates entry seed times based on newly computed best times.

### What counts as passing

At minimum:
- A deterministic, test-covered behavior exists such that after upserting an official, non-DQ result that improves best time, **future draft/pending entries** for that swimmer have seed times updated per a defined policy.
- Add a server-level test that:
  - Creates a swimmer + future entry with an initial seed time,
  - Upserts a faster result,
  - Asserts the entry’s seed time is updated.

---

## Failure 2 (STEP 18) — “CI step to validate OpenAPI schema” does not exist

### What went wrong

`/Users/ryan/Documents/aquify/TODO.md:309` is checked `[x]`:
- “Add CI step to validate OpenAPI schema”

But there is no CI configuration in this repo:
- No `/Users/ryan/Documents/aquify/.github/workflows/*`
- No other common CI config files (GitLab CI, CircleCI, Buildkite, etc.) present at the repo root.

The repo *does* have local lint artifacts:
- `/Users/ryan/Documents/aquify/docs/api/openapi.yaml`
- `/Users/ryan/Documents/aquify/docs/api/.spectral.yaml`
- `/Users/ryan/Documents/aquify/Makefile` includes `lint-openapi`

…but a Make target alone is not a “CI step”.

### How to fix

Add an in-repo CI workflow (provider of choice). If GitHub Actions is intended:
- Create `/Users/ryan/Documents/aquify/.github/workflows/ci.yml` that runs:
  - `make lint-openapi` (ensuring Spectral is available via `npm` / `npx`)
  - (Optionally) `make test` and `make lint` to gate on unit tests/static analysis.

Also adjust `Makefile`’s `lint-openapi` to always run Spectral in CI (common pattern: use `npx @stoplight/spectral-cli ...` so no global install is required).

### What counts as passing

- A CI config exists in-repo that runs OpenAPI validation on every push/PR.
- Breaking `docs/api/openapi.yaml` causes the CI job to fail (proving it is a real gate).

---

## Cross-document consistency issues (not counted as “missing implementation”, but should be fixed)

1) `/Users/ryan/Documents/aquify/CLAUDE.md:32` claims STEP 0–18 are complete.
   - This conflicts with the two failures above (STEP 12 seed-time update and STEP 18 CI gate).
2) `/Users/ryan/Documents/aquify/docs/planning/STEP18-implementation-plan.md:165` shows unchecked implementation status boxes for artifacts that exist.
   - Either update those checkboxes or remove that status section to avoid misleading readers.

---

## Master list (best-effort) — Files most relevant to these failures

Because this directory is not a git repo in this environment (no `.git/`), this list is assembled from the failure locations and the STEP 12/18 implementation wiring:

- STEP 12 (seed-time updates):
  - `/Users/ryan/Documents/aquify/server/internal/app/performance/service.go`
  - `/Users/ryan/Documents/aquify/server/internal/app/handlers/performance.go`
  - `/Users/ryan/Documents/aquify/docs/planning/STEP12-implementation-plan.md`
  - `/Users/ryan/Documents/aquify/TODO.md`
- STEP 18 (CI + OpenAPI lint gate):
  - `/Users/ryan/Documents/aquify/Makefile`
  - `/Users/ryan/Documents/aquify/docs/api/openapi.yaml`
  - `/Users/ryan/Documents/aquify/docs/api/.spectral.yaml`
  - `/Users/ryan/Documents/aquify/server/internal/app/handlers/openapi.go`
  - `/Users/ryan/Documents/aquify/docs/planning/STEP18-implementation-plan.md`

