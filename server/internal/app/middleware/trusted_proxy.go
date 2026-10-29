package middleware

import (
	"context"
	"net/http"
	"strings"
)

type clientInfoKey struct{}

type clientInfo struct {
	ip     string
	secure bool
}

// TrustedProxy resolves the client IP and request scheme exactly once, honouring
// X-Forwarded-For and X-Forwarded-Proto only when the direct TCP peer is inside
// trustedProxyCIDRs. Mount it before logging, security headers and rate
// limiting; they read the result through ClientIP and RequestIsSecure instead of
// looking at forwarding headers themselves.
func TrustedProxy(trustedProxyCIDRs []string) func(http.Handler) http.Handler {
	rl := &rateLimiter{trustedNets: parseTrustedNets(trustedProxyCIDRs)}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := clientInfo{ip: rl.resolveClientAddress(r), secure: r.TLS != nil}
			if !info.secure && rl.peerTrusted(r) {
				info.secure = strings.EqualFold(strings.TrimSpace(firstForwarded(r.Header.Get("X-Forwarded-Proto"))), "https")
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientInfoKey{}, info)))
		})
	}
}

// ClientIP is the client address resolved by TrustedProxy. Without that
// middleware it falls back to the direct peer address (forwarding headers are
// never trusted).
func ClientIP(r *http.Request) string {
	if info, ok := r.Context().Value(clientInfoKey{}).(clientInfo); ok {
		return info.ip
	}
	return (&rateLimiter{}).resolveClientAddress(r)
}

// RequestIsSecure reports whether the request arrived over TLS, directly or via
// a trusted proxy. Without TrustedProxy only a direct TLS connection counts.
func RequestIsSecure(r *http.Request) bool {
	if info, ok := r.Context().Value(clientInfoKey{}).(clientInfo); ok {
		return info.secure
	}
	return r.TLS != nil
}

func firstForwarded(v string) string {
	return strings.Split(v, ",")[0]
}
