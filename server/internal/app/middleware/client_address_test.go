package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientAddressResolverTrustsForwardedForOnlyFromTrustedPeers(t *testing.T) {
	resolve := NewClientAddressResolver([]string{"10.0.0.0/8"})

	trusted := httptest.NewRequest("GET", "/", nil)
	trusted.RemoteAddr = "10.1.2.3:4444"
	trusted.Header.Set("X-Forwarded-For", "203.0.113.9, 10.1.2.3")
	if got := resolve(trusted); got != "203.0.113.9" {
		t.Fatalf("trusted peer: got %q, want forwarded client", got)
	}

	untrusted := httptest.NewRequest("GET", "/", nil)
	untrusted.RemoteAddr = "198.51.100.7:5555"
	untrusted.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := resolve(untrusted); got != "198.51.100.7" {
		t.Fatalf("untrusted peer: got %q, want the peer address", got)
	}
}

func TestClientAddressResolverIgnoresClientSuppliedForwardedForEntries(t *testing.T) {
	resolve := NewClientAddressResolver([]string{"10.0.0.0/8"})

	// The client sent "1.2.3.4"; the proxy chain appended the real peer and
	// an inner proxy. The leftmost entry must not become the rate-limit key.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.1.2.3:4444"
	req.Header.Add("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	req.Header.Add("X-Forwarded-For", "10.9.9.9")
	if got := resolve(req); got != "203.0.113.9" {
		t.Fatalf("got %q, want the first untrusted hop from the right", got)
	}
}

func TestTrustedProxyResolvesIPAndSchemeOnceForLoggingAndHSTS(t *testing.T) {
	var gotIP string
	var gotSecure bool
	h := TrustedProxy([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP, gotSecure = ClientIP(r), RequestIsSecure(r)
	}))
	run := func(remote, xff, proto string) {
		gotIP, gotSecure = "", false
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", xff)
		req.Header.Set("X-Forwarded-Proto", proto)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	run("10.1.2.3:1", "203.0.113.9", "https")
	if gotIP != "203.0.113.9" || !gotSecure {
		t.Fatalf("trusted peer: ip=%q secure=%v", gotIP, gotSecure)
	}
	run("198.51.100.7:1", "203.0.113.9", "https")
	if gotIP != "198.51.100.7" || gotSecure {
		t.Fatalf("untrusted peer must not be believed: ip=%q secure=%v", gotIP, gotSecure)
	}
}

func TestSecurityHeadersIgnoresForwardedProtoFromUntrustedPeer(t *testing.T) {
	handler := TrustedProxy([]string{"10.0.0.0/8"})(SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	hsts := func(remote string) string {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Header().Get("Strict-Transport-Security")
	}
	if hsts("198.51.100.7:1") != "" {
		t.Fatal("HSTS set from X-Forwarded-Proto of an untrusted peer")
	}
	if hsts("10.0.0.5:1") == "" {
		t.Fatal("HSTS missing for trusted proxy reporting https")
	}
	// Without the trusted-proxy layer forwarded headers are never believed.
	bare := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(bare, req)
	if bare.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS trusted forwarded proto with no proxy layer")
	}
}

func TestStructuredLoggerRecordsResolvedClientIP(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := TrustedProxy([]string{"10.0.0.0/8"})(StructuredLogger(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.1.2.3:1"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(buf.String(), `"remote_addr":"203.0.113.9"`) {
		t.Fatalf("log = %s", buf.String())
	}
}

// A client behind a trusted proxy can prepend its own X-Forwarded-Proto; only
// the entry the nearest trusted proxy appended (the rightmost) counts.
func TestTrustedProxyUsesRightmostForwardedProto(t *testing.T) {
	var secure bool
	h := TrustedProxy([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secure = RequestIsSecure(r)
	}))
	run := func(proto string) bool {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:1"
		req.Header.Set("X-Forwarded-Proto", proto)
		h.ServeHTTP(httptest.NewRecorder(), req)
		return secure
	}
	if run("https, http") {
		t.Fatal("spoofed leading https must not make the request secure")
	}
	if !run("http, https") {
		t.Fatal("rightmost https from the trusted proxy should be honoured")
	}
}
