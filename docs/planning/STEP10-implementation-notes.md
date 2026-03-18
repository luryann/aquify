# STEP 10 — CL2/SD3/ZIP Parsing Engine: Implementation Notes

## Scope
STEP 10 implements the `internal/meetfiles/` standalone Go library that handles:
- File type detection (extension + magic bytes)
- ZIP archive reader (extracts .sd3 and .cl2 entries)
- SD3/SDIF v3 parser (fixed-width, 162-byte records with CRLF)
- CL2 best-effort scaffold parser (proprietary format, no public spec)
- Shared normalized model for parsed records
- Swimmer matching: USS# first, then name+DOB+gender fallback
- Error classification: unknown_athlete, bad_event_code, duplicate_row, malformed_record, ambiguous_match
- Import conflict queue generation
- Deterministic import report writer
- Fixture tests for SD3, CL2, and malformed inputs

## Module Location
`internal/meetfiles/` at repo root, **standalone Go module** `github.com/dareaquatics/aquify/meetfiles`.
Not inside `server/` — this is an independent parsing library that the server imports.

## SDIF SD3 Format Reference
- Every record is exactly 160 data bytes + CRLF (162 bytes total)
- Record type is in positions 1-2 (0-indexed: `line[0:2]`)
- All column positions below are 1-indexed (as in spec); subtract 1 for Go slice start

### Key Records
| Record | Purpose |
|--------|---------|
| A0     | File description (file type: 01=entries, 02=results) |
| B1     | Meet info (name, dates, course) |
| C1     | Team ID record |
| D0     | Individual event result (main workhorse) |
| E0     | Relay event header |
| F0     | Relay swimmer (one per leg) |
| G0     | Splits |
| Z0     | File terminator |

### D0 Column Layout (1-indexed)
| Field | Start | Len | Go slice |
|-------|-------|-----|---------|
| Swimmer Name | 12 | 28 | [11:39] |
| USS# | 40 | 12 | [39:51] |
| DOB | 56 | 8 | [55:63] |
| Swimmer Gender | 66 | 1 | [65] |
| Event Gender | 67 | 1 | [66] |
| Event Distance | 68 | 4 | [67:71] |
| Stroke Code | 72 | 1 | [71] |
| Event Number | 73 | 4 | [72:76] |
| Age Group | 77 | 4 | [76:80] |
| Seed Time | 89 | 8 | [88:96] |
| Seed Course | 97 | 1 | [96] |
| Finals Time | 116 | 8 | [115:123] |
| Finals Course | 124 | 1 | [123] |
| Finals Heat | 129 | 2 | [128:130] |
| Finals Lane | 131 | 2 | [130:132] |
| Finals Place | 136 | 3 | [135:138] |

### B1 Column Layout (1-indexed)
| Field | Start | Len | Go slice |
|-------|-------|-----|---------|
| Meet Name | 12 | 30 | [11:41] |
| Meet Start Date | 122 | 8 | [121:129] |
| Meet End Date | 130 | 8 | [129:137] |
| Course | 150 | 1 | [149] |

### C1 Column Layout
| Field | Start | Len | Go slice |
|-------|-------|-----|---------|
| Team Code | 12 | 6 | [11:17] |
| Team Full Name | 18 | 30 | [17:47] |

### E0 Column Layout
| Field | Start | Len | Go slice |
|-------|-------|-----|---------|
| Relay Team Char | 12 | 1 | [11] |
| Team Code | 13 | 6 | [12:18] |
| Event Gender | 21 | 1 | [20] |
| Event Distance | 22 | 4 | [21:25] |
| Stroke | 26 | 1 | [25] |
| Event Number | 27 | 4 | [26:30] |
| Age Group | 31 | 4 | [30:34] |
| Seed Time | 46 | 8 | [45:53] |
| Finals Time | 73 | 8 | [72:80] |

### F0 Column Layout
| Field | Start | Len | Go slice |
|-------|-------|-----|---------|
| Team Code | 16 | 6 | [15:21] |
| Swimmer Name | 23 | 28 | [22:50] |
| USS# | 51 | 12 | [50:62] |
| DOB | 66 | 8 | [65:73] |
| Swimmer Gender | 76 | 1 | [75] |
| Finals Leg Order | 79 | 1 | [78] |
| Leg Time | 80 | 8 | [79:87] |

### Code Tables
**Stroke:** 1=free, 2=back, 3=breast, 4=fly, 5=im, 6=free_relay, 7=medley_relay
**Course:** 1 or S=SCM, 2 or Y=SCY, 3 or L=LCM
**Gender:** M=Male, F=Female, X=Mixed(relay)
**Time special values:** NT, NS, DNF, DQ, SCR (left-justified in 8-char field)
**Dates:** MMDDYYYY format, zero-filled

## CL2 Format
No public specification. Colorado Timing Systems (CTS) proprietary format.
- Scaffold implementation with best-effort detection
- Returns ErrCodeMalformedRecord for unrecognized lines
- Extensible record interface for future implementation once actual files are available

## File Structure
```
internal/meetfiles/
  go.mod                   - module github.com/dareaquatics/aquify/meetfiles
  model.go                 - shared normalized record model + error codes
  detector.go              - file type detection (extension + magic bytes)
  zip.go                   - ZIP reader (extracts .sd3/.cl2 entries)
  matcher.go               - swimmer matching (USS# first, name+DOB+gender fallback)
  conflicts.go             - import conflict queue generation
  report.go                - deterministic import report writer
  sdif/
    records.go             - SDIF record constants, column offsets, code tables
    parser.go              - SD3/SDIF line-by-line parser
    parser_test.go         - fixture tests: valid SD3, malformed SD3
  cl2/
    parser.go              - CL2 best-effort scaffold parser
    parser_test.go         - scaffold tests
  testdata/
    valid.sd3              - minimal valid SD3 with 2 individual results + relay
    malformed.sd3          - SD3 with bad record type and truncated line
    sample.cl2             - minimal CL2 scaffold fixture
```

## Error Classification
| Code | Trigger |
|------|---------|
| unknown_athlete | No swimmer match found (no USS# hit, no name+DOB+gender match) |
| bad_event_code | Event code/distance/stroke combo not in meet event definitions |
| duplicate_row | Same swimmer + event + heat/lane already seen |
| malformed_record | Line too short, invalid record type, bad date/time format |
| ambiguous_match | Multiple swimmers match the name+DOB+gender fallback |

## Matching Algorithm
1. If USS# is non-blank: exact match on USS# in roster → resolved
2. Else: normalize(LastName) + normalize(FirstName) + DOB + Gender → exact match → resolved
3. If zero matches: unknown_athlete conflict
4. If multiple matches on step 2: ambiguous_match conflict

## Import Report (deterministic)
- Sorted by line number ascending
- Summary: total_rows, matched_rows, error_rows, conflict_rows
- Per-error detail: line_number, error_code, error_message, raw_data

## Migration Numbering
No new migrations in STEP 10 — this is a parsing library only.

## Testing Strategy
- SD3 parser: valid file fixture → assert 2 individual results + 1 relay
- SD3 parser: malformed file → assert ParseErrors populated
- CL2 parser: scaffold fixture → assert no panic, ParseErrors may be present
- Matcher: exact USS# match, name+DOB fallback, no-match (unknown), multi-match (ambiguous)
- Conflict queue: generates conflict entries for unmatched/ambiguous results
- Report writer: deterministic output sorted by line number
