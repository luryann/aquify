package keycloak_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/keycloak"
)

func TestReadyProbesNetworkEveryCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
	}))
	c, err := keycloak.NewAdminClient(keycloak.AdminConfig{AdminURL: srv.URL, Realm: "r", ClientID: "c", ClientSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := c.Ready(context.Background()); err != nil {
			t.Fatalf("ready: %v", err)
		}
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected 2 token fetches, got %d", hits)
	}
	srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = c.Ready(ctx)
	if err == nil {
		t.Fatal("expected Ready to fail once keycloak is down, despite cached token")
	}
	if !keycloak.IsUnavailable(err) {
		t.Fatalf("transport failure should classify as unavailable: %v", err)
	}
}

func TestStatusErrorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/r/protocol/openid-connect/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"field":"email","errorMessage":"error-invalid-email"}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := keycloak.NewAdminClient(keycloak.AdminConfig{AdminURL: srv.URL, Realm: "r", ClientID: "c", ClientSecret: "s"})
	_, err := c.CreateUser(context.Background(), "x", "a", "b", "org")
	if !keycloak.IsBadRequest(err) || keycloak.IsUnavailable(err) {
		t.Fatalf("400 must be bad request, got %v", err)
	}
	_, err = c.ListUsers(context.Background(), "org", "", 10, 0)
	if !keycloak.IsUnavailable(err) || keycloak.IsBadRequest(err) {
		t.Fatalf("502 must be unavailable, got %v", err)
	}
}

func TestReadyDoesNotBlockAdminCalls(t *testing.T) {
	release := make(chan struct{})
	var slow int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/r/protocol/openid-connect/token" {
			// The first request (the Ready probe) hangs; later ones are fast.
			if atomic.AddInt32(&slow, 1) == 1 {
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	defer close(release)
	c, err := keycloak.NewAdminClient(keycloak.AdminConfig{AdminURL: srv.URL, Realm: "r", ClientID: "c", ClientSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	readyCtx, cancelReady := context.WithCancel(context.Background())
	readyDone := make(chan error, 1)
	go func() { readyDone <- c.Ready(readyCtx) }()
	for atomic.LoadInt32(&slow) == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.ListUsers(context.Background(), "org", "", 10, 0)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("list users: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admin call blocked behind a hanging readiness probe")
	}

	cancelReady()
	if err := <-readyDone; err == nil {
		t.Fatal("expected Ready to honour context cancellation")
	}
}
