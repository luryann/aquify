package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

func CSRFProtection(headerName string) func(http.Handler) http.Handler {
	headerName = strings.TrimSpace(headerName)
	if headerName == "" {
		headerName = "X-CSRF-Token"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}

			if !RequestUsesCookieAuth(r) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CSRFTokenCookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				httpx.WriteError(w, r, http.StatusForbidden, "csrf_required", "Missing CSRF cookie.", nil)
				return
			}

			token := strings.TrimSpace(r.Header.Get(headerName))
			if token == "" {
				httpx.WriteError(w, r, http.StatusForbidden, "csrf_required", "Missing CSRF header.", nil)
				return
			}

			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) != 1 {
				httpx.WriteError(w, r, http.StatusForbidden, "csrf_invalid", "CSRF token validation failed.", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
