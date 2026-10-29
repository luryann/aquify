package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWaiverIPIgnoresClientSuppliedValues(t *testing.T) {
	h := &GroupsHandlers{}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "198.51.100.7:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := h.waiverIP(r); got != "198.51.100.7" {
		t.Fatalf("default resolver: got %q, want direct peer", got)
	}

	h.SetClientAddressResolver(func(*http.Request) string { return "203.0.113.50" })
	if got := h.waiverIP(r); got != "203.0.113.50" {
		t.Fatalf("wired resolver: got %q", got)
	}
}
