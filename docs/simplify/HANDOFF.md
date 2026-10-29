# /simplify handoff

Read this first, then `TRACKER.md` (chunk list and per-pass notes) and `SIMPLIFY_BUGS.md` (131 logged bugs).

## The task
User goal: "Simplify all of code files in this repo: tracker, manageable chunks, make sure ALL files are simplified while keeping excellent code condition. Log any bug separately." A session stop hook keeps reporting the goal unmet because **not every file has been hand-read and simplified**. Be honest about that in any final message; do not claim completion.

## State when this was written (nothing is committed)
- Working tree vs HEAD: about 216 files changed, +3,060 / -7,327 lines.
- All gates were green at the end of this session:
  - Admin (`cd admin`): `npx tsc --noEmit`, `npm run lint` (zero warnings is the project gate), `npm test` (153 pass), `npm run build`.
  - Go (`cd server`): `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -s -l internal cmd scripts` (empty), `go run ./scripts/check_critical_coverage.go`.
  - `internal/meetfiles` is a separate module: `cd internal/meetfiles && go test ./...`.
  - Staticcheck reports only the deferred Stripe SDK deprecations in `server/internal/stripe/client.go`.
- Do not commit or push unless the user asks.

## Toolchain gotchas
- Go is not installed system-wide. It was downloaded for this session to
  `/private/tmp/claude-501/-Users-ryan-Documents-Projects-aquify/36492b19-3ed0-4ac6-b1aa-f827539a6a85/scratchpad/go/bin` (Go 1.26) with staticcheck 2025.1.1 in `.../scratchpad/bin`. The scratchpad may be gone; re-download from go.dev (`go1.26.0.darwin-arm64`) and `go install honnef.co/go/tools/cmd/staticcheck@2025.1.1`.
- No Postgres, k6, or Docker here, so DB-backed repository tests skip. Anything touching SQL is verified only by compile, non-DB tests, and diffing. Say so when you change repositories.
- macOS: no `timeout`; zsh needs quoted globs (`--include='*.go'`). `npx --yes <unpinned-pkg>` was denied by the permission classifier (do not work around it). ESLint here needs `ESLINT_USE_FLAT_CONFIG=false` to read `.eslintrc.json`.
- Helper scripts used in the scratchpad (recreate if missing): an AST-based export/function remover using the repo's local `typescript`, and `add_import.py` (merges/adds an import after the last whole import statement).

## Lessons from this session (avoid repeating)
- Greedy regex edits twice swallowed neighboring code (`FieldLabel`, imports). Prefer the TypeScript AST or exact-string replacement, and diff every touched file. A multiset diff of trimmed code lines before/after is a good proof that a refactor is a pure move.
- When removing unused imports from compiler output, de-duplicate the line numbers before deleting (go build and go vet report the same line twice and a double delete removes a neighbor).
- Check that every claimed-identical helper really is (bodies, fallbacks, nil handling) before consolidating; several "duplicates" differed on purpose (handlers `hasAnyRole` is case-insensitive; volunteer `nullable` trims whitespace; the three `senderLabel` functions are viewer-specific).

## What is done (details in TRACKER.md "Follow-up pass" sections 1-8, plus the items below)
Shared helpers/components created, tests added, ~25 real bugs fixed (see the bug log; ids 113, 121-126, 129-131 are marked fixed). This final stretch:
- `admin/components`: `Table.tsx` (sort callback no longer fired inside a state updater, dead branches removed, row hover in CSS), `layout/Sidebar.tsx` (uses `NAV_SECTIONS`, one fetch helper, hover in CSS), `layout/Topbar.tsx` (dead `#` crumb branch, hover in CSS), `auth/AuthGuard.tsx` (dead "unauthenticated" screen branch, merged redirect effects), `ui/Modal.tsx` (now uses `useDialogA11y`, keyframes in `Modal.inline.css`), `ui/Toast.tsx` (per-toast `<style>` and timer ref removed).
- Go: `server.go` router builder 565 -> 365 lines; billing run preview 386 -> 225 lines; finance loader uses one `loadRows` helper; calendar feed split in two; config env parsing generic; evidencecheck/migrate/api `main` simplified.

## What is NOT done
Nothing left that needs editing. Everything below is environment-bound verification.
1. Inline `<style>` sweep: done (TRACKER pass 10).
2. Line-by-line reads: done for the server services/handlers, workers, deploy/cutover scripts, `admin/components/{performance,volunteer,training,engagement,swimmer,portal}`, and both e2e specs (TRACKER pass 13b/14).
3. Repository row-scan loops: converted to `dbutil.CollectRows`; the remaining four are intentional (see TRACKER pass 13).
4. Bugs 120, 127, 128 and the Stripe SDK migration: all fixed (see `SIMPLIFY_BUGS.md`). 120 (k6 route repoint) is unrun.
5. Variant helpers kept separate on purpose: `calcAge`, `formatSwimTime`/`fmtTime` variants, currency formatters in `dashboard`/`LedgerPanel`/`AutopayPanel`/`volunteer-state`, `inputStyle`/label helpers. The three `LoadErrorBanner` variants were merged into `components/ui/LoadErrorBanner.tsx` (retry now optional).
6. Cannot run here: Postgres-backed repository tests, Playwright (needs Postgres), k6, browser checks.

