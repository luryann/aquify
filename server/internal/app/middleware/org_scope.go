package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/luryann/aquify/server/internal/app/httpx"
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
			if r.URL.Path == "/api/v1/auth/session" && strings.TrimSpace(principal.OrgID) != "" {
				ctx := context.WithValue(r.Context(), orgContextKey{}, strings.TrimSpace(principal.OrgID))
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
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
		observeOrg(r.Context(), orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type observedOrgKey struct{}

// observedOrg lets an outer middleware (metrics) learn which org a request was
// authenticated for: only orgs that passed the principal check are recorded, so
// a spoofed X-Org-Id header can never create metric series.
type observedOrg struct {
	mu  sync.Mutex
	org string
}

func observeOrg(ctx context.Context, orgID string) {
	if o, ok := ctx.Value(observedOrgKey{}).(*observedOrg); ok {
		o.mu.Lock()
		o.org = orgID
		o.mu.Unlock()
	}
}

func OrgIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(orgContextKey{}).(string)
	return orgID, ok
}
