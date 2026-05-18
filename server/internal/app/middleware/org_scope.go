package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

type orgContextKey struct{}

func RequireOrgScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal == nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
			return
		}

		orgID := strings.TrimSpace(r.Header.Get("X-Org-Id"))
		if orgID == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing X-Org-Id header.", nil)
			return
		}
		if strings.TrimSpace(principal.OrgID) != orgID {
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Principal org_id must match X-Org-Id scope.", map[string]any{
				"header_org_id":    orgID,
				"principal_org_id": principal.OrgID,
			})
			return
		}

		ctx := context.WithValue(r.Context(), orgContextKey{}, orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func OrgIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(orgContextKey{}).(string)
	return orgID, ok
}
