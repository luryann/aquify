# STEP 8 — Meet Setup and Commitments: Implementation Notes

## Scope
STEP 8 implements the full meet lifecycle from creation through entry approval, covering:
- Database migrations: meets, meet_sessions, meet_events, commitments, entries, entry_legs
- Meet CRUD and lifecycle state machine
- Commitment workflow: invite swimmers → parent responds yes/no → parent selects events → lock
- Entry workflow: coach proposes entries → override if needed → approve
- Audit events for privileged actions (coach override, entry approval)
- RBAC enforcement throughout

## State Machines

### Meet
`draft → published → commitment_open → commitment_closed → entry_finalized → completed → archived`

- `draft`: Initial state when created
- `published`: Visible to coaches/parents, not yet open for commitments
- `commitment_open`: Parents can respond and select events
- `commitment_closed`: No more parent responses/selections
- `entry_finalized`: All entries approved, ready for export
- `completed`: Meet has occurred
- `archived`: Fully archived

Allowed transitions:
```
draft             → published
published         → commitment_open | draft
commitment_open   → commitment_closed
commitment_closed → entry_finalized
entry_finalized   → completed
completed         → archived
```

### Commitment
`invited → committed_yes | committed_no → coach_reviewed → locked`

- Parent responds yes/no (transitions from `invited`)
- Coach marks as reviewed (optional step)
- Lock happens at meet level (all commitments transition to `locked` when meet is locked)

### Entry
`draft → submitted → approved → exported → updated_from_results → archived`

- `draft`: Created by coach, not yet finalized
- `submitted`: Coach has finalized the entry
- `approved`: Approved for submission to meet manager
- Parent event selections are stored as `EventSelection` objects linked to a commitment, NOT as entries

## RBAC Permissions

| Action | owner | admin | coach | parent | billing | read-only |
|--------|-------|-------|-------|--------|---------|-----------|
| Create meet | Y | Y | Y | N | N | N |
| Create session/event | Y | Y | Y | N | N | N |
| List/get meets | Y | Y | Y | Y | Y | Y |
| Publish commitment request | Y | Y | Y | N | N | N |
| Transition meet status | Y | Y | Y | N | N | N |
| Commit yes/no | Y | Y | Y | Y (own household) | N | N |
| Select events | Y | Y | Y | Y (own household, pre-lock) | N | N |
| Remove event selection | Y | Y | Y | Y (own household, pre-lock) | N | N |
| Lock commitments | Y | Y | Y | N | N | N |
| List commitments | Y | Y | Y | Y (own household only) | N | N |
| Propose entry | Y | Y | Y | N | N | N |
| Override entry (coach) | Y | Y | Y | N | N | N |
| Approve entry | Y | Y | Y | N | N | N |
| List entries | Y | Y | Y | Y | N | Y |

## API Endpoints

```
POST   /meets                                                     create meet
GET    /meets                                                     list meets
GET    /meets/{meetID}                                            get meet
POST   /meets/{meetID}/sessions                                   create session
POST   /meets/{meetID}/events                                     create event
POST   /meets/{meetID}/transition                                 meet lifecycle transition
POST   /meets/{meetID}/commitment-request                         publish commitment request
GET    /meets/{meetID}/commitments                                list commitments
POST   /meets/{meetID}/commitments/{commitmentID}/respond         parent commit yes/no
POST   /meets/{meetID}/commitments/{commitmentID}/event-selections  select event(s)
DELETE /meets/{meetID}/commitments/{commitmentID}/event-selections/{selectionID}  remove selection
POST   /meets/{meetID}/lock-commitments                           lock all commitments
GET    /meets/{meetID}/entries                                    list entries
POST   /meets/{meetID}/entries                                    propose entry
POST   /meets/{meetID}/entries/{entryID}/override                 coach override
POST   /meets/{meetID}/entries/{entryID}/approve                  approve entry
```

## Migration Numbering
Continuing from 20260315001700:
- 20260315001800_create_meets.sql
- 20260315001900_create_meet_sessions.sql
- 20260315002000_create_meet_events.sql
- 20260315002100_create_commitments.sql
- 20260315002200_create_entries.sql
- 20260315002300_create_entry_legs.sql

## Implementation Files
- `db/migrations/20260315001800_create_meets.sql`
- `db/migrations/20260315001900_create_meet_sessions.sql`
- `db/migrations/20260315002000_create_meet_events.sql`
- `db/migrations/20260315002100_create_commitments.sql`
- `db/migrations/20260315002200_create_entries.sql`
- `db/migrations/20260315002300_create_entry_legs.sql`
- `server/internal/app/meets/service.go`
- `server/internal/app/handlers/meets.go`
- `server/internal/app/server.go` (updated)
- `server/internal/app/server_meets_test.go`

## Key Design Decisions
1. Event selections (parent-selected events) are stored as `EventSelection` records linked to a commitment. They are NOT entries. Entries are created by the coach via "propose entry".
2. The in-memory service uses seeded swimmer/household data (via SeedSwimmer/SeedParentHousehold) for scope enforcement, following the groups service pattern.
3. Entry state machine enforces: once entries are `approved` or beyond, they cannot be modified.
4. Lock rules: parent cannot add/remove event selections once the meet status is `commitment_closed` or beyond.
5. Coach override audit event captures: actor, action, before (event_id), after (new_event_id), override_reason, and entry snapshot.
6. Entry approval audit event captures: actor, action, entry_id, swimmer_id, meet_event_id.
