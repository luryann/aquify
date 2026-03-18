// Package stripe provides a Stripe API client interface, an in-memory stub
// implementation for testing and development, and standalone webhook signature
// verification. A production deployment should swap in a real client that
// wraps the official github.com/stripe/stripe-go/vN SDK.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrInvalidSignature = errors.New("stripe: invalid webhook signature")
	ErrExpiredTimestamp = errors.New("stripe: webhook timestamp too old (replay attack window exceeded)")
	ErrMalformedHeader  = errors.New("stripe: malformed Stripe-Signature header")
)

// ─── Types ────────────────────────────────────────────────────────────────────

// PaymentIntentParams holds parameters for creating a Stripe PaymentIntent.
type PaymentIntentParams struct {
	// AmountCents is the amount in the smallest currency unit (e.g. cents for USD).
	AmountCents int
	// Currency is the ISO 4217 currency code (e.g. "usd").
	Currency string
	// Description is an optional human-readable description.
	Description string
	// Metadata contains arbitrary key-value pairs; include "invoice_id" and
	// "org_id" so webhook handlers can route back to the correct invoice.
	Metadata map[string]string
}

// PaymentIntentResult is a simplified representation of a Stripe PaymentIntent.
type PaymentIntentResult struct {
	// ID is the Stripe PaymentIntent ID (pi_...).
	ID string `json:"id"`
	// ClientSecret is the client-side secret used to confirm the PaymentIntent.
	ClientSecret string `json:"client_secret"`
	// Status is the current PaymentIntent status.
	Status string `json:"status"`
	// Currency is the ISO 4217 currency code.
	Currency string `json:"currency"`
	// AmountCents is the amount in smallest currency units.
	AmountCents int `json:"amount_cents"`
}

// CheckoutSessionParams holds parameters for creating a Stripe Checkout Session.
type CheckoutSessionParams struct {
	// AmountCents is the line-item amount in smallest currency units.
	AmountCents int
	// Currency is the ISO 4217 currency code.
	Currency string
	// Description is the line-item description shown to the customer.
	Description string
	// Metadata contains arbitrary key-value pairs (same convention as PaymentIntent).
	Metadata map[string]string
	// SuccessURL is where Stripe redirects after successful payment.
	SuccessURL string
	// CancelURL is where Stripe redirects if the customer cancels.
	CancelURL string
}

// CheckoutSessionResult is a simplified representation of a Stripe Checkout Session.
type CheckoutSessionResult struct {
	// ID is the Stripe Checkout Session ID (cs_...).
	ID string `json:"id"`
	// URL is the hosted Checkout page URL. Redirect the customer here.
	URL string `json:"url"`
}

// WebhookEventData holds a parsed Stripe webhook event.
type WebhookEventData struct {
	// ID is the Stripe event ID (evt_...).
	ID string
	// Type is the Stripe event type (e.g. "payment_intent.succeeded").
	Type string
	// Created is the Unix timestamp of the event.
	Created int64
	// RawPayload is the original JSON bytes (used for storage in webhook_events).
	RawPayload json.RawMessage
	// Object is the parsed data.object field.
	Object map[string]any
	// Metadata is the data.object.metadata map (convenience accessor).
	Metadata map[string]string
}

// BalanceReport is a simplified response from Stripe's balance or charges API.
type BalanceReport struct {
	// TotalChargeCents is the net amount of succeeded charges in the query window.
	TotalChargeCents int
}

// ReconciliationReport compares the internal billing ledger against Stripe.
type ReconciliationReport struct {
	LedgerStripeTotalCents  int       `json:"ledger_stripe_total_cents"`
	StripeReportedCents     int       `json:"stripe_reported_cents"`
	MismatchCents           int       `json:"mismatch_cents"`
	HasMismatch             bool      `json:"has_mismatch"`
	AlertTriggered          bool      `json:"alert_triggered"`
	AlertThresholdCents     int       `json:"alert_threshold_cents"`
	GeneratedAt             time.Time `json:"generated_at"`
}

// ─── Client interface ─────────────────────────────────────────────────────────

// Client is the Stripe API surface used by this application. A production
// deployment should provide a real implementation backed by stripe-go. All
// financial-facing tests should inject a *StubClient.
type Client interface {
	// CreatePaymentIntent creates a new PaymentIntent on Stripe.
	CreatePaymentIntent(ctx context.Context, params PaymentIntentParams) (*PaymentIntentResult, error)

	// CreateCheckoutSession creates a Stripe Checkout Session.
	CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (*CheckoutSessionResult, error)

	// ParseWebhookEvent parses and (for real clients) verifies the webhook payload.
	// The sigHeader is the value of the "Stripe-Signature" HTTP header.
	// The webhookSecret is the endpoint signing secret (whsec_...).
	// Stub implementations skip signature verification.
	ParseWebhookEvent(payload []byte, sigHeader, webhookSecret string) (*WebhookEventData, error)

	// FetchBalanceCents returns the net succeeded charge total visible to Stripe.
	// Used by the reconciliation endpoint. Real clients query the Stripe Charges
	// or Balance Transactions API; the stub returns a configurable value.
	FetchBalanceCents(ctx context.Context) (int, error)
}

// ─── Webhook signature verification ──────────────────────────────────────────

// DefaultTimestampTolerance is the maximum age of a Stripe webhook timestamp
// before it is rejected to prevent replay attacks.
const DefaultTimestampTolerance = 5 * time.Minute

