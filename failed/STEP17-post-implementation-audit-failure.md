# STEP 17 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** STEP 17 — Background Jobs and Queues
**Result:** FAIL — All 4 conditions not well-met

---

## Condition Results

| Condition | Status | Reason |
|---|---|---|
| 1. No bugs, potential issues, or edge cases | ❌ FAIL | Multiple critical concurrency bugs and silent failure paths |
| 2. No code quality regressions | ❌ FAIL | Unbounded memory growth, misleading JSON tags, hidden side effects, dead test code |
| 3. No clear code security issues | ❌ FAIL | Silent task-drop with no DLQ visibility, permanently-invisible pending tasks |
| 4. All directions fully completed | ❌ FAIL | `exports.go` not a separate file per plan; STEP17 plan checkboxes not updated |

---

## What Went Wrong

### Condition 1: Bugs, Potential Issues, and Edge Cases

#### [CRITICAL] Data race on `s.handlers` map in `worker.go`
**File:** `server/internal/workers/worker.go` (~line 194)

`s.handlers` is a plain `map[string]Handler` that is written by `RegisterHandler` (which acquires `s.mu.Lock()`) but read in `enqueueAt` after the lock is released (`s.mu.RUnlock()` at ~line 176). Any concurrent `Enqueue` call after a late `RegisterHandler` call is an unguarded concurrent map read/write — a data race detectable by `go test -race`. The `s.retryPolicies` map has the same structural issue but is only written during construction, so it is safe in practice.

**How to fix:** Move the `s.handlers[task.Type]` lookup to occur *inside* the lock scope (before `s.mu.RUnlock()`), or capture the handler reference in a local variable inside the lock. Alternatively, use `sync.Map` for the handlers map.

**What counts as passing:** Running `go test -race ./server/...` produces zero race detector reports in the workers package.

---

#### [CRITICAL] `allTasks` map grows unbounded; no eviction (confidence: 90)
**File:** `server/internal/workers/worker.go` (~lines 213–215, 247–257)

Every task ever enqueued is stored permanently in `s.allTasks` except when explicitly deleted from the DLQ. Completed, failed, and archived tasks are never evicted. In a long-running server this results in unbounded memory growth. The `JobMetrics` rolling history cap of 500 demonstrates the design intended to avoid this, but `allTasks` has no equivalent limit or TTL eviction. `ListTasks` and `QueueStats` range over this map and grow proportionally slower over time.

**How to fix:** Add a completed-task eviction pass: after a task enters `StatusCompleted` or `StatusFailed`-and-DLQ, start a TTL timer (e.g., 1 hour) after which the entry is removed from `allTasks`. Alternatively, cap `allTasks` at a maximum size (e.g., 10,000 entries) with a FIFO eviction of oldest completed tasks. The DLQ list in `s.dlq` should retain DLQ entries until explicitly deleted; only non-DLQ completed/failed entries need eviction.

**What counts as passing:** In a stress test where 50,000 tasks are enqueued and completed, the length of `allTasks` does not grow past a documented cap.

---

#### [CRITICAL] Silent task-drop in `EnqueueIn` / delayed dispatch goroutine
**File:** `server/internal/workers/worker.go` (~lines 221–228)

