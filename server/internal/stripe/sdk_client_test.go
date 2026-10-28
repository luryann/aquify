package stripe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	stripeapi "github.com/stripe/stripe-go/v84"
)

type sdkTransport func(*http.Request) (*http.Response, error)

func (f sdkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSDKClientUsesOwnKeyAndCallerContext(t *testing.T) {
	originalKey := stripeapi.Key
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	for _, key := range []string{"sk_test_first", "sk_test_second"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			c, err := NewSDKClient(key)
			if err != nil {
				t.Fatal(err)
			}
			c.backend = stripeapi.GetBackendWithConfig(stripeapi.APIBackend, &stripeapi.BackendConfig{
				MaxNetworkRetries: stripeapi.Int64(0),
				HTTPClient: &http.Client{Transport: sdkTransport(func(r *http.Request) (*http.Response, error) {
					if r.Header.Get("Authorization") != "Bearer "+key {
						t.Errorf("wrong API credential: %s", r.Header.Get("Authorization"))
					}
					if r.Context().Value(contextKey{}) != "request" {
						t.Error("caller context discarded")
					}
					if err := r.Context().Err(); err != nil {
						return nil, err
					}
					body := `{}`
					if r.URL.Path == "/v1/balance_transactions" {
						body = `{"object":"list","data":[],"has_more":false}`
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})},
			})
			calls := []func(context.Context) error{
				c.Ready,
				func(ctx context.Context) error {
					_, err := c.CreatePaymentIntent(ctx, PaymentIntentParams{AmountCents: 100, Currency: "usd"})
					return err
				},
				func(ctx context.Context) error {
					_, err := c.CreateCheckoutSession(ctx, CheckoutSessionParams{AmountCents: 100, Currency: "usd"})
					return err
				},
				func(ctx context.Context) error {
					_, err := c.CreateRefund(ctx, RefundParams{PaymentIntentID: "pi_test", AmountCents: 100})
					return err
				},
				func(ctx context.Context) error { _, err := c.FetchBalanceCents(ctx); return err },
			}
			for _, call := range calls {
				if err := call(ctx); err != nil {
					t.Fatal(err)
				}
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				if err := call(canceled); !errors.Is(err, context.Canceled) {
					t.Errorf("expected cancellation, got %v", err)
				}
			}
			if stripeapi.Key != originalKey {
				t.Error("global Stripe credentials changed")
			}
		})
	}
}
