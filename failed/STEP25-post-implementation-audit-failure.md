# STEP 25 Post-Implementation Audit — Failure Report

**Date:** 2026-03-18
**Step:** STEP 25 — Observability & Support Readiness
**Audit Result:** CONDITION 1 NOT WELL MET

---

## Condition Results

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, no UI issues, no potential issues, no edge cases | ❌ FAIL |
| 2 | No regressions in code quality | ✅ PASS |
| 3 | No clear code security issues | ✅ PASS |
| 4 | All directions from STEP 25 followed and completed | ✅ PASS |

---

## What Went Wrong

### Condition 1 — High-Cardinality Prometheus Metric Labels

**File:** `server/internal/app/middleware/telemetry.go`, line 43

```go
route := r.URL.Path // use raw path; pattern matching adds noise in unit tests
```

The `PrometheusMetrics` middleware uses `r.URL.Path` (the raw request path) as the
`route` label for both the `dare_http_requests_total` counter and the
`dare_http_request_duration_seconds` histogram. This is acknowledged in the code
comment but creates a real production risk known as **Prometheus label cardinality
explosion**.

**Why this is a problem:**

Every unique URL path becomes a distinct label combination in Prometheus. The DARE
API serves paths like:

- `/api/v1/members/550e8400-e29b-41d4-a716-446655440000`
- `/api/v1/members/6ba7b810-9dad-11d1-80b4-00c04fd430c8`
- `/api/v1/imports/abc123.../rows`
- `/api/v1/meets/def456.../events/789.../entries`

With thousands of swimmers, hundreds of imports, and hundreds of meets — each with
its own UUID in the path — this generates **tens of thousands of unique time series**
in Prometheus. Each unique label set consumes memory in Prometheus permanently (it
never shrinks its TSDB head until compaction). In a production club with normal usage
patterns, this can easily OOM Prometheus within days of launch.

**Concrete example of cardinality explosion:**

```
# 500 swimmers × 10 route types = 5,000 unique /members/{id} series
# 200 imports × 5 route types = 1,000 unique /imports/{id} series
# 100 meets × 20 route types = 2,000 unique /meets/{id} series
# Total: ~8,000+ unique series from IDs alone
# Multiplied by method (GET/POST/PUT/DELETE) and status (200/400/404/500) = 32,000+
```

Prometheus documentation explicitly warns against using high-cardinality values as
label values. Cardinality > 10,000 distinct label combinations is considered
problematic; > 100,000 can cause OOMs.

**Same issue in the OTelTracing middleware:**

```go
// server/internal/app/middleware/telemetry.go, line 21-24
otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
    return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
}),
```

Span names also use raw URL paths, creating high-cardinality trace names. This
bloats the trace backend (Jaeger/Tempo) and makes trace search unreliable.

---

## How to Fix

### Fix the `PrometheusMetrics` middleware to use route patterns

Chi's `RouteContext` provides the matched route pattern (e.g., `/api/v1/members/{id}`
instead of `/api/v1/members/550e8400-...`). Use `chi.RouteContext` to get the
normalized pattern:

```go
// In PrometheusMetrics, replace:
route := r.URL.Path

// With:
import "github.com/go-chi/chi/v5"

routeCtx := chi.RouteContext(r.Context())
route := r.URL.Path // fallback
if routeCtx != nil && routeCtx.RoutePattern() != "" {
    route = routeCtx.RoutePattern()
}
```

**Important:** `chi.RouteContext` is only populated *after* the router has matched a
route. Since `PrometheusMetrics` is a top-level middleware that runs *before* routing,
the pattern is empty at the time the middleware wraps. The pattern is available
*after* the handler returns (which is when `PrometheusMetrics` records its metrics,
because it calls `next.ServeHTTP(wrapped, r)` first, then reads the status).

The fix is correct as written above because `route` is read **after**
`next.ServeHTTP(wrapped, r)` completes, at which point Chi has already matched and
set the route pattern in the context. This is standard practice for Chi telemetry.

