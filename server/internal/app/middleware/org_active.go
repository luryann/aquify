package middleware

import (
	"context"
	"net/http"

	"github.com/luryann/aquify/server/internal/app/httpx"
)

// OrgStatusChecker answers whether an org may currently serve requests.
type OrgStatusChecker interface {
	IsActive(ctx context.Context, orgID string) (bool, error)
}

// RequireActiveOrg rejects requests from principals whose org is suspended,
// offboarding, deleted or unregistered (tracker items 2.3 / 7.1). It must run
// after Auth and RequireOrgScope. A nil checker disables the gate.
func RequireActiveOrg(checker OrgStatusChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if checker == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, ok := OrgIDFromContext(r.Context())
			if !ok || orgID == "" {
				httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing org scope.", nil)
				return
			}
			active, err := checker.IsActive(r.Context(), orgID)
			if err != nil {
				httpx.WriteError(w, r, http.StatusServiceUnavailable, "org_status_unavailable", "Could not verify organization status.", err)
				return
			}
			if !active {
				httpx.WriteError(w, r, http.StatusForbidden, "org_unavailable", "This organization is not available.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
