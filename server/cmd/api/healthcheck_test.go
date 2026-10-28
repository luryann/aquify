package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHealth(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		err := checkHealth(server.URL)
		server.Close()
		if (err == nil) != (status == http.StatusOK) {
			t.Fatalf("status %d: err=%v", status, err)
		}
	}
}
