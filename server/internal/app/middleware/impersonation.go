package middleware

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

// RejectImpersonationHeaders refuses any request carrying admin-impersonation
// headers. Impersonation is not implemented (there is no audited session-switch
// path), so rejection is unconditional: there is deliberately no role, MFA or
// feature-flag branching to drift from the eventual real implementation.
func RejectImpersonationHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(r.Header.Get("X-Impersonate-User-Id")) == "" &&
				strings.TrimSpace(r.Header.Get("X-Impersonate-Reason")) == "" {
				next.ServeHTTP(w, r)
				return
			}
			httpx.WriteError(w, r, http.StatusForbidden, "impersonation_unavailable", "Admin impersonation is unavailable.", nil)
		})
	}
}
