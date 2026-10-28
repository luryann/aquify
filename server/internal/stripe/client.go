// Package stripe provides a Stripe API client interface, an in-memory stub,
// and a production client backed by Stripe's official Go SDK.
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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	stripeapi "github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/balancetransaction"
	"github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/paymentintent"
	striperefund "github.com/stripe/stripe-go/v84/refund"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrInvalidSignature = errors.New("stripe: invalid webhook signature")
	ErrExpiredTimestamp = errors.New("stripe: webhook timestamp too old (replay attack window exceeded)")
	ErrMalformedHeader  = errors.New("stripe: malformed Stripe-Signature header")
	ErrInvalidConfig    = errors.New("stripe: missing secret key")
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
	// IdempotencyKey deduplicates retries for the same logical payment creation request.
	IdempotencyKey string
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
	// IdempotencyKey deduplicates retries for the same logical checkout creation request.
	IdempotencyKey string
}

// CheckoutSessionResult is a simplified representation of a Stripe Checkout Session.
type CheckoutSessionResult struct {
	// ID is the Stripe Checkout Session ID (cs_...).
	ID string `json:"id"`
	// URL is the hosted Checkout page URL. Redirect the customer here.
	URL string `json:"url"`
}

// RefundParams holds parameters for creating a Stripe refund.
type RefundParams struct {
	PaymentIntentID string
	AmountCents     int
	Reason          string
	IdempotencyKey  string
}

// RefundResult is a simplified representation of a Stripe refund object.
type RefundResult struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	AmountCents int    `json:"amount_cents"`
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
	LedgerStripeTotalCents int       `json:"ledger_stripe_total_cents"`
	StripeReportedCents    int       `json:"stripe_reported_cents"`
	MismatchCents          int       `json:"mismatch_cents"`
	HasMismatch            bool      `json:"has_mismatch"`
	AlertTriggered         bool      `json:"alert_triggered"`
	AlertThresholdCents    int       `json:"alert_threshold_cents"`
	GeneratedAt            time.Time `json:"generated_at"`
}

// ─── Client interface ─────────────────────────────────────────────────────────

// Client is the Stripe API surface used by this application. A production
// deployment should provide a real implementation backed by stripe-go. All
// financial-facing tests should inject a *StubClient.
type Client interface {
	// Ready performs a lightweight authenticated probe against Stripe.
	Ready(ctx context.Context) error

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

	// CreateRefund creates a Stripe refund against a succeeded payment intent.
	CreateRefund(ctx context.Context, params RefundParams) (*RefundResult, error)
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
	if strings.TrimSpace(webhookSecret) == "" {
		return ErrInvalidSignature
	}
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
	// CreateRefundErr, if non-nil, is returned by CreateRefund.
	CreateRefundErr error
	// BalanceCents is returned by FetchBalanceCents. Defaults to 0.
	BalanceCents int

	// createdIntents records each PaymentIntent created for test assertions.
	createdIntents []*PaymentIntentResult
	// createdSessions records each CheckoutSession created for test assertions.
	createdSessions []*CheckoutSessionResult
	// createdRefunds records each refund request handled by the stub.
	createdRefunds []RefundParams
	// intentResultsByKey deduplicates create-intent retries by idempotency key.
	intentResultsByKey map[string]*PaymentIntentResult
	// sessionResultsByKey deduplicates create-session retries by idempotency key.
	sessionResultsByKey map[string]*CheckoutSessionResult
}

// NewStubClient returns a new StubClient with default settings.
func NewStubClient() *StubClient {
	return &StubClient{
		intentResultsByKey:  make(map[string]*PaymentIntentResult),
		sessionResultsByKey: make(map[string]*CheckoutSessionResult),
	}
}

// Ready reports the stub as available for local/test environments.
func (c *StubClient) Ready(_ context.Context) error {
	return nil
}

// CreatedIntents returns a snapshot of all PaymentIntents created via this stub.
func (c *StubClient) CreatedIntents() []*PaymentIntentResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*PaymentIntentResult, len(c.createdIntents))
	copy(out, c.createdIntents)
	return out
}

