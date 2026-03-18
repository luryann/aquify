package middleware

import (
	"net/http"
	"slices"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

func RBACStub(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || principal == nil {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
				return
			}

			for _, role := range principal.Roles {
				if slices.Contains(allowedRoles, role) {
					next.ServeHTTP(w, r)
					return
				}
			}

			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to access this resource.", map[string]any{
				"required_roles": allowedRoles,
				"user_roles":     principal.Roles,
			})
		})
	}
}
