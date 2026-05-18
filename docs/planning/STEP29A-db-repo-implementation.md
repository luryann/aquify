# STEP29A DB Repository Implementation

## Goal
Replace in-memory service maps with DB-backed repositories using pgx/v5.

## Approach: Write-Through Cache with pgxpool

### Strategy
1. Add pgx/v5 dependency
2. Create `server/internal/db` package (pool management)
3. Per-domain `repository.go` with INSERT/UPDATE/SELECT operations
4. Service constructor `NewServiceWithPool(pool)` that:
   - On startup: loads all records from DB into in-memory maps
   - On writes: writes to DB first, then updates in-memory map
   - On reads: reads from in-memory map (fast path)
5. Wire in server.go
6. Add integration tests with real pool

### ID Format
- DB-backed path: use `uuid.New().String()` for IDs (UUID format required by DB schema)
- In-memory (nil pool) path: keep `"hsh_" + randomToken(12)` (existing tests unchanged)
- Existing tests are unaffected since they use in-memory path

### org_id consideration
- DB schema uses `org_id UUID NOT NULL`
- Test orgs use "org-test" (not UUID)
- Integration tests will use proper UUID org IDs
- In-memory tests (existing) still use "org-test" - those don't touch normalized tables

## Implementation Checklist

### Phase 1: Infrastructure
- [ ] Add pgx/v5 to go.mod
- [ ] Create `server/internal/db/pool.go`

### Phase 2: Members Domain
- [ ] Create `members/repository.go` (households, contacts, swimmers, swimmer_groups)
- [ ] Update `members/service.go` to use repository
- [ ] Add `NewServiceWithPool` constructor

### Phase 3: Groups Domain
- [ ] Create `groups/repository.go` (groups, eligibility_rules, registrations, waiver_acceptances)
- [ ] Update `groups/service.go` to use repository

### Phase 4: Meets Domain
- [ ] Create `meets/repository.go` (meets, sessions, events, commitments, entries, entry_legs)
- [ ] Update `meets/service.go` to use repository

### Phase 5: Imports/Exports/Performance
- [ ] Create `imports/repository.go`
- [ ] Create `exports/repository.go`
- [ ] Create `performance/repository.go`
- [ ] Update services

### Phase 6: Billing/Comms/Compliance
- [ ] Create `billing/repository.go`
- [ ] Create `comms/repository.go`
- [ ] Create `compliance/repository.go`
- [ ] Update services

### Phase 7: Wire in server.go
- [ ] Create DB pool from DATABASE_URL
- [ ] Pass pool to all service constructors
- [ ] Keep snapshot bridge as fallback

### Phase 8: Integration Tests
- [ ] Add `server/internal/app/server_db_repos_test.go`
- [ ] Test members domain round-trip
- [ ] Test groups domain round-trip
- [ ] Test meets domain round-trip
- [ ] Test billing domain round-trip

## Key SQL Patterns

### On startup (loadFromDB)
```sql
SELECT id, org_id, household_name, primary_email, primary_phone_e164, status, notes,
       created_at, updated_at, created_by::text, updated_by::text
FROM households WHERE deleted_at IS NULL
```

### On CREATE (upsert)
```sql
INSERT INTO households (id, org_id, household_name, primary_email, primary_phone_e164, status, notes, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::uuid, $9::uuid)
ON CONFLICT (id) DO UPDATE SET ...
```

## Notes
- Keep `ExportRuntimeSnapshot`/`LoadRuntimeSnapshot` for backward compat
- The snapshot middleware still fires but becomes redundant when pool is active
- Tests with nil pool use in-memory (existing behavior unchanged)
- Integration tests use pool with proper UUIDs
