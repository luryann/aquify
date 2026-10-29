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

// SessionCookies issues and clears the session cookie trio. It is a per-router
// dependency (not package state) so tests can inject a failing CSRF token
// factory or a non-Secure cookie flag without affecting parallel routers.
type SessionCookies struct {
	// Insecure drops the Secure cookie attribute. The zero value is Secure; it
	// is only ever set for explicit local/dev/test environments so plain-HTTP
	// development can receive cookies.
	Insecure bool
	// NewCSRFToken mints the double-submit token; nil means the package NewCSRFToken.
	NewCSRFToken func() (string, error)
}

// NewToken mints a CSRF token with the configured factory.
func (c SessionCookies) NewToken() (string, error) {
	if c.NewCSRFToken != nil {
		return c.NewCSRFToken()
	}
	return NewCSRFToken()
}

func NewCSRFToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := csrfTokenRandRead(raw); err != nil {
		return "", fmt.Errorf("csrf token generation failed: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func RequestUsesCookieAuth(r *http.Request) bool {
	// An Authorization header always wins in extractAuthToken, so such a request
	// is never authenticated by ambient cookies. Browsers cannot attach that
	// header cross-site without a CORS preflight, so CSRF does not apply even
	// when unrelated cookies ride along (e.g. other localhost apps).
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	cookieHeader := strings.TrimSpace(r.Header.Get("Cookie"))
	return cookieHeader != ""
}

func (c SessionCookies) Set(w http.ResponseWriter, accessToken, refreshToken, csrfToken string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   !c.Insecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   !c.Insecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFTokenCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   !c.Insecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
	})
}

func (c SessionCookies) Clear(w http.ResponseWriter) {
	expired := time.Unix(0, 0).UTC()
	for _, name := range []string{AccessTokenCookieName, RefreshTokenCookieName, CSRFTokenCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: name != CSRFTokenCookieName,
			Secure:   !c.Insecure,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
			Expires:  expired,
		})
	}
}