### Fix the OTelTracing middleware span name formatter

```go
// In OTelTracing, replace:
otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
    return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
}),

// With:
otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
    routeCtx := chi.RouteContext(r.Context())
    path := r.URL.Path
    if routeCtx != nil && routeCtx.RoutePattern() != "" {
        path = routeCtx.RoutePattern()
    }
    return fmt.Sprintf("%s %s", r.Method, path)
}),
```

**Note:** For span names, the formatter runs during the span creation (when the
request arrives), so `RoutePattern()` may be empty at that point. The better approach
for OTel span names with Chi is to use `otelhttp.WithRouteTag` or update the span
name after routing completes using middleware placed *after* the router has run. An
alternative is to use the `otelchi` library
(`go.opentelemetry.io/contrib/instrumentation/github.com/go-chi/chi/v5/otelchi`)
which is purpose-built for Chi and correctly captures route patterns.

---

## What Counts as Passing

Condition 1 is considered **well-met** when:

1. The `route` label in `dare_http_requests_total` and `dare_http_request_duration_seconds`
   uses the Chi route **pattern** (e.g., `/api/v1/members/{memberID}`) rather than
   the raw URL path.
2. Prometheus cardinality stays bounded — running the full test suite and checking
   that the number of distinct label combinations in `/metrics` does not grow with
   the number of unique resource IDs used in tests.
3. OTel span names use route patterns or a strategy that avoids unbounded cardinality.
4. The existing test suite continues to pass after the fix (the comment said raw
   path "adds noise in unit tests" — but with the fix the tests should still pass
   since patterns like `/api/v1/members/{memberID}` are stable and test-friendly).

---

## Master List of Files Edited in STEP 25

All files created or modified during STEP 25 implementation:

### New Files Created
- `server/internal/telemetry/telemetry.go`
- `server/internal/telemetry/metrics.go`
- `server/internal/app/middleware/telemetry.go`
- `docs/observability/grafana/api-error-rates.json`
- `docs/observability/grafana/queue-depth-job-failures.json`
- `docs/observability/grafana/import-success-failure.json`
- `docs/observability/grafana/webhook-lag.json`
- `docs/observability/prometheus/alerts.yaml`
- `docs/runbooks/on-call.md`
- `docs/planning/STEP25-observability-plan.md`

### Modified Files
- `server/internal/app/middleware/logging.go` — enriched with org_id/user_id/trace_id/span_id
- `server/internal/workers/jobhandlers/imports.go` — added OTel tracing span
- `server/internal/workers/jobhandlers/billing.go` — added OTel tracing spans
- `server/internal/app/server.go` — wired OTel middleware, Prometheus metrics, /metrics endpoint
- `server/internal/app/config.go` — added TelemetryConfig struct and LoadConfig support
- `server/cmd/api/main.go` — wired telemetry.Setup() with shutdown flush
- `server/go.mod` — added OTel SDK, OTLP HTTP exporter, prometheus/client_golang, otelhttp deps
- `server/go.sum` — updated with new dependency hashes
- `TODO.md` — marked all STEP 25 tasks complete
- `CLAUDE.md` — updated STEP 25 status and observability runtime notes

---

## Additional Notes (Non-Blocking)

These are not failures but worth noting for improvement:

1. **`resource.New()` partial error handling:** When `resource.New()` returns a
   partial resource with a non-nil error (e.g., some OS detectors fail), the code
   falls back to a minimal resource and discards the partial result. Consider using
   the partial resource even on partial error:
   ```go
   if resErr != nil && res == nil {
       res = resource.NewSchemaless(...)
   }
   ```

2. **No unit tests for `server/internal/telemetry`:** The telemetry package has no
   direct tests. The no-op path is exercised implicitly by the server test suite,
   but a dedicated test for `LoadConfig()` and the no-op path would improve coverage.

3. **`/metrics` endpoint is unauthenticated:** Correctly documented in the code
   comment and CLAUDE.md, but this should be noted as a deployment prerequisite —
   firewall this path before go-live.
