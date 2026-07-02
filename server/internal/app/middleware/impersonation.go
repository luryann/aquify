package middleware

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

func RejectImpersonationHeaders(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targetUserID := strings.TrimSpace(r.Header.Get("X-Impersonate-User-Id"))
			reason := strings.TrimSpace(r.Header.Get("X-Impersonate-Reason"))
			if targetUserID == "" && reason == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !enabled {
				httpx.WriteError(w, r, http.StatusForbidden, "impersonation_disabled", "Admin impersonation is disabled in this environment.", nil)
				return
			}

			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal == nil {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
				return
			}
			if !principal.MFAAuthenticated {
				httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA step-up is required for impersonation.", nil)
				return
			}
			if strings.TrimSpace(reason) == "" {
				httpx.WriteError(w, r, http.StatusForbidden, "impersonation_reason_required", "Impersonation reason is required.", nil)
				return
			}
			if targetUserID == "" {
				httpx.WriteError(w, r, http.StatusForbidden, "impersonation_target_required", "Impersonation target user ID is required.", nil)
				return
			}

			allowed := false
			for _, role := range principal.Roles {
				if role == "owner" {
					allowed = true
					break
				}
			}
			if !allowed {
				httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only owners may impersonate users.", nil)
				return
			}

			httpx.WriteError(w, r, http.StatusForbidden, "impersonation_unavailable", "Admin impersonation remains unavailable until audited session-switch controls are fully implemented.", nil)
		})
	}
}
