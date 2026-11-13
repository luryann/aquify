# ADR: Meet file model — host event files in, entry bundle out

- Status: Accepted
- Date: 2026-11-10
- Supersedes: `2026-03-15-hy3-format-handling.md`, `2026-03-15-hy3-export-v1.md`

## Context

The first meet-file implementation was written before we had real files from a
host or a real entry export to compare against. It accepted `.sd3`, `.cl2`,
`.hy3` and `.zip` as meet definitions, treated `.hy3` as a ZIP container, and
exported a single SD3 entry file in two invented variants.

Real files show a different, simpler model:

- A host publishes its meet as two **meet event files**, a `.hyv` and a `.ev3`,
  usually zipped together. They describe the same meet; the `.ev3` additionally
  carries sessions, event order and start times.
- A club answers with an **entry bundle**: an SDIF `.sd3`, a Hy-Tek `.hy3`
  (fixed-width text, 128 data columns plus a two-digit checksum per line, not a
  ZIP), two PDF entry reports (by event and by swimmer) and a text entry log,
  all sharing the stem `<TEAM>-Entries-<Meet name>-<DDMonYYYY>-<HHMM>`.

## Decision

1. **Import** accepts only `.hyv`, `.ev3`, or a `.zip` of them
   (`meetfiles.ParseMeetEvents`). When both formats are supplied the `.ev3` is
   the base and the `.hyv` is used as a cross-check; disagreements are returned
   to the coach as warnings rather than silently resolved.
2. **Export** is one endpoint, `POST /meets/{id}/entries/export`, returning the
   whole bundle as a ZIP built by `internal/meetfiles/entries`.
3. Layouts are **pinned to a reference bundle**. `entries/reference_test.go`
   rebuilds that bundle from its own data and requires the SD3, HY3 and log to
   match byte for byte and the PDFs to match text-run for text-run. The
   reference files contain real swimmers' details, so they stay untracked
   (`meetfilesClaude/`) and the test skips without them; `entries_test.go`
   covers the same layouts with fictional data.
4. The **CL2 parser, the ZIP-style HY3 reader and the SD3 "standard/extended"
   exporter are removed.** Results import keeps the SDIF parser and accepts
   `.sd3` (or a ZIP holding one).
5. The bundle names **aquify** as the producing software. The HY3 `A1` vendor
   field stays `Hy-Tek, Ltd`, which is what the format carries for files meant
   for Hy-Tek meet software. Reports carry no third-party logos.

## Details worth knowing

- SD3 swimmers are ordered by last then first name in byte order; the
  by-swimmer report orders case-insensitively by "Last, First"; HY3 athletes
  are numbered in order of first appearance walking events in number order.
- The SD3 `Z0` record counts D0 and D3 records together and reports one more
  swimmer than there are athletes. This mirrors the reference file.
- SD3 writes the swimmer's age left-justified; HY3 writes it right-justified.
- PDF text is measured with the same Helvetica metrics the reference reports
  were laid out with (`entries/fontmetrics.go`, generated), so right-aligned
  and centred text lands on the same coordinates.
- `.hyv` column 10 / `.ev3` column 21 hold an event's entry time standard in
  short course yards and `.hyv` column 14 / `.ev3` column 17 the same standard
  in long course metres; `.hyv` column 12 / `.ev3` column 15 is the entry fee.
  These meanings were inferred from one host's files (the two columns differ by
  the usual yards-to-metres conversion) and should be re-checked against a
  second host before anything is enforced from them.

## Consequences

- New stored fields: club mailing address (`org_settings.address_*`), swimmer
  `middle_initial`, meet `swimmer_fee_cents` (migration `20260802000100`).
- Exports refuse swimmers without a male/female competition category or a date
  of birth instead of writing a record the host cannot import.
- Hosts that distribute results only as `.cl2` are not importable until a
  results path for that extension is added back (the file content is SDIF, so
  this is a detector change, not a new parser).
- Relay entries are still not exported.
