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
	mu          sync.Mutex
	entries     map[string]*rateLimitEntry
	limit       int
	window      time.Duration
	trustedNets []*net.IPNet
	// nextSweep gates the full-map stale-entry sweep so it runs at most
	// once per sweepInterval instead of on every request; without this,
	// allow() would do an O(n) scan of every distinct client under the
	// global lock on every single HTTP request app-wide.
	nextSweep     time.Time
	sweepInterval time.Duration
}

type rateLimitEntry struct {
	count      int
	windowEnds time.Time
}

// NewRateLimiter returns a rate-limiting middleware. trustedProxyCIDRs is an
// optional list of CIDR ranges (e.g. "10.0.0.0/8", "192.168.1.1/32") whose
// requests may be trusted to provide a real client IP via X-Forwarded-For.
// When the list is empty, X-Forwarded-For is never trusted and the limiter
// always keys on the direct TCP peer address (RemoteAddr).
func NewRateLimiter(limit int, window time.Duration, trustedProxyCIDRs []string) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	var trustedNets []*net.IPNet
	for _, cidr := range trustedProxyCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		// Accept plain IPs as /32 (IPv4) or /128 (IPv6)
		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(cidr)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				cidr = ip.String() + "/32"
			} else {
				cidr = ip.String() + "/128"
			}
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil && ipNet != nil {
			trustedNets = append(trustedNets, ipNet)
		}
	}

	rl := &rateLimiter{
		entries:       make(map[string]*rateLimitEntry),
		limit:         limit,
		window:        window,
		trustedNets:   trustedNets,
		sweepInterval: window,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.clientAddress(r)
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

	if rl.nextSweep.IsZero() || now.After(rl.nextSweep) {
		for k, entry := range rl.entries {
			if now.After(entry.windowEnds) {
				delete(rl.entries, k)
			}
		}
		rl.nextSweep = now.Add(rl.sweepInterval)
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

// clientAddress returns the IP address to use as the rate-limit key.
// X-Forwarded-For is only trusted when the direct TCP peer (RemoteAddr)
// is within the configured trustedNets set. This prevents spoofing by
// external clients who can freely set the X-Forwarded-For header.
func (rl *rateLimiter) clientAddress(r *http.Request) string {
	remoteHost := remoteIP(r.RemoteAddr)

	if len(rl.trustedNets) > 0 && remoteHost != "" && isTrustedIP(remoteHost, rl.trustedNets) {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return forwarded
		}
	}

	if remoteHost != "" {
		return remoteHost
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

// remoteIP extracts the IP portion from an addr of the form "host:port" or
// plain "host".
func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

// isTrustedIP reports whether ip (a raw IP string) is contained within any of
// the provided CIDR networks.
func isTrustedIP(ip string, trustedNets []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range trustedNets {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}
