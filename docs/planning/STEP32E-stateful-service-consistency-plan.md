# STEP 32E Stateful Service Consistency And Persistence Hardening Plan

## Goal

Finish `TODO.md` step `32E)` by removing the remaining DB-backed request paths
that still rely on process-local state after startup, then prove that writes,
reads, and duplicate-prevention logic stay correct across multiple API
instances and restarts.

## Problems Identified

- `training.Service` still treats its in-memory map as the active read source in
  DB-backed mode, so a second API instance can miss creates/updates/deletes
  performed by another instance until restart.
- `groups.Service` writes to Postgres in DB-backed mode but still validates
  conflicts against process-local maps and, in several paths, logs DB write
  failures while returning success and mutating local state anyway.
- `members.Service` also keeps a startup-loaded in-memory copy of DB-backed
  data, so cross-instance household/swimmer/link changes are not guaranteed to
  appear without a restart.

## Scope

1. Make DB-backed `training` reads and writes consult the repository as the
   canonical source instead of only startup-loaded memory.
2. Add DB-backed reload/read-through behavior for the stale `members` and
   `groups` request paths that need cross-instance consistency.
3. Return persistence errors from `groups` write paths instead of swallowing
   them and diverging local state from the DB.
4. Add focused DB integration coverage for:
   - cross-instance members visibility after create/update/link changes
   - cross-instance groups conflict handling and state visibility
   - cross-instance training create/update/delete visibility

## Notes

- In-memory mode remains available for lightweight unit tests.
- DB-backed mode should treat Postgres as the source of truth for core domain
  reads and conflict checks.
- Runtime snapshot helpers remain backward-compatible, but they should no longer
  be the only mechanism that restores correct state after restart.

## Verification Checklist

- [ ] DB-backed services no longer return success after a rejected shared-DB write
- [ ] Members/groups/training tests prove second-instance visibility without restart
- [ ] Duplicate/conflict paths remain correct under multi-instance DB-backed execution
- [ ] `CLAUDE.md` updated if conventions changed
- [ ] Final diff review confirms `TODO.md` step `32E` coverage
