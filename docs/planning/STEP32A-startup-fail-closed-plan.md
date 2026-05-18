# STEP 32A Startup Fail-Closed Plan

## Goal

Close the production startup fail-open gaps called out in `TODO.md` section `32A`.

## Acceptance Targets

- In non-development environments, API startup fails when `DATABASE_URL` is missing or unusable.
- In non-development environments, API startup fails when object storage config is missing or initialization fails.
- Startup validation emits actionable dependency-specific errors instead of silently falling back to in-memory DB or stub storage.

## Implementation Notes

- Keep dev/local/test fallback behavior intact so existing local workflows and tests are not broken.
- Add a dedicated startup dependency validator in the API app layer and wire it into `server/cmd/api/main.go` before router creation.
- Cover strict-env success/failure paths with focused unit tests that stub DB/storage initialization hooks.

## Verification

- Run targeted Go tests for startup validation and existing provider/schema startup coverage.
- Update `TODO.md` only after the strict-env checks are implemented and passing.
