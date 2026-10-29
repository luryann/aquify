package middleware

import (
	"net/http/httptest"
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
