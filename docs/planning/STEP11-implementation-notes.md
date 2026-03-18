# STEP 11 — SD3 and SDIF Export Workflows: Implementation Notes

## Scope

STEP 11 implements the SD3/SDIF export pipeline:

1. **SD3 Exporter Library** (`internal/meetfiles/sdif/exporter.go`) — standalone Go library for generating Standard and Extended SD3 entries files, SDIF roster export, and SDIF roster import.
2. **Export Service** (`server/internal/app/exports/service.go`) — in-memory stub service that generates SD3 content and tracks export jobs.
3. **Export Handlers** (`server/internal/app/handlers/exports.go`) — HTTP endpoints for triggering and downloading exports.
4. **HY3 Export Decision** (`docs/decisions/2026-03-15-hy3-export-v1.md`) — documents HY3 export as explicitly out-of-scope for v1.

## SD3 Format Reference (export-specific)

See `docs/planning/STEP10-implementation-notes.md` for column layouts. Key export columns:

### D0 (Individual Event Entry/Result) — export-relevant fields
| Field            | Col (1-idx) | Len | Notes |
|------------------|-------------|-----|-------|
| Record ID        | 1           | 2   | "D0"  |
| Org Code         | 3           | 1   | "1"   |
| Name (Last,First)| 12          | 28  | left-justified |
| USS#             | 40          | 12  | left-justified |
| Attach           | 52          | 1   | "A"   |
| DOB (MMDDYYYY)   | 56          | 8   | |
| Swimmer Gender   | 66          | 1   | M/F   |
| Event Gender     | 67          | 1   | M/F/X |
| Distance         | 68          | 4   | right-justified |
| Stroke Code      | 72          | 1   | 1-7   |
| Event Number     | 73          | 4   | left-justified |
| Age Group        | 77          | 4   | e.g. "1824" |
| Seed Time        | 89          | 8   | **Extended only**; "NT      " for standard |
| Seed Course      | 97          | 1   | **Extended only** |
| Finals Heat      | 129         | 2   | right-justified |
| Finals Lane      | 131         | 2   | right-justified |

### B1 (Meet Info) — export-relevant fields
| Field         | Col | Len |
|---------------|-----|-----|
| Meet Name     | 12  | 30  |
| Start Date    | 122 | 8   | MMDDYYYY |
| End Date      | 130 | 8   | MMDDYYYY |
| Course        | 150 | 1   | S=SCM, Y=SCY, L=LCM |

### C1 (Team) — export-relevant fields
| Field      | Col | Len |
|------------|-----|-----|
| Team Code  | 12  | 6   |
| Team Name  | 18  | 30  |

### D1 (Individual Administrative / Roster)
Same identity fields as D0: name at col 12, USS# at col 40, DOB at col 56, gender at col 66.

### F0 (Relay Leg)
| Field      | Col | Len |
|------------|-----|-----|
| Team Code  | 16  | 6   |
| Name       | 23  | 28  |
| USS#       | 51  | 12  |
| DOB        | 66  | 8   |
| Gender     | 76  | 1   |
| Leg Order  | 79  | 1   | finals order |
| Leg Time   | 80  | 8   |

## SD3 Variants

### Standard SD3 (export_type = "standard_sd3")
- Includes: A0, B1, C1, D0 (identity + event fields only), Z0
- Seed times in D0 are omitted (set to "NT      ")
- Used for entry submission when seeding is not required

### Extended SD3 (export_type = "extended_sd3")
- Includes everything in Standard plus:
  - Seed time at col 89-96
  - Seed course at col 97
- Used when seed times are available and required by the target meet management software

## SDIF Roster (export_type = "sdif_roster")
- File type code "5" in A0
- Contains A0, B1, C1, D1 records (one D1 per swimmer)
- D1 has same identity layout as D0 for the swimmer fields
- Used for exporting swimmer rosters to Hy-Tek / Meet Manager
- `ImportSDIFRoster()` parses D1 records for round-trip testing

## SDIF Time Format
- 8-char field in SD3
- Sub-minute: `"  ss.hh "` (2 spaces + 5-char time + 1 space)
- 1-9 minutes: `"m:ss.hh "` (7 chars + 1 trailing space)
- 10+ minutes: `"mm:ss.hh"` (exactly 8 chars)
- Special codes: `"NT      "` (left-justified, right-padded to 8)

## Export Line Ordering Rules
1. A0 (file description) — exactly one
2. B1 (meet info) — exactly one
3. C1 (team) — one per distinct team; host team first
4. D0 records — sorted by event_number (ascending), then swimmer last name
5. E0 + F0 records — relay entries sorted by event_number; legs in leg_order
6. Z0 (file terminator) — exactly one

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/api/v1/meets/{meetID}/exports` | Trigger export (standard_sd3, extended_sd3, sdif_roster) |
| GET    | `/api/v1/meets/{meetID}/exports` | List all exports for a meet |
| GET    | `/api/v1/exports/{exportID}` | Get export job details |
| GET    | `/api/v1/exports/{exportID}/download` | Download generated file |

## Export Request Body
```json
{
  "export_type": "standard_sd3",
  "meet_name": "Spring Invitational",
  "meet_start_date": "20260315",
  "meet_end_date": "20260316",
  "course": "SCY",
  "host_team_code": "DARE",
  "host_team_name": "DARE Aquatics",
  "entries": [
    {
      "swimmer_last_name": "Smith",
      "swimmer_first_name": "John",
      "swimmer_uss_number": "123456789012",
      "swimmer_dob": "02151999",
      "swimmer_gender": "M",
      "team_code": "DARE",
      "event_number": "1",
      "distance": 100,
      "stroke": "free",
      "event_gender": "M",
      "age_group": "1824",
      "course": "SCY",
      "seed_time_ms": 59100,
      "heat": 0,
      "lane": 0
    }
  ]
}
```

## Authorization
- Owner, admin, coach may trigger exports and download files
- All privileged actions are audit-logged

## Export State Machine
`pending → generating → ready | failed`

In the stub implementation, the transition is synchronous (pending → ready in a single call). In production, this would be an async Asynq job.

## HY3 Export
Explicitly out of scope for v1. HY3 is treated as import-only. See `docs/decisions/2026-03-15-hy3-export-v1.md`.

## File Structure

```
internal/meetfiles/
  sdif/
    exporter.go           — SD3 exporter (standard + extended + roster)
    exporter_test.go      — export tests + SDIF round-trip fixture tests

server/internal/app/
  exports/
    service.go            — in-memory export job service
  handlers/
    exports.go            — HTTP handlers for export endpoints
  server.go               — updated to register exports routes
  server_exports_test.go  — integration tests

docs/decisions/
  2026-03-15-hy3-export-v1.md  — HY3 export decision record
```

## Testing Strategy
- SD3 standard export: assert D0 records have "NT      " in seed time field
- SD3 extended export: assert D0 records have valid seed time in seed time field
- SDIF roster round-trip: export → import → assert same swimmer list
- Export validation: empty meet name → ValidationError
- Time encoding: sub-minute, 1-9 min, 10+ min, NT, blank
- Integration: POST /exports → 201 with id; GET /download → 200 with file content
- RBAC: parent role cannot trigger exports (403)
- Audit: export.generated audit event recorded on success
