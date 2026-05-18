# STEP29B Artifact Durability Plan

## Goal

Finish `TODO.md` Step `29B) Artifact storage and async durability` by replacing
stubbed artifact handling with real S3-compatible storage, moving export
generation onto background workers, and adding lifecycle cleanup for expiring
artifacts.

## Current repo reality

- `server/internal/storage/client.go` only exposes fake presigned URLs and a
  no-op delete path.
- `server/internal/app/imports/service.go` stores import metadata, but uploaded
  objects are never read, validated against real object storage, or cleaned up.
- `server/internal/app/exports/service.go` generates export files synchronously
  and retains bytes in process memory.
- `server/internal/app/handlers/reports.go` builds CSV downloads inline and does
  not persist generated artifacts anywhere.
- `server/internal/workers/jobhandlers/exports.go` and
  `server/internal/workers/jobhandlers/cleanup.go` are scaffold handlers.

## Execution strategy

1. Extend the object storage abstraction to support real S3-compatible
   operations needed by the app: put/get/delete, existence checks, and
   presigned URLs.
2. Replace the storage stub with a MinIO/S3-backed implementation when storage
   credentials are configured, while preserving an in-memory fake for tests.
3. Convert exports to queue-backed generation:
   create pending jobs in the request path, enqueue a worker payload, generate
   bytes in the worker, upload artifacts to storage, and update export metadata
   to ready/failed.
4. Persist generated CSV report artifacts in object storage and return download
   URLs instead of keeping file bytes in memory.
5. Add expiry metadata and a cleanup sweep that deletes expired export/report
   artifacts and expires stale import upload windows.

## Scope for this pass

- Import upload/download/delete and presigned URL durability
- Export artifact persistence and async generation
- Generated CSV report persistence
- TTL/cleanup handling for export/report artifacts and stale upload URLs

## Notes

- This pass should fit the current runtime snapshot bridge rather than attempt a
  full normalized artifact table redesign.
- Synchronous JSON report responses can remain for operator visibility, but CSV
  downloads should become storage-backed artifacts.
- Export generation requests must not report `ready` until the worker has
  uploaded the artifact and persisted the resulting metadata.

## Checkpoints

- [ ] Add Step 29B planning artifact
- [ ] Replace stub-only storage client with real S3-compatible implementation
- [ ] Move export generation to background workers with storage-backed artifacts
- [ ] Persist generated CSV report artifacts in object storage
- [ ] Implement expiry metadata and cleanup sweeper behavior
- [ ] Add or update tests
- [ ] Update `CLAUDE.md` and `TODO.md`
