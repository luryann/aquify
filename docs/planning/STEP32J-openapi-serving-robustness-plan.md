# STEP 32J: OpenAPI Serving Robustness Plan

## Goal
Make `/api/v1/openapi` available regardless of production runtime working directory or repo layout by serving a packaged OpenAPI artifact from within the Go module.

## Acceptance Criteria
- Replace the handler's runtime file-path dependency with an embedded or packaged artifact strategy.
- Add tests that prove the endpoint remains available outside the repository-root runtime layout.
- Add startup/check coverage that fails loudly if the packaged OpenAPI artifact becomes unavailable or empty.

## Implementation Notes
- Keep `docs/api/openapi.yaml` as the editable source of truth for humans and tooling.
- Package a copy of the spec inside `server/` so the built API binary can serve it without external files.
- Add a drift test that compares the packaged artifact to `docs/api/openapi.yaml` during repo tests.
- Add a startup validation helper so production boot fails before serving if the packaged spec is unexpectedly missing.

## Verification
- Run targeted Go tests covering the OpenAPI handler and startup validation.
- Review the final diff to confirm only `32J`-scoped files changed.
