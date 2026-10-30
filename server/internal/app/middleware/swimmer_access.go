package middleware

import (
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

// SwimmerAccessChecker reports whether swimmer-role sign-in is blocked, either
// org-wide or for one swimmer.
type SwimmerAccessChecker interface {
	SwimmerAccessDisabled(orgID, swimmerID string) (bool, error)
}

// SwimmerAccessChecks is a func adapter for SwimmerAccessChecker.
type SwimmerAccessChecks func(orgID, swimmerID string) (bool, error)

// SwimmerAccessDisabled implements SwimmerAccessChecker.
func (f SwimmerAccessChecks) SwimmerAccessDisabled(orgID, swimmerID string) (bool, error) {
	return f(orgID, swimmerID)
}

// RequireSwimmerAccess answers 403 swimmer_access_disabled to swimmer-only
// principals whose org or linked swimmer has sign-in turned off. Admin, coach
// and parent principals (including mixed-role users) are never affected.
// It must run after Auth; /auth/* routes sit outside it so the UI can still
// read the session and log out.
func RequireSwimmerAccess(checker SwimmerAccessChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if checker == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal == nil || !IsSwimmerOnly(principal) {
				next.ServeHTTP(w, r)
				return
			}
			disabled, err := checker.SwimmerAccessDisabled(principal.OrgID, principal.SwimmerID)
			if err != nil {
				httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not verify swimmer access.", err)
				return
			}
			if disabled {
				httpx.WriteError(w, r, http.StatusForbidden, "swimmer_access_disabled", "Swimmer sign-in is turned off. Ask a parent or coach.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
