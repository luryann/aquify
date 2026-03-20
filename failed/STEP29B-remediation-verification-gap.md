# STEP29B Remediation Verification Gap

Date: 2026-03-20
Step: STEP 29B remediation follow-up
Status: NOT FULLY VERIFIED

## What Went Wrong

The Step 29B remediation itself now passes the targeted and broadened checks for the affected paths, but I could not complete a fully clean `go test ./...` run for the entire `server/` module in this sandboxed environment.

The blocker is an unrelated auth test:

- `server/internal/app/authn.TestOIDCCallbackRejectsTamperedIDTokenPayload`

That test fails here because `httptest.NewServer` cannot bind a local listener in the current sandbox:

- `listen tcp6 [::1]:0: bind: operation not permitted`

This is an environment limitation, not a Step 29B regression, but it still prevents me from claiming the post-change verification bar is fully and comfortably met across the whole server test suite.

## How To Fix It

1. Re-run `env GOCACHE=/Users/ryan/Documents/aquify/.gocache/go-build GOMODCACHE=/Users/ryan/Documents/aquify/.gomodcache go test ./...` from `/Users/ryan/Documents/aquify/server` in an environment that allows local listener binding for `httptest.NewServer`.
2. Confirm the authn package passes there without code changes.
3. If it still fails outside the sandbox, fix the auth test or auth implementation separately and then rerun the full suite.

## What Counts As Passing

This verification gap is closed only when all of the following are true:

1. The Step 29B-targeted checks still pass:
   - `go test ./internal/app`
   - `go test ./internal/workers/jobhandlers`
   - `go test ./...` from `/Users/ryan/Documents/aquify/internal/meetfiles`
2. A full `go test ./...` from `/Users/ryan/Documents/aquify/server` completes successfully in a non-restricted environment.
3. No additional regressions appear in the export, report-artifact, or cleanup paths during that full-suite run.
