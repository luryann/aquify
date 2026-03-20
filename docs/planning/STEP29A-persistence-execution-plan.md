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

- [ ] Add runtime snapshot storage migration
- [ ] Add snapshot repository package
- [ ] Convert service constructors to optionally load persisted state
- [ ] Persist writes for converted domains
- [ ] Wire `server.go` to the persistent runtime
- [ ] Add real-DB round-trip tests
- [ ] Update `CLAUDE.md` and `TODO.md`
