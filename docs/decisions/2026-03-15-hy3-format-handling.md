# ADR: HY3 Format Handling Strategy

- Status: Accepted
- Date: 2026-03-15

## Context

HyTek Meet Manager exports swimmer entries and results in a `.hy3` file.  These
files are used by many swim clubs and will appear in the DARE import flow.

Two implementation strategies were considered:
1. **Container-only ZIP handling** — treat `.hy3` as a ZIP wrapper, extract the
   inner `.sd3` or `.cl2` payload, and route it through the existing parsers.
2. **First-class semantic parser** — build a dedicated parser that understands
   the HY3 container schema at a structural level (headers, metadata records,
   checksums).

## Decision

Use **container-only ZIP handling** for v1.

`.hy3` files are standard ZIP archives with a renamed extension.  Their inner
payloads are `.sd3` (SDIF) or `.cl2` (Colorado Timing Systems) files that the
existing parsers already handle.  A separate semantic parser offers no
incremental correctness benefit because all swim-data semantics live in the
inner payload, not the container layer.

Payload selection follows deterministic precedence:
1. SD3 files are preferred over CL2 files (more widely standardised).
2. When multiple files of the same type exist, the lexicographically earliest
   filename is selected and an `hy3_ambiguous_payload` error is flagged.

## Consequences

- The `internal/meetfiles` package gains a `ReadHY3` function and `FileTypeHY3`
  detector without requiring a new parser package.
- The import finalization endpoint must check `FileTypeHY3` and route through
  `ReadHY3` before dispatching to the SD3 or CL2 parser.
- HY3-specific packaging failures (empty archive, no parseable payload,
  ambiguous payload) are surfaced via dedicated error codes so operators can
  diagnose import problems without reading raw ZIP contents.
- If HyTek ever introduces `.hy3` files whose payloads are not SD3/CL2, this
  decision must be revisited and a first-class parser may be required.

## Alternatives Considered

- **First-class semantic parser**: rejected for v1 due to lack of a public
  specification and because all known `.hy3` samples are ZIP containers with
  standard SD3 or CL2 payloads.
- **Treat `.hy3` as `.zip` with no special handling**: rejected because the
  separate `FileTypeHY3` type is needed to correctly route audit metadata,
  surface HY3-specific error codes, and maintain per-source-format telemetry.

## References

- `internal/meetfiles/hy3.go`
- `internal/meetfiles/hy3_test.go`
- `internal/meetfiles/detector.go`
- `TODO.md` STEP 10 HY3 items
- `docs/planning/STEP10-implementation-notes.md`
