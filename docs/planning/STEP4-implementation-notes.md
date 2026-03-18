# STEP 4 Implementation Notes

Date: 2026-03-15  
Owner: Codex session

Purpose: execute `TODO.md` section `4) API skeleton and cross-cutting middleware`.

## Checklist

- [x] Initialize Chi router with versioned API prefix
- [x] Add request ID middleware
- [x] Add structured logging middleware
- [x] Add panic recovery middleware
- [x] Add CORS middleware with explicit origins
- [x] Add JSON error response helper
- [x] Add validation error formatter
- [x] Add auth middleware stub
- [x] Add RBAC middleware stub
- [x] Add org scope middleware stub
- [x] Add route group for health endpoints
- [x] Add `/health/live` endpoint
- [x] Add `/health/ready` endpoint
- [x] Add OpenAPI serve endpoint stub

## Implementation Strategy

1. Create `server/internal/app` packages for router, middleware, handlers, and error helpers.
2. Wire middleware globally in a deterministic order:
   - request ID
   - structured logging
   - panic recovery
   - CORS
3. Keep health endpoints public and place them under `/api/v1/health/*`.
4. Add a protected stub route to exercise auth/RBAC/org middleware composition.
5. Verify with `go test ./...` and update docs/tracking files (`TODO.md`, `CLAUDE.md`).

## Notes

- `SysPrompts/SYSTEM.md` currently mentions STEP 1, but repository status and `TODO.md` show STEP 0–3 complete and STEP 4 next; proceeding with STEP 4 per current request.
- New API base path is `/api/v1`.
- Health endpoints:
  - `GET /api/v1/health/live`
  - `GET /api/v1/health/ready`
- OpenAPI stub endpoint:
  - `GET /api/v1/openapi`
- Stub-protected route for middleware verification:
  - `GET /api/v1/authz/ping`
  - requires `Authorization: Bearer <token>` and `X-Org-Id`.

## Verification

- `GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache go test ./...`
  - Result: pass (all packages compile; no test files yet).