The goroutine spawned for delayed tasks selects on `ctx.Done()` (the *caller's* context) and `time.After(delay)`. When the caller is an HTTP handler, the request context is cancelled when the response is sent — which happens before the delay elapses. The goroutine then exits via `<-ctx.Done()` without dispatching the task. The task is recorded in `allTasks` with `StatusPending` but is never transitioned to `StatusFailed` or moved to the DLQ. The operator sees a permanently-pending task with no error in the admin `/api/v1/jobs` endpoint, and it never surfaces for remediation.

**How to fix:** Replace the caller's `ctx` in the delayed goroutine with the server's internal shutdown context (`s.ctx` / `s.cancel`). Change the goroutine to:
```go
go func() {
    select {
    case <-s.ctx.Done():
        s.setStatus(it, StatusFailed)
        // move to DLQ with reason "server_shutdown"
    case <-time.After(delay):
        s.taskCh[task.Queue] <- it
    }
}()
```
This ensures tasks are either dispatched or properly failed on shutdown.

**What counts as passing:** A test enqueues a delayed task using a context that is immediately cancelled, waits for the delay to elapse, and confirms either the task ran OR the task appears in the DLQ with a failure reason — never permanently stuck in `StatusPending`.

---

#### [IMPORTANT] `UniqueKey` deduplication is declared but never implemented
**File:** `server/internal/workers/queue.go` (~lines 79–82), `server/internal/workers/worker.go` `enqueueAt`

`EnqueueOptions.UniqueKey` and `WithUniqueKey` are part of the public `Queue` interface and are documented as preventing duplicate enqueues of the same key. However, `enqueueAt` never reads `o.UniqueKey`. Any caller that uses `WithUniqueKey(...)` — such as the reconciliation scheduler to prevent duplicate nightly jobs — will silently enqueue duplicates.

**How to fix:** In `enqueueAt`, after acquiring `s.mu.Lock()`, check if `o.UniqueKey != ""` and scan `s.allTasks` for an existing `StatusPending` or `StatusRunning` task with the same `UniqueKey`. If found, return a well-typed `ErrDuplicateTask` error. Store the `UniqueKey` in `internalTask` so the lookup is O(1) if a secondary index map (`s.uniqueKeyIndex map[string]string`) is maintained alongside `s.allTasks`.

**What counts as passing:** A test enqueues the same task type with the same `UniqueKey` twice. The second call returns `ErrDuplicateTask` and `ListTasks` shows only one task.

---

#### [IMPORTANT] `processTask` reads `Retried`/`MaxRetries` without lock
**File:** `server/internal/workers/worker.go` (~lines 411–428)

`processTask` reads `it.Retried` and `it.MaxRetries` from `internalTask` without holding `s.mu`. `RetryDLQTask` writes `found.Retried = 0` under `s.mu.Lock()`. If an HTTP admin retry request and a concurrent worker processing the same task overlap, there is a read/write race on `it.Retried`.

**How to fix:** Read `it.Retried` and `it.MaxRetries` inside a `s.mu.RLock()` / `s.mu.RUnlock()` scope, or copy both values into local variables under the lock before using them in the retry decision logic.

**What counts as passing:** `go test -race ./server/...` shows no race on the `Retried` field under concurrent DLQ-retry and task-processing scenarios.

---

### Condition 2: Code Quality Regressions

#### No graceful shutdown: workers and scheduler goroutines are never stopped
**Files:** `server/cmd/api/main.go`, `server/internal/app/server.go` (~lines 131–148)

`main.go` calls `server.ListenAndServe()` with no OS signal handling, no `context.WithCancel`, and no `http.Server.Shutdown()`. The worker goroutines and scheduler goroutines started inside `NewRouterWithServices` are never stopped on SIGTERM. In-flight tasks are abandoned, the DLQ is not flushed, and tickers are not torn down. This is a significant regression because STEP 17 specifically added `Stop()` methods to `Server` and `Scheduler`, but they are never called.

**How to fix:**
```go
// In main.go
sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
go func() {
    <-sig
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    httpServer.Shutdown(ctx)
    // if workers are accessible, call workerServer.Stop() and scheduler.Stop()
}()
```
The `workerServer` and `scheduler` need to be returned from or accessible after `NewRouterWithServices` for the shutdown path to call `Stop()`.

**What counts as passing:** A test or integration check sends SIGTERM and verifies that the process exits cleanly with no goroutine leaks (all goroutines finish within the shutdown timeout).

---

#### `duration_ms` JSON tag stores nanoseconds
**File:** `server/internal/workers/metrics.go` (~line 33)

`JobExecution.Duration` is annotated with `json:"duration_ms"` in the struct tag but stores nanoseconds. API consumers reading the `/api/v1/jobs/metrics` endpoint will misinterpret nanosecond values as milliseconds (off by 1,000,000x).

**How to fix:** Either rename the field to `json:"duration_ns"`, or store milliseconds by converting `time.Since(start)` to `Milliseconds()` at write time and keeping the `duration_ms` tag. The latter is more API-friendly.

**What counts as passing:** The metric endpoint returns a `duration_ms` field that, when compared to actual wall-clock elapsed time, is within 10% of the expected millisecond value.

---

#### `TypeSnapshot` acquires write lock in a read-only path
**File:** `server/internal/workers/metrics.go` (~lines 96–106)

`TypeSnapshot` iterates known type keys under `RLock` then calls `m.counters(k)` for each. `counters()` always acquires `m.mu.Lock()` (a full write lock) even when the entry already exists and no write is needed. Under high task throughput this causes repeated full write-lock acquisitions in what should be a read-only snapshot path.

**How to fix:** Add a `countersRead(key string)` path that acquires only `m.mu.RLock()` and returns zero-value counters if the key is absent (since `TypeSnapshot` is only called for known-present keys). Use this in `TypeSnapshot` instead of `counters`.

**What counts as passing:** Under concurrent write traffic (`Record()` calls), `TypeSnapshot` holds a write lock for no longer than the duration of a single map lookup.

---

#### Dead test code: unused `srv` in `TestWorker_RetryOnError`
**File:** `server/internal/app/server_step17_test.go` (~lines 95–107)

`srv` is created and has a handler registered but is never started and never used. Only `srv2` is tested. The handler closure registered on `srv` also closes over the `count` variable from the outer scope, creating potential confusion about shared state.

**How to fix:** Remove the `srv` variable entirely. The test only needs `srv2`. Remove lines 95–107 and adjust `count` initialization to occur immediately before `srv2` is created.

**What counts as passing:** The test file contains no unused variable warnings from `go vet ./...`.

---

#### Starting goroutines inside a constructor is a hidden side effect
**File:** `server/internal/app/server.go` (~lines 131–148)

`NewRouterWithServices` calls `workerServer.Start()` and `scheduler.Start()` inside the constructor body. Any test that calls `NewRouterWithServices` (including prior-step tests that pass all-nil services) will spin up goroutines via the auto-wired worker server path. These goroutines are never stopped because the tests do not call any teardown. Over time, accumulated goroutine leaks in the test binary will cause flaky timing-sensitive tests.

**How to fix:** Move the `Start()` calls out of `NewRouterWithServices` and into `main.go` after the router is constructed. The constructor should be side-effect-free (pure initialization). `main.go` becomes responsible for calling `workerServer.Start()`, `scheduler.Start()`, and the shutdown teardown.

**What counts as passing:** `go test ./server/...` with `-count=3` shows no goroutine count increase between test runs (detectable via `runtime.NumGoroutine()` checks or `-v` goroutine leak detection).

---

### Condition 3: Security Issues

#### Silent task-drop creates permanently-invisible pending tasks
**File:** `server/internal/workers/worker.go` (~lines 221–228)

As described in Condition 1 (the delayed-task goroutine issue), tasks dropped due to a short-lived caller context are permanently stuck in `StatusPending` in `allTasks` with no error, no DLQ entry, and no log message. This means:
- The admin `/api/v1/jobs` endpoint reports these tasks as "pending" indefinitely.
- They never appear in the DLQ for operator inspection or retry.
- If the task was a compliance data export or data deletion job, the user waiting for their GDPR export or deletion will never receive it, with no indication of failure.

This is a reliability/compliance security gap: silent failure of regulated data operations with no operator visibility.

**How to fix:** See the fix described under Condition 1 (delayed task goroutine). Any path that causes a task not to be dispatched must transition the task to `StatusFailed` and/or move it to the DLQ with an explicit failure reason.

**What counts as passing:** No task can remain in `StatusPending` indefinitely. All non-dispatched tasks either complete or surface in the DLQ with an actionable error reason.

---

### Condition 4: Directions Fully Completed

#### `exports.go` not created as a separate file per the implementation plan
**Files:** `server/internal/workers/jobhandlers/imports.go`, STEP17 plan (~line 6)

The STEP17 implementation plan specifies that `GenerateExportHandler` should be in a separate file. Instead, it was placed inside `imports.go`. While the plan also misspells the path as `server/internal/workers/handlers/` (the actual path `jobhandlers/` is acceptable), co-locating the export handler with the import handler in the same file violates single-responsibility and deviates from the plan's intent.

**How to fix:** Create `server/internal/workers/jobhandlers/exports.go` containing `GenerateExportPayload`, `GenerateExportHandler`, and any exports-specific types. Remove these from `imports.go`.

**What counts as passing:** `ls server/internal/workers/jobhandlers/` shows a distinct `exports.go` file containing only export-related handler logic.

---

#### STEP17 implementation plan progress checkboxes not updated
**File:** `docs/planning/STEP17-implementation-plan.md` (~lines 67–81)

Every checkbox in the Progress Tracking section remains `[ ]` (unchecked), even though TODO.md marks STEP 17 as complete. This diverges from the established pattern of other steps and leaves the plan file misleadingly showing zero progress.

**How to fix:** Mark all completed checkboxes with `[x]` in `docs/planning/STEP17-implementation-plan.md`.

**What counts as passing:** All STEP 17 checkboxes in the implementation plan file are `[x]`.

---

## Master List of Files Edited/Created in STEP 17

| File | Action |
|---|---|
| `server/internal/workers/queue.go` | Created |
| `server/internal/workers/metrics.go` | Created |
| `server/internal/workers/worker.go` | Created |
| `server/internal/workers/scheduler.go` | Created |
| `server/internal/workers/jobhandlers/imports.go` | Created |
| `server/internal/workers/jobhandlers/email.go` | Created |
| `server/internal/workers/jobhandlers/billing.go` | Created |
| `server/internal/workers/jobhandlers/cleanup.go` | Created |
| `server/internal/app/handlers/jobs.go` | Created |
| `server/internal/app/server.go` | Modified (added workers wiring, `NewRouterWithServices` 15th param) |
| `server/internal/app/config.go` | Modified (added worker env var config) |
| `server/cmd/api/main.go` | Modified (wired worker server) |
| `server/internal/app/server_step17_test.go` | Created |
| `docs/planning/STEP17-implementation-plan.md` | Created |

---

## Priority Order for Remediation

1. **[P0]** Fix the delayed-task goroutine to use server context, not caller context (silent task-drop + security gap)
2. **[P0]** Fix the data race on `s.handlers` map (race detector failure)
3. **[P1]** Add graceful shutdown with `Stop()` calls in `main.go`
4. **[P1]** Fix `duration_ms` tag to actually store/return milliseconds
5. **[P1]** Implement `UniqueKey` deduplication or remove the dead interface
6. **[P1]** Add eviction/TTL for completed tasks in `allTasks`
7. **[P2]** Fix `processTask` lock gap on `Retried` field read
8. **[P2]** Fix `TypeSnapshot` write-lock in read-only path
9. **[P2]** Extract `exports.go` as a separate handler file
10. **[P3]** Remove dead `srv` from `TestWorker_RetryOnError`
11. **[P3]** Move `Start()` calls out of `NewRouterWithServices` constructor
12. **[P3]** Mark STEP17 plan progress checkboxes complete