// VerifyStripeSignature verifies the Stripe-Signature header against the raw
// payload bytes and the webhook endpoint secret. It implements Stripe's
// documented HMAC-SHA256 scheme.
//
// If tolerance is zero, DefaultTimestampTolerance (5 min) is used.
// Returns ErrInvalidSignature, ErrExpiredTimestamp, or ErrMalformedHeader on failure.
func VerifyStripeSignature(payload []byte, sigHeader, webhookSecret string, tolerance time.Duration) error {
	if tolerance <= 0 {
		tolerance = DefaultTimestampTolerance
	}

	// Parse "t=<unix>,v1=<hmac>[,v0=<old>]" components.
	var timestamp int64
	var v1Sigs []string
	for _, part := range strings.Split(sigHeader, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "t="):
			t, err := strconv.ParseInt(part[2:], 10, 64)
			if err != nil {
				return ErrMalformedHeader
			}
			timestamp = t
		case strings.HasPrefix(part, "v1="):
			v1Sigs = append(v1Sigs, part[3:])
		}
	}

	if timestamp == 0 || len(v1Sigs) == 0 {
		return ErrMalformedHeader
	}

	// Enforce replay-attack window.
	if time.Since(time.Unix(timestamp, 0)) > tolerance {
		return ErrExpiredTimestamp
	}

	// Compute expected HMAC-SHA256 raw bytes: HMAC-SHA256(secret, "<timestamp>.<payload>").
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(signedPayload))
	expectedBytes := mac.Sum(nil)

	// Accept if any v1 entry matches (timing-safe comparison on raw bytes, not hex strings).
	// Decoding both sides to []byte guarantees constant-length inputs (32 bytes each)
	// so hmac.Equal's constant-time guarantee is preserved regardless of header content.
	for _, sig := range v1Sigs {
		sigBytes, decErr := hex.DecodeString(sig)
		if decErr != nil {
			continue // malformed hex — skip without short-circuiting
		}
		if hmac.Equal(sigBytes, expectedBytes) {
			return nil
		}
	}
	return ErrInvalidSignature
}

// parseWebhookPayload parses the JSON payload into a WebhookEventData struct.
func parseWebhookPayload(payload []byte) (*WebhookEventData, error) {
	var raw struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created int64  `json:"created"`
		Data    struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("stripe: failed to parse webhook payload: %w", err)
	}

	var obj map[string]any
	_ = json.Unmarshal(raw.Data.Object, &obj)

	// Extract metadata convenience map from data.object.metadata.
	meta := make(map[string]string)
	if m, ok := obj["metadata"]; ok {
		if mmap, ok := m.(map[string]any); ok {
			for k, v := range mmap {
				if s, ok := v.(string); ok {
					meta[k] = s
				}
			}
		}
	}

	cp := make([]byte, len(payload))
	copy(cp, payload)

	return &WebhookEventData{
		ID:         raw.ID,
		Type:       raw.Type,
		Created:    raw.Created,
		RawPayload: json.RawMessage(cp),
		Object:     obj,
		Metadata:   meta,
	}, nil
}

// ─── Stub client ─────────────────────────────────────────────────────────────

// StubClient is an in-memory Stripe client for testing and local development.
// It skips webhook signature verification and returns deterministic fake data.
// All methods are safe for concurrent use.
type StubClient struct {
	mu sync.Mutex

	// CreatePaymentIntentErr, if non-nil, is returned by CreatePaymentIntent.
	CreatePaymentIntentErr error
	// CreateCheckoutSessionErr, if non-nil, is returned by CreateCheckoutSession.
	CreateCheckoutSessionErr error
	// BalanceCents is returned by FetchBalanceCents. Defaults to 0.
	BalanceCents int

	// createdIntents records each PaymentIntent created for test assertions.
	createdIntents []*PaymentIntentResult
	// createdSessions records each CheckoutSession created for test assertions.
	createdSessions []*CheckoutSessionResult
}

// NewStubClient returns a new StubClient with default settings.
func NewStubClient() *StubClient {
	return &StubClient{}
}

// CreatedIntents returns a snapshot of all PaymentIntents created via this stub.
func (c *StubClient) CreatedIntents() []*PaymentIntentResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*PaymentIntentResult, len(c.createdIntents))
	copy(out, c.createdIntents)
	return out
}

// CreatePaymentIntent returns a fake PaymentIntentResult. If
// CreatePaymentIntentErr is set, it returns that error instead.
func (c *StubClient) CreatePaymentIntent(_ context.Context, params PaymentIntentParams) (*PaymentIntentResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CreatePaymentIntentErr != nil {
		return nil, c.CreatePaymentIntentErr
	}

	cur := params.Currency
	if cur == "" {
		cur = "usd"
	}

	id := "pi_stub_" + stubRandHex(8)
	result := &PaymentIntentResult{
		ID:           id,
		ClientSecret: id + "_secret_" + stubRandHex(8),
		Status:       "requires_payment_method",
		Currency:     cur,
		AmountCents:  params.AmountCents,
	}
	c.createdIntents = append(c.createdIntents, result)
	return result, nil
}

// CreateCheckoutSession returns a fake CheckoutSessionResult.
func (c *StubClient) CreateCheckoutSession(_ context.Context, params CheckoutSessionParams) (*CheckoutSessionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CreateCheckoutSessionErr != nil {
		return nil, c.CreateCheckoutSessionErr
	}

	id := "cs_stub_" + stubRandHex(8)
	result := &CheckoutSessionResult{
		ID:  id,
		URL: "https://checkout.stripe.com/pay/" + id,
	}
	c.createdSessions = append(c.createdSessions, result)
	return result, nil
}

// ParseWebhookEvent parses the payload without verifying the signature.
// This is intentional for testing — the stub accepts any (or empty) sigHeader.
func (c *StubClient) ParseWebhookEvent(payload []byte, _, _ string) (*WebhookEventData, error) {
	return parseWebhookPayload(payload)
}

// FetchBalanceCents returns the configured BalanceCents value (default 0).
func (c *StubClient) FetchBalanceCents(_ context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.BalanceCents, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func stubRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
