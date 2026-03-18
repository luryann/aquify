package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

type principalContextKey struct{}

type Principal struct {
	UserID           string   `json:"user_id"`
	OrgID            string   `json:"org_id,omitempty"`
	HouseholdID      string   `json:"household_id,omitempty"`
	Roles            []string `json:"roles"`
	MFAAuthenticated bool     `json:"mfa_authenticated"`
}

type PrincipalResolver interface {
	ResolvePrincipal(token string) (*Principal, bool)
}

func Auth(resolver PrincipalResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Missing Authorization header.", nil)
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authorization header must use Bearer token format.", nil)
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if token == "" {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Bearer token is required.", nil)
				return
			}

			var (
				principal *Principal
				ok        bool
			)
			if resolver != nil {
				principal, ok = resolver.ResolvePrincipal(token)
			}
			if !ok {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Bearer token is not recognized.", nil)
				return
			}

			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(*Principal)
	return principal, ok
}
