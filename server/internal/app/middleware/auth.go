package middleware

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/luryann/aquify/server/internal/app/httpx"
)

type principalContextKey struct{}

type Principal struct {
	UserID           string   `json:"user_id"`
	Email            string   `json:"email,omitempty"`
	Name             string   `json:"name,omitempty"`
	OrgID            string   `json:"org_id,omitempty"`
	HouseholdID      string   `json:"household_id,omitempty"`
	SwimmerID        string   `json:"swimmer_id,omitempty"`
	Roles            []string `json:"roles"`
	MFAAuthenticated bool     `json:"mfa_authenticated"`
	// SessionStartedAt (unix seconds) is when the sign-in that produced this
	// session happened; refresh rotation carries it forward so the chain has an
	// absolute lifetime. Zero for sessions that predate the field.
	SessionStartedAt int64 `json:"session_started_at,omitempty"`
}

type PrincipalResolver interface {
	ResolvePrincipal(token string) (*Principal, bool)
}

// PrincipalResolverErr is optionally implemented by resolvers that can tell an
// unknown token (nil, nil) from an unavailable session store (non-nil error).
type PrincipalResolverErr interface {
	ResolvePrincipalErr(token string) (*Principal, error)
}

func Auth(resolver PrincipalResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, errCode, errMessage := extractAuthToken(r)
			if token == "" {
				httpx.WriteError(w, r, http.StatusUnauthorized, errCode, errMessage, nil)
				return
			}

			var (
				principal *Principal
				ok        bool
			)
			if resolverErr, isErr := resolver.(PrincipalResolverErr); isErr && resolver != nil {
				p, err := resolverErr.ResolvePrincipalErr(token)
				if err != nil {
					w.Header().Set("Retry-After", "1")
					httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Authentication temporarily unavailable.", err)
					return
				}
				principal, ok = p, p != nil
			} else if resolver != nil {
				principal, ok = resolver.ResolvePrincipal(token)
			}
			if !ok {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Bearer token is not recognized.", nil)
				return
			}

			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			recordLoggedPrincipal(ctx, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractAuthToken(r *http.Request) (string, string, string) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return "", "unauthorized", "Authorization header must use Bearer token format."
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			return "", "unauthorized", "Bearer token is required."
		}
		return token, "", ""
	}

	cookie, err := r.Cookie(AccessTokenCookieName)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value), "", ""
	}

	return "", "unauthorized", "Missing Authorization header or session cookie."
}

func ExtractAuthToken(r *http.Request) (string, string, string) {
	return extractAuthToken(r)
}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(*Principal)
	return principal, ok
}

// IsParentOnly reports whether the principal is a parent with no staff role,
// so household scoping applies.
func IsParentOnly(principal *Principal) bool {
	return HasAnyRole(principal, "parent") && !HasAnyRole(principal, "admin", "coach")
}

// IsSwimmerOnly reports whether the principal is a swimmer with no staff or
// parent role, so own-household scoping applies.
func IsSwimmerOnly(principal *Principal) bool {
	return HasAnyRole(principal, "swimmer") && !HasAnyRole(principal, "admin", "coach", "parent")
}

// HasAnyRole reports whether the principal holds at least one of the required
// roles (exact match). A nil principal holds no roles.
func HasAnyRole(principal *Principal, required ...string) bool {
	if principal == nil {
		return false
	}
	for _, role := range principal.Roles {
		if slices.Contains(required, role) {
			return true
		}
	}
	return false
}

// ContextWithPrincipal returns ctx carrying p, as Auth does after resolving a
// token. Intended for tests and for components that act on a principal's behalf.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}
