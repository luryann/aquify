# STEP 9 — File Interoperability Data Model: Implementation Notes

## Scope
STEP 9 implements the data model and API layer for file import/export tracking, covering:
- Database migrations: file_imports, file_import_rows, file_import_errors, match_conflicts, file_exports
- Object storage client wrapper (interface + in-memory stub for MinIO/S3)
- API endpoints for import job management, conflict resolution, and report download
- Config extension for storage settings
- RBAC enforcement throughout

## File Import State Machine
`uploaded → scanning → parsing → matching → conflicts_pending → applied → failed → archived`

- `uploaded`: Record created, presigned upload URL issued
- `scanning`: File received, malware/type check underway (async)
- `parsing`: File is being parsed (async)
- `matching`: Swimmer/event matching in progress (async)
- `conflicts_pending`: Parsing done; one or more records have unresolved conflicts
- `applied`: All records matched and applied to the database
- `failed`: Unrecoverable error at any stage
- `archived`: Historical record; no further action needed

## Match Conflict State Machine
`open → resolved_manual | resolved_auto | rejected → closed`

- `open`: Conflict detected, awaiting resolution
- `resolved_manual`: Admin/coach manually matched the record to a swimmer
- `resolved_auto`: System found a deterministic match after additional data
- `rejected`: Record rejected/skipped (e.g., not a known swimmer for this org)
- `closed`: Conflict fully processed and no further action needed

## File Export State Machine
`pending → generating → ready → failed → expired`

## RBAC Permissions

| Action | owner | admin | coach | parent | billing | read-only |
|--------|-------|-------|-------|--------|---------|-----------|
| Request upload URL | Y | Y | Y | N | N | N |
| Finalize import | Y | Y | Y | N | N | N |
| List imports | Y | Y | Y | N | N | N |
| Get import detail | Y | Y | Y | N | N | N |
| List conflicts | Y | Y | Y | N | N | N |
| Resolve conflict | Y | Y | N | N | N | N |
| Download report | Y | Y | Y | N | N | N |

## API Endpoints

```
POST   /imports/upload-url                                   request presigned upload URL
POST   /imports/{importID}/finalize                          finalize uploaded import file
GET    /imports                                              list import jobs
GET    /imports/{importID}                                   get import details
GET    /imports/{importID}/conflicts                         list conflicts for import
POST   /imports/{importID}/conflicts/{conflictID}/resolve    resolve conflict (admin/owner only)
GET    /imports/{importID}/report                            download import report (JSON)
```

## Migration Numbering
Continuing from 20260315002400:
- 20260315002500_create_file_imports.sql
- 20260315002600_create_file_import_rows.sql
- 20260315002700_create_file_import_errors.sql
- 20260315002800_create_match_conflicts.sql
- 20260315002900_create_file_exports.sql

## Implementation Files
- `db/migrations/20260315002500_create_file_imports.sql`
- `db/migrations/20260315002600_create_file_import_rows.sql`
- `db/migrations/20260315002700_create_file_import_errors.sql`
- `db/migrations/20260315002800_create_match_conflicts.sql`
- `db/migrations/20260315002900_create_file_exports.sql`
- `server/internal/storage/client.go`          — ObjectStorage interface + stub
- `server/internal/app/imports/service.go`     — in-memory service
- `server/internal/app/handlers/imports.go`    — HTTP handlers
- `server/internal/app/server.go`              — updated router wiring
- `server/internal/app/config.go`              — add StorageConfig
- `server/internal/app/server_imports_test.go` — handler tests

## Key Design Decisions
1. Object storage wrapper defines an `ObjectStorage` interface so the stub can be swapped for a real MinIO/S3 implementation without touching service code.
2. In-memory service follows the same in-memory pattern as meets/groups/members services.
3. Presigned upload URLs in the stub are returned as deterministic fake strings (not real HTTP URLs); real implementation replaces this.
4. Conflict resolution is owner/admin only (coaches can view but not resolve).
5. Import report is a JSON blob capturing summary counts, error list, and row match results.
6. `file_import_rows` tracks each parsed row; `file_import_errors` tracks hard parse errors; `match_conflicts` tracks identity ambiguity.
7. Config adds `STORAGE_ENDPOINT`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_BUCKET`, `STORAGE_USE_SSL` env vars (stubbed, not wired to real storage yet).