// CreatedSessions returns a snapshot of all CheckoutSessions created via this stub.
func (c *StubClient) CreatedSessions() []*CheckoutSessionResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*CheckoutSessionResult, len(c.createdSessions))
	copy(out, c.createdSessions)
	return out
}

// CreatedRefunds returns a snapshot of all refund requests created via this stub.
func (c *StubClient) CreatedRefunds() []RefundParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]RefundParams, len(c.createdRefunds))
	copy(out, c.createdRefunds)
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
	key := strings.TrimSpace(params.IdempotencyKey)
	if key != "" {
		if existing, ok := c.intentResultsByKey[key]; ok {
			return existing, nil
		}
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
	if key != "" {
		c.intentResultsByKey[key] = result
	}
	return result, nil
}

// CreateCheckoutSession returns a fake CheckoutSessionResult.
func (c *StubClient) CreateCheckoutSession(_ context.Context, params CheckoutSessionParams) (*CheckoutSessionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CreateCheckoutSessionErr != nil {
		return nil, c.CreateCheckoutSessionErr
	}

	key := strings.TrimSpace(params.IdempotencyKey)
	if key != "" {
		if existing, ok := c.sessionResultsByKey[key]; ok {
			return existing, nil
		}
	}

	id := "cs_stub_" + stubRandHex(8)
	result := &CheckoutSessionResult{
		ID:  id,
		URL: "https://checkout.stripe.com/pay/" + id,
	}
	c.createdSessions = append(c.createdSessions, result)
	if key != "" {
		c.sessionResultsByKey[key] = result
	}
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

// CreateRefund returns a fake RefundResult.
func (c *StubClient) CreateRefund(_ context.Context, params RefundParams) (*RefundResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CreateRefundErr != nil {
		return nil, c.CreateRefundErr
	}

	c.createdRefunds = append(c.createdRefunds, params)
	return &RefundResult{
		ID:          "re_stub_" + stubRandHex(8),
		Status:      "succeeded",
		AmountCents: params.AmountCents,
	}, nil
}

// SDKClient is a production Stripe client backed by stripe-go.
type SDKClient struct {
	secretKey string
	backend   stripeapi.Backend
}

// NewSDKClient returns a Stripe SDK-backed client.
func NewSDKClient(secretKey string) (*SDKClient, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, ErrInvalidConfig
	}
	return &SDKClient{secretKey: strings.TrimSpace(secretKey), backend: stripeapi.GetBackend(stripeapi.APIBackend)}, nil
}

// Ready performs a cheap authenticated read against the Stripe account API.
func (c *SDKClient) Ready(ctx context.Context) error {
	return c.backend.Call(http.MethodGet, "/v1/account", c.secretKey, &stripeapi.Params{Context: ctx}, &stripeapi.Account{})
}

// CreatePaymentIntent creates a Stripe PaymentIntent via stripe-go.
func (c *SDKClient) CreatePaymentIntent(ctx context.Context, params PaymentIntentParams) (*PaymentIntentResult, error) {

	req := &stripeapi.PaymentIntentParams{
		Amount:      stripeapi.Int64(int64(params.AmountCents)),
		Currency:    stripeapi.String(strings.ToLower(strings.TrimSpace(params.Currency))),
		Description: stripeapi.String(params.Description),
		Metadata:    params.Metadata,
	}
	if strings.TrimSpace(params.IdempotencyKey) != "" {
		req.SetIdempotencyKey(strings.TrimSpace(params.IdempotencyKey))
	}
	req.Context = ctx
	intent, err := (paymentintent.Client{B: c.backend, Key: c.secretKey}).New(req)
	if err != nil {
		return nil, err
	}
	return &PaymentIntentResult{
		ID:           intent.ID,
		ClientSecret: intent.ClientSecret,
		Status:       string(intent.Status),
		Currency:     string(intent.Currency),
		AmountCents:  int(intent.Amount),
	}, nil
}

