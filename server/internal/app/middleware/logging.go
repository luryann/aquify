package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
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
			// Auth runs on a derived request, so it reports the principal back
			// through this holder for the access log.
			holder := &loggedPrincipal{}
			r = r.WithContext(context.WithValue(r.Context(), loggedPrincipalKey{}, holder))

			next.ServeHTTP(writer, r)

			// Build the base attribute list.
			attrs := []any{
				"request_id", chimiddleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", loggedPath(r),
				"status", writer.Status(),
				"bytes", writer.BytesWritten(),
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"remote_addr", ClientIP(r),
				"user_agent", r.UserAgent(),
			}

			// Enrich with authenticated principal fields when available.
			if userID, orgID := holder.get(); userID != "" || orgID != "" {
				attrs = append(attrs, "user_id", userID, "org_id", orgID)
			} else if p, ok := PrincipalFromContext(r.Context()); ok {
				attrs = append(attrs, "user_id", p.UserID, "org_id", p.OrgID)
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

// loggedPath is the request path for the access log. Routes whose path carries
// a secret (the calendar feed token is itself the credential) log the route
// pattern instead, so the secret never reaches the logs.
func loggedPath(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if pattern := rc.RoutePattern(); strings.Contains(pattern, "{token}") {
			return pattern
		}
	}
	return r.URL.Path
}

type loggedPrincipalKey struct{}

// loggedPrincipal is filled by Auth so the outer access logger can attribute
// the request to a user and club.
type loggedPrincipal struct {
	mu            sync.Mutex
	userID, orgID string
}

func (h *loggedPrincipal) get() (string, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.userID, h.orgID
}

func recordLoggedPrincipal(ctx context.Context, p *Principal) {
	if p == nil {
		return
	}
	if h, ok := ctx.Value(loggedPrincipalKey{}).(*loggedPrincipal); ok {
		h.mu.Lock()
		h.userID, h.orgID = p.UserID, p.OrgID
		h.mu.Unlock()
	}
}
