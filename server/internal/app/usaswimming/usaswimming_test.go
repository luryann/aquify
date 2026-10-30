package usaswimming

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsRedactAndEncrypt(t *testing.T) {
	svc := NewService(NewCipher("k"), nil)
	if _, err := svc.Update("o", Updates{BaseURL: ptr("http://x.example.com")}, "u"); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("http url: %v", err)
	}
	if _, err := svc.Update("o", Updates{Enabled: boolPtr(true)}, "u"); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("enable unconfigured: %v", err)
	}
	v, err := svc.Update("o", Updates{BaseURL: ptr("https://swims.example.com"), APIKey: ptr("abcd1234")}, "u")
	if err != nil || !v.HasAPIKey || v.APIKeyHint != "1234" || !v.Configured {
		t.Fatalf("%+v %v", v, err)
	}
	if raw, _ := svc.load("o"); strings.Contains(raw.APIKeyEnc, "abcd1234") || raw.APIKeyEnc == "" {
		t.Fatal("key not encrypted")
	}
	// No cipher: fail closed.
	if _, err := NewService(nil, nil).Update("o", Updates{APIKey: ptr("x")}, "u"); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("no cipher: %v", err)
	}
	// Stub base URL only with stub.
	if _, err := svc.Update("o", Updates{BaseURL: ptr("stub")}, "u"); err == nil {
		t.Fatal("stub should be rejected without a stub client")
	}
}

func TestHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/health":
		case "/swimmers/A1/times":
			_, _ = w.Write([]byte(`{"times":[{"distance":50,"stroke":"Free","course":"scy","time_ms":25000,"swim_date":"2026-01-02"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := NewHTTPClient(ClientConfig{BaseURL: srv.URL, APIKey: "good", AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
	ts, err := c.OfficialTimes(context.Background(), "A1")
	if err != nil || len(ts) != 1 || ts[0].Stroke != "free" || ts[0].Course != "SCY" {
		t.Fatalf("%+v %v", ts, err)
	}
	if _, err := c.OfficialTimes(context.Background(), "NOPE"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("404: %v", err)
	}
	bad, _ := NewHTTPClient(ClientConfig{BaseURL: srv.URL, APIKey: "bad", AllowLoopback: true})
	err = bad.TestConnection(context.Background())
	if !errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), "bad") && strings.Contains(err.Error(), "Bearer") {
		t.Fatalf("401: %v", err)
	}
}

func ptr(s string) *string { return &s }
func boolPtr(b bool) *bool { return &b }

func TestValidateBaseURLBlocksInternalTargets(t *testing.T) {
	for _, raw := range []string{
		"https://127.0.0.1", "https://localhost", "https://10.0.0.5", "https://169.254.169.254/latest",
		"https://192.168.1.2", "https://[::1]", "http://example.com", "https://example.com/?x=1", "https://example.com/#f",
	} {
		if err := ValidateBaseURL(raw, false); err == nil {
			t.Errorf("ValidateBaseURL(%q) accepted, want rejection", raw)
		}
	}
	if err := ValidateBaseURL("https://api.example.com/v1", false); err != nil {
		t.Errorf("public https URL rejected: %v", err)
	}
	if err := ValidateBaseURL("http://127.0.0.1:9000", true); err != nil {
		t.Errorf("loopback with allowLoopback rejected: %v", err)
	}
}

func TestAPIKeyHintCutsOnRuneBoundary(t *testing.T) {
	svc := NewService(NewCipher("k"), nil)
	v, err := svc.Update("o", Updates{BaseURL: ptr("https://swims.example.com"), APIKey: ptr("key-日本語キー")}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if v.APIKeyHint != "本語キー" {
		t.Fatalf("hint = %q", v.APIKeyHint)
	}
}

func TestOfficialTimesBadSwimDateLeavesZeroDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"times":[{"distance":50,"stroke":"free","course":"scy","time_ms":25000,"swim_date":"nope"}]}`))
	}))
	defer srv.Close()
	c, _ := NewHTTPClient(ClientConfig{BaseURL: srv.URL, APIKey: "good", AllowLoopback: true})
	ts, err := c.OfficialTimes(context.Background(), "A1")
	if err != nil || len(ts) != 1 || !ts[0].SwimDate.IsZero() {
		t.Fatalf("%+v %v", ts, err)
	}
}
