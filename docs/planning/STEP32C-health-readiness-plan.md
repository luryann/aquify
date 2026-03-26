# STEP 32C Health/Readiness Correctness Plan

## Goal

Finish `TODO.md` step `32C` by replacing the hard-coded `/api/v1/health/ready`
response with live dependency checks that reflect whether the API can serve
production traffic safely.

## Scope

- Add a reusable readiness checker in the app layer.
- Probe the dependencies this process actually relies on:
  - database connectivity when `DATABASE_URL` is configured
  - durable queue backend when Redis-backed workers are configured
  - object storage reachability when real storage is configured
  - external provider clients that are required outside explicit dev/test envs
- Return non-ready HTTP status when any critical dependency is unavailable.
- Add focused handler tests for success and failure paths.

## Assumptions

- `live` remains a shallow process health check.
- Dev/test fallback mode should still report stubbed dependencies as ready
  rather than forcing local infra for every test.
- Provider readiness should avoid side effects; use config/client validity and
  lightweight reachability checks instead of triggering business operations.

## Verification

- Run targeted Go tests for readiness handler behavior and startup coverage.
- Update `TODO.md` only after the readiness checks and failure-mode tests pass.
