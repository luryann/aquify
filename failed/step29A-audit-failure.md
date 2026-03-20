# Step 29A Audit Failure

Date: 2026-03-18
Task audited: `STEP 29A`

## Outcome

Step 29A does **not** meet all four required conditions.

Conditions that failed:

1. `No bugs, no UI issues, potential issues, or edge cases`
4. `Followed all directions presented earlier and completed them fully`

Conditions that appear acceptable from this audit:

2. `No clear regression in code quality`
3. `No clear code security issues`

## 1) What went wrong

### A. The implementation does not satisfy the Step 29A acceptance text

`TODO.md` marks Step 29A complete with these checked items:

- `Replace in-memory member/group/meet services with DB-backed repositories.`
- `Replace in-memory import/export/performance services with DB-backed repositories.`
- `Replace in-memory billing/comms/compliance/report composition paths with DB-backed repositories.`
- `Add transactional repository tests against the real database harness for all converted domains.`

But the audited implementation is explicitly a snapshot bridge over the same in-memory services, not DB-backed repositories:

- `docs/planning/STEP29A-persistence-execution-plan.md` says this pass adds `a small Postgres-backed runtime snapshot repository` and notes `This is an implementation bridge, not the final sqlc/pgx end-state.`
- `CLAUDE.md` says the current Step 29A layer is `a whole-domain snapshot bridge rather than the final normalized repository implementation.`
- `server/internal/app/server.go` still constructs the same in-memory services with `members.NewService()`, `groups.NewService()`, `meets.NewService()`, `imports.NewService()`, `exports.NewService()`, `performance.NewService()`, `billing.NewService()`, `comms.NewService()`, and `compliance.NewService()`.

That means the code and docs are internally consistent with each other, but they do **not** fully match the checked-off Step 29A wording in `TODO.md`.

### B. Successful write responses can still lose durability

The persistence middleware writes the HTTP response first, then attempts snapshot persistence:

- `server/internal/app/runtime_persistence.go:109-123`

The failure path is ignored:

- `server/internal/app/runtime_persistence.go:122` uses `_ = persistRuntimeSnapshots(...)`

So if Postgres or `psql` fails after a successful mutating API request, the client still receives success even though the runtime state was not durably persisted. For a step whose stated purpose is process-restart durability, that is a real edge-case bug.

### C. The real database verification is incomplete in this audit environment

The Step 29A integration test exists, but it was skipped because `DATABASE_URL` is not set:

- `server/internal/app/runtime_persistence_integration_test.go:25`

That means the strongest test for the new persistence path did not actually run during this audit. The code may still work on a real database, but this audit cannot honestly certify that condition 1 was fully met.

## 2) How to fix it

Two valid paths exist. Only one should be chosen.

### Path A: Make Step 29A honestly match what was implemented

If Step 29A is intended to be the snapshot bridge:

- Update `TODO.md` so Step 29A describes a `runtime_snapshots` durability bridge instead of `DB-backed repositories`.
- Update any handoff language that could mislead future implementers into believing repository conversion is complete.
- Add an explicit note that normalized repository conversion remains future work.

This fixes the direction/completeness mismatch, but it does **not** fix the durability bug below.

### Path B: Make the implementation match the original Step 29A wording

If Step 29A must truly be `DB-backed repositories`:

- Replace the in-memory service backing stores with real repository implementations per domain.
- Persist normalized records in the existing domain tables instead of whole-domain JSON blobs.
- Move snapshot bridging language out of `CLAUDE.md` and the Step 29A plan once real repositories are in place.

This is a larger change, but it is the only way the current checked boxes in `TODO.md` become literally true.

### Required fix regardless of path

The middleware durability bug should be fixed either way:

- Do not return a successful mutating response until persistence succeeds, or
- persist before sending the response, or
- route writes through transactional service/repository methods so the API response reflects durable commit status, or
- if asynchronous persistence is intentionally accepted, explicitly downgrade the claim from durable persistence and surface/save failure state instead of silently swallowing it.

Also add tests for:

- persistence failure after a mutating request
- startup hydrate failure behavior
- multiple sequential writes with restart between them
- export/import content round-trip durability

## 3) What counts as passing

This audit should count as passing only if all of the following are true:

1. The implementation and the task wording match exactly enough that a future engineer would not be misled about whether repository conversion is complete.
2. Mutating API success implies durable persistence success, or the system explicitly documents and surfaces weaker guarantees.
3. The real DB persistence test runs against an actual `DATABASE_URL` + `psql` harness and passes.
4. The Step 29A persistence path is covered for all claimed converted domains.
5. `TODO.md`, `CLAUDE.md`, and the Step 29A planning document all describe the same completed scope.

## 4) Master list of files edited in Step 29A

This is the Step 29A-specific file set I could confirm from the implementation artifacts and references in the repo:

- `CLAUDE.md`
- `TODO.md`
- `docs/planning/STEP29A-persistence-execution-plan.md`
- `db/migrations/20260318090000_create_runtime_snapshots.sql`
- `server/internal/runtimepersist/repository.go`
- `server/internal/app/runtime_persistence.go`
- `server/internal/app/runtime_persistence_integration_test.go`
- `server/internal/app/server.go`
- `server/internal/app/members/runtime_snapshot.go`
- `server/internal/app/groups/runtime_snapshot.go`
- `server/internal/app/meets/runtime_snapshot.go`
- `server/internal/app/imports/runtime_snapshot.go`
- `server/internal/app/exports/runtime_snapshot.go`
- `server/internal/app/performance/runtime_snapshot.go`
- `server/internal/app/billing/runtime_snapshot.go`
- `server/internal/app/comms/runtime_snapshot.go`
- `server/internal/app/compliance/runtime_snapshot.go`

## Audit evidence

Reviewed documents:

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `CLAUDE.md`
- `TODO.md`

Verification run during audit:

- `cd /Users/ryan/Documents/aquify/admin && npm run build` -> passed
- `GOCACHE=$(pwd)/.gocache go test ./internal/runtimepersist ./internal/testutil ./internal/app/members ./internal/app/groups ./internal/app/meets ./internal/app/imports ./internal/app/exports ./internal/app/performance ./internal/app/billing ./internal/app/comms ./internal/app/compliance` -> passed for available tests/packages
- `GOCACHE=$(pwd)/.gocache go test ./internal/app -run TestRuntimeSnapshotsRoundTripAcrossConvertedDomains -count=1 -v` -> skipped because `DATABASE_URL` was not set
