# STEP29A Persistence Execution Plan

## Goal

Finish `TODO.md` Step `29A) Persistent backend runtime` by replacing the
current process-memory-only backend runtime with database-backed persistence for
the existing domain services, then verify the converted runtime against the real
database harness.

## Current repo reality

- `server/internal/app/server.go` still wires almost every business domain to an
  in-memory service constructor.
- The domain tables already exist in `db/migrations`, but the runtime does not
  yet have a shared DB adapter/repository layer.
- `server/internal/testutil` currently provides a `psql`-driven real database
  harness, which is enough to validate persistence behavior without introducing
  a large new query-generation toolchain in the same change.

## Execution strategy

1. Add a small Postgres-backed runtime snapshot repository.
2. Persist the existing in-memory service state into that repository after each
   successful write so process restarts no longer discard core state.
3. Load persisted state during service construction when a database URL is
   configured.
4. Wire the server to use the persistent runtime automatically when
   `DATABASE_URL` is available.
5. Add real-database round-trip tests for the converted domains.

## Scope for this pass

- Membership / groups / meets
- Imports / exports / performance
- Billing / comms / compliance
- Report composition paths via persisted domain services

## Notes

- This is an implementation bridge, not the final sqlc/pgx end-state.
- The purpose of Step 29A is production durability first: surviving process
  restarts and running against a real database.
- Mutating API requests must not return success unless the snapshot save for the
  resulting state also succeeds.
- Follow-on work can still replace snapshot persistence with fully normalized
  repositories per table without changing the higher-level handlers.

## Checkpoints

- [x] Add runtime snapshot storage migration (2026-03-21: runtime_snapshots table via psql)
- [x] Add snapshot repository package (2026-03-21: server/internal/runtimepersist)
- [x] Convert service constructors to optionally load persisted state (2026-03-21: LoadRuntimeSnapshot)
- [x] Persist writes for converted domains (2026-03-21: runtimePersistenceMiddleware)
- [x] Wire `server.go` to the persistent runtime (2026-03-21)
- [x] Add real-DB round-trip tests (2026-03-21: runtime_persistence_integration_test.go)
- [x] Update `CLAUDE.md` and `TODO.md` (2026-03-21)

## Phase 2: Full DB-backed repositories (2026-03-22)

- [x] Add pgx/v5 dependency (github.com/jackc/pgx/v5)
- [x] Create server/internal/db/pool.go (pgxpool connection management)
- [x] Create members/repository.go (households, contacts, swimmers, swimmer_groups)
- [x] Create groups/repository.go (groups, eligibility_rules, registrations, waiver_acceptances)
- [x] Create meets/repository.go (meets, sessions, events, commitments, entries)
- [x] Create performance/repository.go (results, best_times, time_standards)
- [x] Create billing/repository.go (invoices, invoice_items, payments, refunds, ledger_events)
- [x] Create comms/repository.go (announcements, email_campaigns)
- [x] Create compliance/repository.go (data_export_requests, data_deletion_requests)
- [x] Create imports/repository.go (file_imports)
- [x] Create exports/repository.go (file_exports)
- [x] Add NewServiceWithPool constructors to all 9 services
- [x] Wire DB pool in server.go (NewServiceWithPool replaces NewService when DATABASE_URL set)
- [x] Add integration tests (server_db_repos_test.go) for all converted domains
- [x] Update CLAUDE.md and TODO.md
