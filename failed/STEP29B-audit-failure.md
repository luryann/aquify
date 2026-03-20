# STEP 29B Audit Failure

Date: 2026-03-20
Step audited: STEP 29B
Result: FAIL

## Summary

STEP 29B does not currently meet the required bar.

The implementation does add real object-storage plumbing, export queuing, report artifact persistence, and cleanup hooks, but there are still material correctness and completion problems:

1. The server export generator regressed away from the repository's proven SD3/SDIF exporter logic and now emits incompatible record layouts/codes.
2. Export ordering regressed from numeric event ordering to plain lexicographic ordering.
3. The report-artifact portion is only partially completed: CSVs are persisted, but the request path still streams inline content and discards the stored artifact metadata/download URL rather than exposing the storage-backed artifact workflow described in the STEP 29B plan.

Because of those issues, the step does not satisfy:

- Condition 1: no bugs / no potential issues / no edge cases
- Condition 2: no regressions in code quality
- Condition 4: earlier directions completed fully

I did not find a new clear code-security vulnerability in the Step 29B paths I reviewed, but the overall step still fails because the other conditions are not well-met.

## What Went Wrong

### 1. Export file generation regressed to a non-canonical format

The repository already has a tested, parseable SD3/SDIF exporter in `internal/meetfiles/sdif/exporter.go`, but `server/internal/app/exports/service.go` now uses a separate handwritten formatter instead of reusing that implementation.

Concrete mismatches:

- `server/internal/app/exports/service.go` writes roster exports with `A0` file type `"4"` and `D3` records.
- The canonical exporter in `internal/meetfiles/sdif/exporter.go` writes roster exports with `A0` file type `"5"` and `D1` records.
- `server/internal/app/exports/service.go` writes SD3 stroke codes as letters (`B`, `R`, `F`, `I`, `E`, `M`, `S`) instead of the canonical numeric SDIF codes (`1`-`7`).
- `server/internal/app/exports/service.go` also changed the `D0` field layout away from the canonical positions used by the tested exporter.

This is a launch-blocking correctness problem because STEP 29B was supposed to change durability/async behavior, not silently replace the export wire format used for Meet Manager interoperability.

Relevant files:

- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go`

Relevant evidence:

- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go:485`
- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go:509`
- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go:567`
- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go:590`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go:141`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go:402`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go:489`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go:526`

### 2. Event ordering regressed

The canonical exporter sorts event numbers numerically when possible. The new server export code sorts using raw string comparison.

That means event `"10"` will sort before event `"2"`, which changes output ordering and can break downstream expectations/golden behavior.

Relevant files:

- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go`

Relevant evidence:

- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go:474`
- `/Users/ryan/Documents/aquify/internal/meetfiles/sdif/exporter.go:308`

### 3. Report artifact workflow is only partially implemented

The STEP 29B plan called for generated CSV report artifacts to be storage-backed artifacts rather than an inline-only in-memory response path. The code now stores the CSV in object storage, but the handlers immediately discard the returned artifact metadata and still stream the CSV directly in the same response.

That means the storage artifact exists, but the user-facing/reporting flow still does not expose a durable artifact URL or artifact object in the API response. This is only a partial completion of the Step 29B reporting goal.

Relevant files:

- `/Users/ryan/Documents/aquify/server/internal/app/reports/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/reports.go`
- `/Users/ryan/Documents/aquify/docs/planning/STEP29B-artifact-durability-plan.md`

Relevant evidence:

- `/Users/ryan/Documents/aquify/server/internal/app/reports/service.go:78`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/reports.go:123`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/reports.go:127`

## How To Fix It

1. Remove the duplicated export-file formatter from `server/internal/app/exports/service.go`.
2. Reuse the canonical exporter from `internal/meetfiles/sdif/exporter.go` for:
   - standard SD3
   - extended SD3
   - SDIF roster export
3. Restore canonical event ordering by using the same numeric-aware event sort logic already implemented in the meetfiles exporter.
4. Add tests at the server export layer that assert the generated bytes match the canonical exporter output or at minimum parse cleanly and match existing golden expectations.
5. Finish the report-artifact flow by returning artifact metadata or a storage-backed download URL/object in the relevant report responses, or by adding an explicit report-artifact retrieval endpoint that the request path returns.
6. Add report artifact tests that verify:
   - the CSV is stored in object storage
   - the storage key/expiry metadata is retained
   - the API exposes the artifact/download path rather than silently discarding it

## What Counts As Passing

STEP 29B should count as passing only when all of the following are true:

1. Export generation is storage-backed and async, while still using the repository's canonical, tested SD3/SDIF format implementation.
2. Server export output is covered by tests that would fail on:
   - wrong record types
   - wrong stroke/course encoding
   - wrong field positions
   - wrong event ordering
3. CSV report generation persists artifacts to object storage and exposes a durable artifact contract in the API, rather than only writing the file inline and dropping the artifact metadata.
4. Cleanup tests verify stale imports, expired exports, and expired reports are all transitioned and deleted correctly.
5. The Step 29B docs (`TODO.md`, `CLAUDE.md`, plan doc, API contract) match the shipped behavior exactly.

## Files Edited In STEP 29B

This is the master list of files I identified as part of the Step 29B implementation set:

- `/Users/ryan/Documents/aquify/.env.example`
- `/Users/ryan/Documents/aquify/CLAUDE.md`
- `/Users/ryan/Documents/aquify/Makefile`
- `/Users/ryan/Documents/aquify/TODO.md`
- `/Users/ryan/Documents/aquify/docs/api/openapi.yaml`
- `/Users/ryan/Documents/aquify/docs/planning/STEP29B-artifact-durability-plan.md`
- `/Users/ryan/Documents/aquify/server/cmd/api/main.go`
- `/Users/ryan/Documents/aquify/server/go.mod`
- `/Users/ryan/Documents/aquify/server/go.sum`
- `/Users/ryan/Documents/aquify/server/internal/app/config.go`
- `/Users/ryan/Documents/aquify/server/internal/app/exports/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/exports.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/imports.go`
- `/Users/ryan/Documents/aquify/server/internal/app/handlers/reports.go`
- `/Users/ryan/Documents/aquify/server/internal/app/imports/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/reports/service.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server_exports_test.go`
- `/Users/ryan/Documents/aquify/server/internal/app/server_imports_test.go`
- `/Users/ryan/Documents/aquify/server/internal/storage/client.go`
- `/Users/ryan/Documents/aquify/server/internal/workers/jobhandlers/cleanup.go`
- `/Users/ryan/Documents/aquify/server/internal/workers/jobhandlers/exports.go`
- `/Users/ryan/Documents/aquify/server/internal/workers/jobhandlers/imports.go`

## Verification Performed

I reviewed:

- `/Users/ryan/Documents/aquify/design-language-guide.html`
- `/Users/ryan/Documents/aquify/docs/planning/Developer Handoff Master Document.md`
- `/Users/ryan/Documents/aquify/docs/planning/Plan.md`
- `/Users/ryan/Documents/aquify/CLAUDE.md`
- `/Users/ryan/Documents/aquify/TODO.md`
- the Step 29B plan doc
- the Step 29B code paths listed above

I also ran:

- `env GOCACHE=/Users/ryan/Documents/aquify/.gocache/go-build GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache go test ./internal/app`
- `env GOCACHE=/Users/ryan/Documents/aquify/.gocache/go-build GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache go test ./internal/workers/jobhandlers`
- `env GOCACHE=/Users/ryan/Documents/aquify/.gocache/go-build GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache go test ./...` from `/Users/ryan/Documents/aquify/internal/meetfiles`

Those test runs passed, but they do not invalidate the failures above because the current server export path is not sufficiently covered against the repository's canonical SD3/SDIF expectations.
