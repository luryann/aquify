# STEP 10 — Post-Implementation Audit Failure

**Audit date:** 2026-03-15
**Failing condition:** Condition 1 — Bugs, potential issues, or edge cases
**All other conditions:** Conditions 2, 3, and 4 all pass (see notes below).

---

## Conditions Summary

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | **FAIL** |
| 2 | No regressions in code quality | PASS |
| 3 | No clear code security issues | PASS |
| 4 | All directions followed and completed fully | PASS |

---

## What Went Wrong (Condition 1 Failure)

### Issue: `ErrCodeUnknownAthlete` is exported but never emitted — dead API surface inconsistent with the STEP 10 spec

**Location:** `internal/meetfiles/model.go` (line 133–134) and `internal/meetfiles/conflicts.go` (line 48)

Two error codes were defined in `model.go` that semantically overlap:

```go
// model.go
ErrCodeUnknownAthlete ErrorCode = "unknown_athlete"  // EXPORTED but NEVER USED
ErrCodeNoMatch        ErrorCode = "no_match"          // Used in conflicts.go
```

The STEP 10 implementation notes (the authoritative spec for this step) explicitly define the error classification table as:

| Code | Trigger |
|------|---------|
| `unknown_athlete` | No swimmer match found (no USS# hit, no name+DOB+gender match) |
| `ambiguous_match` | Multiple swimmers match the name+DOB+gender fallback |

However, `conflicts.go` emits `ErrCodeNoMatch` (`"no_match"`) for the no-match scenario instead of `ErrCodeUnknownAthlete` (`"unknown_athlete"`). The result:

1. **`ErrCodeUnknownAthlete` is dead exported code** — it is defined and exported but is never emitted by any function in the library. Callers who check for this code (as documented in the spec) will never see it.
2. **`ErrCodeNoMatch` is undocumented** — it is emitted by `BuildConflictQueue` but does not appear in the STEP 10 spec error classification table. It was added as a second code for the same semantic case without removing the first.
3. **The test in `conflicts_test.go`** checks for `ErrCodeNoMatch`, which masks the problem at test time but does not test for the spec-specified code.

This is a broken API contract. Any downstream caller (e.g. the import service, the conflict resolution UI, or any future code that switches on `ErrorCode`) that follows the spec and checks for `"unknown_athlete"` will never match, and the error will be silently mishandled.

---

## How to Fix It

There are two valid approaches. **Approach A is preferred** because it aligns with the spec:

### Approach A — Replace `ErrCodeNoMatch` with `ErrCodeUnknownAthlete` (spec-aligned)

1. **`internal/meetfiles/conflicts.go`, line 48:** Change `ErrCodeNoMatch` → `ErrCodeUnknownAthlete`

   ```go
   // Before
   Code: ErrCodeNoMatch,
   // After
   Code: ErrCodeUnknownAthlete,
   ```

2. **`internal/meetfiles/model.go`:** Remove `ErrCodeNoMatch` entirely (it serves no purpose once replaced).

   Delete lines:
   ```go
   // ErrCodeNoMatch means no swimmer matched after all strategies were tried.
   ErrCodeNoMatch ErrorCode = "no_match"
   ```

3. **`internal/meetfiles/conflicts_test.go`, line 59:** Update the assertion to check `ErrCodeUnknownAthlete`:

   ```go
   // Before
   if c0.Code != meetfiles.ErrCodeNoMatch {
   // After
   if c0.Code != meetfiles.ErrCodeUnknownAthlete {
   ```

### Approach B — Keep `ErrCodeNoMatch`, remove `ErrCodeUnknownAthlete`, update spec

This is acceptable only if the STEP 10 implementation notes are updated to reflect `no_match` instead of `unknown_athlete` throughout. This approach requires also updating `STEP10-implementation-notes.md`.

---

## What Counts as Passing

Condition 1 passes when:

1. Every `ErrorCode` constant exported from `model.go` is actually emitted by at least one function in the library (no dead exported error codes).
2. The error code emitted for the "no swimmer found" scenario matches the code documented in `docs/planning/STEP10-implementation-notes.md`.
3. Tests assert against the correct (spec-aligned) error code.
4. `go test -count=1 ./...` passes with no failures.
5. `go vet ./...` produces no output.

---

## Notes on Passing Conditions

### Condition 2 (Code quality) — PASS
All code is idiomatic Go, well-commented, and consistent with library conventions established in prior steps. `go vet` produces no output. The one defect is confined to a naming/wiring inconsistency, not a structural quality regression.

### Condition 3 (Security) — PASS
No SQL injection, no command injection, no path traversal, no format string vulnerabilities.
**Known deferred item:** `ReadZip` and `ReadHY3` use `io.ReadAll` with no decompression size cap, creating a potential ZIP bomb / memory exhaustion vector for adversarial inputs. This is explicitly a TODO item deferred to STEP 22 ("Add file upload size caps", "Add malware-scan integration hook for imports") and is the responsibility of the upload layer, not the parsing library. It does not constitute a new security issue introduced by STEP 10.

### Condition 4 (Directions followed) — PASS
All 30 STEP 10 TODO items are checked off and confirmed implemented:
- `internal/meetfiles` standalone module (`go.mod`, `model.go`)
- File type detector with `.sd3`, `.cl2`, `.zip`, `.hy3` support
- ZIP archive reader
- SD3/SDIF v3 fixed-width parser (A0/B1/C1/D0/E0/F0/G0/Z0 records)
- CL2 best-effort scaffold parser
- Swimmer matcher (USS# primary, name+DOB+gender fallback)
- Conflict queue generation (unknown athlete, ambiguous match, bad event code, duplicate row)
- Deterministic import report writer
- HY3 container-only ingest with deterministic SD3 > CL2 > lexicographic payload selection
- HY3 error codes: `hy3_empty_archive`, `hy3_no_parseable_payload`, `hy3_ambiguous_payload`
- HY3 normalized metadata capture (`SourceFile`, `SelectedPayload`, `AllPayloads`, `UnknownFiles`)
- HY3 format decision record (`docs/decisions/2026-03-15-hy3-format-handling.md`)
- 46 tests passing across all three packages

---

## Master List of Files Created/Modified in STEP 10

| File | Status |
|------|--------|
| `internal/meetfiles/go.mod` | Created |
| `internal/meetfiles/model.go` | Created |
| `internal/meetfiles/detector.go` | Created |
| `internal/meetfiles/zip.go` | Created |
| `internal/meetfiles/matcher.go` | Created |
| `internal/meetfiles/conflicts.go` | Created |
| `internal/meetfiles/report.go` | Created |
| `internal/meetfiles/hy3.go` | Created |
| `internal/meetfiles/hy3_test.go` | Created |
| `internal/meetfiles/matcher_test.go` | Created |
| `internal/meetfiles/conflicts_test.go` | Created |
| `internal/meetfiles/detector_test.go` | Created |
| `internal/meetfiles/zip_test.go` | Created |
| `internal/meetfiles/sdif/records.go` | Created |
| `internal/meetfiles/sdif/parser.go` | Created |
| `internal/meetfiles/sdif/parser_test.go` | Created |
| `internal/meetfiles/cl2/parser.go` | Created |
| `internal/meetfiles/cl2/parser_test.go` | Created |
| `internal/meetfiles/testdata/build_fixtures.go` | Created |
| `docs/decisions/2026-03-15-hy3-format-handling.md` | Created |
| `docs/planning/STEP10-implementation-notes.md` | Created |
