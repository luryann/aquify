package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
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

	trustedNets := parseTrustedNets(trustedProxyCIDRs)

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
			allowed, retryAfter := rl.allow(key, time.Now().UTC())
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please try again shortly.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// parseTrustedNets parses CIDR ranges (plain IPs become /32 or /128), skipping
// blank or invalid entries.
func parseTrustedNets(cidrs []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
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
			nets = append(nets, ipNet)
		}
	}
	return nets
}

// NewClientAddressResolver returns a function that resolves a request's client
// IP with the same trusted-proxy rule as the rate limiter: X-Forwarded-For is
// honoured only when the direct peer is inside trustedProxyCIDRs.
func NewClientAddressResolver(trustedProxyCIDRs []string) func(*http.Request) string {
	rl := &rateLimiter{trustedNets: parseTrustedNets(trustedProxyCIDRs)}
	return rl.resolveClientAddress
}

// allow reports whether the request may proceed and, when it may not, how
// long until the caller's current window resets.
func (rl *rateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
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
		return true, 0
	}
	if entry.count >= rl.limit {
		retryAfter := entry.windowEnds.Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}
	entry.count++
	return true, 0
}

// clientAddress returns the IP address to use as the rate-limit key.
// X-Forwarded-For is only trusted when the direct TCP peer (RemoteAddr)
// is within the configured trustedNets set. This prevents spoofing by
// external clients who can freely set the X-Forwarded-For header.
func (rl *rateLimiter) clientAddress(r *http.Request) string {
	if info, ok := r.Context().Value(clientInfoKey{}).(clientInfo); ok {
		return info.ip
	}
	return rl.resolveClientAddress(r)
}

// peerTrusted reports whether the direct TCP peer is a configured trusted proxy.
func (rl *rateLimiter) peerTrusted(r *http.Request) bool {
	remoteHost := remoteIP(r.RemoteAddr)
	return len(rl.trustedNets) > 0 && remoteHost != "" && isTrustedIP(remoteHost, rl.trustedNets)
}

// resolveClientAddress applies the trusted-proxy rule directly to the request.
func (rl *rateLimiter) resolveClientAddress(r *http.Request) string {
	remoteHost := remoteIP(r.RemoteAddr)

	if rl.peerTrusted(r) {
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
