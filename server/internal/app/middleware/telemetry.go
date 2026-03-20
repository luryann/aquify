package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

// OTelTracing wraps the handler with OpenTelemetry HTTP instrumentation.
// The span name formatter is Chi-aware: before routing it returns the HTTP
// method only; once Chi has matched a route it returns "METHOD /route/pattern"
// (e.g. "GET /api/v1/members/{memberID}"). Because otelhttp may invoke the
// formatter both before and after the downstream chain, making the formatter
// itself Chi-aware ensures the final span name is always the bounded pattern
// form regardless of otelhttp version behaviour. PrometheusMetrics (inner
// middleware) also calls span.SetName as a belt-and-suspenders update.
func OTelTracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, serviceName,
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil && routeCtx.RoutePattern() != "" {
					return fmt.Sprintf("%s %s", r.Method, routeCtx.RoutePattern())
				}
				// Before routing completes the pattern is empty; return method
				// only. PrometheusMetrics updates the span name after routing.
				return r.Method
			}),
		)
	}
}

// PrometheusMetrics records per-request counters and latency histograms using
// the supplied *telemetry.Metrics instance. It uses the matched Chi route
// pattern (e.g. /api/v1/members/{memberID}) as the route label to prevent
// Prometheus cardinality explosion from unique resource IDs in URL paths.
// Unmatched/404 paths are recorded under the sentinel label "unmatched" so
// that scan or misconfigured-client traffic cannot grow the label cardinality
// (the actual path is still captured in the structured access log).
// It also updates the active OTel span name to the same bounded route pattern.
// Note: otelhttp records http.response.status_code automatically; this
// middleware does not duplicate that attribute.
func PrometheusMetrics(m *telemetry.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(wrapped, r)

			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}

			// Use the Chi route pattern (e.g. /api/v1/members/{memberID}) rather
			// than r.URL.Path. Chi populates RoutePattern in the context after
			// routing completes, which has already happened by the time
			// next.ServeHTTP returns. This prevents cardinality explosion from
			// the thousands of unique UUIDs that appear in production URL paths.
			// Unmatched paths (404s) use the sentinel "unmatched" rather than
			// the raw path to prevent cardinality growth from path-scan traffic.
			route := "unmatched"
			if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil && routeCtx.RoutePattern() != "" {
				route = routeCtx.RoutePattern()
			}
			statusStr := fmt.Sprintf("%d", status)

			m.HTTPRequestsTotal.WithLabelValues(r.Method, route, statusStr).Inc()
			m.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())

			// Update the OTel span name to the bounded route pattern. This update
			// happens before otelhttp ends the span (PrometheusMetrics is inner
			// relative to OTelTracing), so the trace backend records the route
			// pattern form. Combined with the Chi-aware SpanNameFormatter in
			// OTelTracing, this ensures correctness across otelhttp versions.
			span := trace.SpanFromContext(r.Context())
			if span.SpanContext().IsValid() {
				span.SetName(fmt.Sprintf("%s %s", r.Method, route))
			}
		})
	}
}
