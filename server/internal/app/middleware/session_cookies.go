package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const (
	AccessTokenCookieName  = "aquify_access_token"
	RefreshTokenCookieName = "aquify_refresh_token"
	CSRFTokenCookieName    = "aquify_csrf_token"
)

func NewCSRFToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(raw)
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
