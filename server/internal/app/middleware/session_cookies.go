package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	AccessTokenCookieName  = "aquify_access_token"
	RefreshTokenCookieName = "aquify_refresh_token"
	CSRFTokenCookieName    = "aquify_csrf_token"
)

var csrfTokenRandRead = rand.Read

// CSRFTokenFactory allows callers/tests to override CSRF token generation.
// Production code should leave this pointed at NewCSRFToken.
var CSRFTokenFactory = NewCSRFToken

func NewCSRFToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := csrfTokenRandRead(raw); err != nil {
		return "", fmt.Errorf("csrf token generation failed: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func RequestUsesCookieAuth(r *http.Request) bool {
	cookieHeader := strings.TrimSpace(r.Header.Get("Cookie"))
	return cookieHeader != ""
}

func SetSessionCookies(w http.ResponseWriter, accessToken, refreshToken, csrfToken string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFTokenCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
}

func ClearSessionCookies(w http.ResponseWriter) {
	expired := time.Unix(0, 0).UTC()
	for _, name := range []string{AccessTokenCookieName, RefreshTokenCookieName, CSRFTokenCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: name != CSRFTokenCookieName,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
			Expires:  expired,
		})
	}
}
