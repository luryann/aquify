package stripe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	stripeapi "github.com/stripe/stripe-go/v84"
)

func TestSDKClientBoundedWhenStripeHangs(t *testing.T) {
	var hits int32
	var keys [2]string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := atomic.AddInt32(&hits, 1); n <= 2 {
			mu.Lock()
			keys[n-1] = r.Header.Get("Idempotency-Key")
			mu.Unlock()
		}
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	backend := stripeapi.GetBackendWithConfig(stripeapi.APIBackend, &stripeapi.BackendConfig{
		URL:               stripeapi.String(srv.URL),
		HTTPClient:        &http.Client{Timeout: 200 * time.Millisecond},
		MaxNetworkRetries: stripeapi.Int64(1),
	})
	c := newSDKClientWithBackend("sk_test_x", backend)
	start := time.Now()
	_, err := c.CreatePaymentIntent(context.Background(), PaymentIntentParams{AmountCents: 100, Currency: "usd", IdempotencyKey: "idem-1"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("call not bounded: %s", d)
	}
	if !IsTimeout(err) {
		t.Fatalf("expected timeout classification, got %T %v", err, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if atomic.LoadInt32(&hits) != 2 || keys[0] != "idem-1" || keys[1] != "idem-1" {
		t.Fatalf("retry must reuse idempotency key: hits=%d keys=%v", hits, keys)
	}
	// Ready honours a cancelled context.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start = time.Now()
	if err := c.Ready(ctx); err == nil || time.Since(start) > time.Second {
		t.Fatalf("ready not bounded by ctx: %v %s", err, time.Since(start))
	}
}

func TestNewSDKClientConfiguresBoundedBackend(t *testing.T) {
	if sdkHTTPTimeout > 20*time.Second || sdkMaxNetworkRetries > 1 {
		t.Fatal("SDK bounds too loose for 60s server write timeout")
	}
	if _, err := NewSDKClient("sk_test_x"); err != nil {
		t.Fatal(err)
	}
}
