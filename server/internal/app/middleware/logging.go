package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

// StructuredLogger logs every HTTP request with request/timing metadata plus
// the authenticated principal's user_id, org_id, and the active trace_id
// (when an OTel span is present in the context).
func StructuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			writer := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(writer, r)

			// Build the base attribute list.
			attrs := []any{
				"request_id", chimiddleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", writer.Status(),
				"bytes", writer.BytesWritten(),
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			}

			// Enrich with authenticated principal fields when available.
			if p, ok := PrincipalFromContext(r.Context()); ok {
				attrs = append(attrs,
					"user_id", p.UserID,
					"org_id", p.OrgID,
				)
			}

			// Attach the active trace ID so log lines can be correlated with
			// distributed traces in Grafana/Jaeger/Tempo.
			if spanCtx := trace.SpanFromContext(r.Context()).SpanContext(); spanCtx.IsValid() {
				attrs = append(attrs,
					"trace_id", spanCtx.TraceID().String(),
					"span_id", spanCtx.SpanID().String(),
				)
			}

			logger.Info("http_request", attrs...)
		})
	}
}
