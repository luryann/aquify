package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const requestIDHeader = "X-Request-Id"

// RequestID assigns each request an opaque random ID (no hostname prefix, unlike
// chi's middleware.RequestID). A sane incoming X-Request-Id is honoured. The ID
// is stored under chi's context key so chimiddleware.GetReqID keeps working,
// and echoed in the X-Request-Id response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validIncomingRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), chimiddleware.RequestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validIncomingRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
		if !ok {
			return false
		}
	}
	return true
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "req-unavailable"
	}
	return hex.EncodeToString(b)
}