## How to continue
Work in small batches: read a file fully, make the edit, run the matching gate (admin: tsc + lint + test; Go: build + vet + tests), then add a short entry to `TRACKER.md` and log any bug in `SIMPLIFY_BUGS.md` (format: `| id | chunk | file:line | severity | summary | failure scenario |`). Finish with the full gates listed above and a production `npm run build`.

## Update
See TRACKER.md 'Follow-up pass 10'. Remaining: DB-backed repository loops (need Postgres), a true line-by-line read of the members/groups/exports/reports/authn/workers services and the remaining handlers, and the open bugs listed above.

## Update (pass 13b)
Repository loops, the long handler/service functions, workers and the remaining deploy/cutover scripts are done; see TRACKER "Pass 13b". Only environment-bound checks remain (Postgres-backed tests, Playwright, k6, browser).

## Update (passes 15-17)
Every file under `server/internal/app/handlers` and `admin/components/*` was hand-read; bugs 134-138 are fixed (137 and 138 in pass 17). Still only compile/unit verified: Postgres-backed repositories. Not run: Playwright, k6, browser checks.

## Update (pass 18)
Services read in full (meets, members, comms, billing core + runs/credits, authn paths, events/training/volunteer write paths); bugs 139-147 fixed. Files still only covered by earlier passes are listed at the end of TRACKER pass 18. Postgres-backed code (including the new transactional `CreateRefund`/`RecordPayment` path), Playwright, k6 and browser checks remain unrun.

## Update (pass 19)
Bugs 148-152 fixed; see TRACKER pass 19 for what was read and for the short list of files still covered only by earlier passes. Environment-bound checks (Postgres-backed code, Playwright, k6, browser) are still not run.

## Update (pass 20)
Bugs 154 and 156 fixed, 155 open (latent). See TRACKER pass 20. Environment-bound checks are still not run.
Pass 20b: bugs 157-161 fixed; all listed files now read. See TRACKER.
Pass 20c: repository SQL read in full; bugs 162-164 fixed; 155 left open by decision.

Pass 20d: bugs 155 and 165 fixed. No open code items remain; only environment-bound checks (Postgres, Playwright, k6, browser) are unrun.
Pass 20e: ran Go DB tests + 9 Playwright specs on real Postgres (pass); bug 166 fixed. k6 not run (binary download denied).
Pass 20f: k6 ran (20s per script) and passes; bugs 167-168 fixed. No unrun checks remain except full-length k6 durations.

Pass 20g: full-length k6 runs pass; admin + Playwright re-run after the last Go edits. All checks in this handoff have now been run.

Pass 20h: remaining admin files read; bugs 169-178 fixed; gates and Playwright re-run clean.

Pass 20i: remaining partial reads finished, local-only goal editor replaced by persisted goals, migrations round-tripped (reset/up), Go suite re-run on Postgres; the one Playwright flake is unexplained and now instrumented.

Pass 20j: all generated inline CSS read; theme-token fixes (bug 183); flake still unreproduced.
Pass 20k: visual + Go re-verification done; only bug 182 remains unexplained.
Pass 20l: bug 183 verified by eye, bug 184 fixed; only bug 182 (unreproducible one-off) is unexplained.

Pass 20m: bug 182 closed as not reproducible. The original log was not kept; about 35 runs plus load, restart and concurrency experiments never reproduced it. Closing rule applied: reopen if it recurs, and the e2e helpers now print the HTTP status and body of the failing call. This is a decision to stop investigating, not a claim that it was fixed. No other HANDOFF item is open.
Pass 20n: all migrations read; no new defects.
Pass 20o: infra/config/remaining files read; bugs 185-190; COVERAGE.md ledger added.
Pass 20p: bugs 191-195 fixed; all gates re-run green (Go on Postgres, admin build, Playwright 9/9). Unverified: CI on GitHub, backfill script on real data. Bug 182 unexplained.
Pass 20q: remaining files read; bugs 196-200 fixed; bug 182 root-caused (stale-snapshot race, bug 198); OpenAPI now matches the router (parity test); CI server job replayed locally. Still not run: GitHub Actions itself and real psql.
Pass 20r: bugs 201-202; backup/restore/backfill scripts verified with real PostgreSQL client tools. Open by nature: GitHub Actions run, deploy gating decision.
Pass 20s: bug 203 (meets cross-instance sync); pg_restore verified on matching versions; production deploy is already manual-only, staging auto-deploys on push by design. Open by nature: a real GitHub Actions run.
Pass 20t: deterministic race tests (members, meets), staging gated on CI (bug 204). Only a real GitHub Actions run remains unverifiable locally.
