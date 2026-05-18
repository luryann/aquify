# STEP 32B Durable Background Jobs Plan

## Goal

Finish `TODO.md` step `32B) Durable background job processing` by replacing the
process-local worker queue with a durable Redis-backed Asynq implementation for
production-style execution, while preserving the existing in-memory path for
unit tests and lightweight local flows.

## Current Gaps

- `server/internal/workers` only provides an in-memory `Server`.
- Jobs admin endpoints depend directly on `*workers.Server`, so queue stats,
  DLQ visibility, and retry/delete actions are process-local only.
- `server/cmd/api/main.go` always starts HTTP + workers together; the deployed
  `dare-worker` service command (`worker`) is not handled explicitly.
- There is no restart/recovery regression coverage proving queued work survives
  process replacement when using Redis.

## Implementation Outline

1. Add a Redis/Asynq-backed queue backend in `server/internal/workers`.
2. Introduce shared worker admin interfaces so HTTP handlers can read task
   state from either the in-memory server or the Redis-backed backend.
3. Extend app config with Redis-backed worker settings and wire them into
   startup/build logic.
4. Split API vs worker process modes in `server/cmd/api/main.go`.
5. Add tests for:
   - durable enqueue/list/metrics/DLQ adapter behavior
   - restart/recovery across two worker instances backed by the same Redis
     server
6. Update `TODO.md` and `CLAUDE.md` only after tests pass.

## Notes / Assumptions

- Keep dev/test defaults safe: if Redis is not configured, tests can keep using
  the in-memory backend unless the test explicitly exercises durability.
- Use Redis via Asynq for queue durability; do not introduce new memory-backed
  source-of-truth behavior.
- If module download or Redis-backed integration testing is blocked by the
  environment, document that clearly and keep the implementation otherwise
  complete.
