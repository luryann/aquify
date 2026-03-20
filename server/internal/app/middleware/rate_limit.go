package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateLimitEntry
	limit   int
	window  time.Duration
}

type rateLimitEntry struct {
	count      int
	windowEnds time.Time
}

func NewRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	rl := &rateLimiter{
		entries: make(map[string]*rateLimitEntry),
		limit:   limit,
		window:  window,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientAddress(r)
			if !rl.allow(key, time.Now().UTC()) {
				w.Header().Set("Retry-After", "60")
				httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please try again shortly.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for k, entry := range rl.entries {
		if now.After(entry.windowEnds) {
			delete(rl.entries, k)
		}
	}

	entry, ok := rl.entries[key]
	if !ok || now.After(entry.windowEnds) {
		rl.entries[key] = &rateLimitEntry{
			count:      1,
			windowEnds: now.Add(rl.window),
		}
		return true
	}
	if entry.count >= rl.limit {
		return false
	}
	entry.count++
	return true
}

func clientAddress(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
