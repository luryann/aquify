package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error(
						"panic_recovered",
						"request_id", chimiddleware.GetReqID(r.Context()),
						"panic", fmt.Sprintf("%v", recovered),
						"stack", string(debug.Stack()),
					)
					httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Internal server error.", nil)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