// CreateCheckoutSession creates a hosted Stripe Checkout session via stripe-go.
func (c *SDKClient) CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (*CheckoutSessionResult, error) {

	req := &stripeapi.CheckoutSessionParams{
		Mode:       stripeapi.String(string(stripeapi.CheckoutSessionModePayment)),
		SuccessURL: stripeapi.String(params.SuccessURL),
		CancelURL:  stripeapi.String(params.CancelURL),
		Metadata:   params.Metadata,
		// Webhooks arrive as payment_intent.* events, so the PaymentIntent
		// itself must carry the invoice/org metadata.
		PaymentIntentData: &stripeapi.CheckoutSessionPaymentIntentDataParams{
			Metadata: params.Metadata,
		},
		LineItems: []*stripeapi.CheckoutSessionLineItemParams{
			{
				Quantity: stripeapi.Int64(1),
				PriceData: &stripeapi.CheckoutSessionLineItemPriceDataParams{
					Currency: stripeapi.String(strings.ToLower(strings.TrimSpace(params.Currency))),
					ProductData: &stripeapi.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripeapi.String(params.Description),
					},
					UnitAmount: stripeapi.Int64(int64(params.AmountCents)),
				},
			},
		},
	}
	if strings.TrimSpace(params.IdempotencyKey) != "" {
		req.SetIdempotencyKey(strings.TrimSpace(params.IdempotencyKey))
	}
	req.Context = ctx
	cs, err := (session.Client{B: c.backend, Key: c.secretKey}).New(req)
	if err != nil {
		return nil, err
	}
	return &CheckoutSessionResult{ID: cs.ID, URL: cs.URL}, nil
}

// ParseWebhookEvent verifies the signature and parses the payload.
func (c *SDKClient) ParseWebhookEvent(payload []byte, sigHeader, webhookSecret string) (*WebhookEventData, error) {
	if err := VerifyStripeSignature(payload, sigHeader, webhookSecret, DefaultTimestampTolerance); err != nil {
		return nil, err
	}
	return parseWebhookPayload(payload)
}

// FetchBalanceCents returns the summed gross amount for successful charge-type
// balance transactions currently visible to the Stripe account.
func (c *SDKClient) FetchBalanceCents(ctx context.Context) (int, error) {

	total := 0
	params := &stripeapi.BalanceTransactionListParams{}
	params.Filters.AddFilter("type", "", "charge")
	params.Context = ctx
	iter := (balancetransaction.Client{B: c.backend, Key: c.secretKey}).List(params)
	for iter.Next() {
		tx := iter.BalanceTransaction()
		if tx == nil {
			continue
		}
		total += int(tx.Amount)
	}
	if err := iter.Err(); err != nil {
		return 0, err
	}
	return total, nil
}

// CreateRefund creates a refund against a Stripe payment intent.
func (c *SDKClient) CreateRefund(ctx context.Context, params RefundParams) (*RefundResult, error) {

	req := &stripeapi.RefundParams{
		PaymentIntent: stripeapi.String(params.PaymentIntentID),
		Amount:        stripeapi.Int64(int64(params.AmountCents)),
	}
	if strings.TrimSpace(params.IdempotencyKey) != "" {
		req.SetIdempotencyKey(strings.TrimSpace(params.IdempotencyKey))
	}
	switch strings.TrimSpace(strings.ToLower(params.Reason)) {
	case "fraudulent":
		req.Reason = stripeapi.String(string(stripeapi.RefundReasonFraudulent))
	case "duplicate":
		req.Reason = stripeapi.String(string(stripeapi.RefundReasonDuplicate))
	default:
		req.Reason = stripeapi.String(string(stripeapi.RefundReasonRequestedByCustomer))
	}
	req.Context = ctx
	refund, err := (striperefund.Client{B: c.backend, Key: c.secretKey}).New(req)
	if err != nil {
		return nil, err
	}
	return &RefundResult{
		ID:          refund.ID,
		Status:      string(refund.Status),
		AmountCents: int(refund.Amount),
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func stubRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
