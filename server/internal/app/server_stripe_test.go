package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newStripeRouter(t *testing.T, billingSvc *billing.Service, stripeClient stripelib.Client) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := Config{
		Auth:   AuthConfig{EnableDevStubTokens: true},
		Stripe: StripeConfig{WebhookSecret: "whsec_test", Currency: "usd", ReconciliationAlertThresholdCents: 100},
	}
	return NewRouterWithServices(cfg, logger, nil, nil, nil, nil, nil, nil, nil, billingSvc, stripeClient, nil, nil, nil, nil)
}

// ─── Webhook payload builders ─────────────────────────────────────────────────

func buildWebhookPayload(eventID, eventType string, obj map[string]any) []byte {
	payload, _ := json.Marshal(map[string]any{
		"id":      eventID,
		"type":    eventType,
		"created": time.Now().Unix(),
		"data": map[string]any{
			"object": obj,
		},
	})
	return payload
}

// signWebhookPayload computes a Stripe-compatible Stripe-Signature header for
// testing the real signature verification path.
func signWebhookPayload(payload []byte, secret string) string {
	ts := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", ts, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", ts, sig)
}

// postWebhook sends a raw webhook payload to /api/v1/webhooks/stripe.
func postWebhook(router http.Handler, payload []byte, sigHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	if sigHeader != "" {
		req.Header.Set("Stripe-Signature", sigHeader)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── Tests: Webhook deduplication ────────────────────────────────────────────

func TestStripeWebhookDuplicateReplay(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)

	payload := buildWebhookPayload("evt_dup001", "payment_intent.succeeded", map[string]any{
		"id":     "pi_xxx",
		"amount": float64(1000),
		"metadata": map[string]any{
			"org_id":     "org-demo",
			"invoice_id": "inv-xxx",
		},
	})

	// First delivery.
	rr1 := postWebhook(router, payload, "")
	if rr1.Code != http.StatusOK {
		t.Fatalf("first webhook: expected 200, got %d: %s", rr1.Code, rr1.Body.String())
	}

	// Second delivery of same event ID.
	rr2 := postWebhook(router, payload, "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("second webhook: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}

	var resp2 map[string]any
	json.NewDecoder(rr2.Body).Decode(&resp2)
	if resp2["status"] != "already_processed" {
		t.Fatalf("expected status=already_processed on duplicate, got %v", resp2["status"])
	}
}

// ─── Tests: payment_intent.succeeded ─────────────────────────────────────────

func TestStripeWebhookPaymentIntentSucceededRecordsPayment(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	billingSvc := billing.NewService()

	// Issue an invoice so the webhook can find it.
	p := adminPrincipal()
	inv, _ := billingSvc.CreateInvoice("org-demo", "actor-1", p, billing.CreateInvoiceInput{
		HouseholdID:   "hh-001",
		InvoiceNumber: "INV-STRIPE-1",
	})
	item, _ := billingSvc.AddInvoiceItem("org-demo", "actor-1", p, inv.ID, billing.AddInvoiceItemInput{
		Description:     "Program fee",
		Quantity:        1,
		UnitAmountCents: 5000,
		Category:        "program_fee",
	})
	_ = item
	billingSvc.IssueInvoice("org-demo", "actor-1", p, inv.ID)

	router := newStripeRouter(t, billingSvc, stub)

	payload := buildWebhookPayload("evt_pi_succeeded", "payment_intent.succeeded", map[string]any{
		"id":     "pi_test_1",
		"amount": float64(5000),
		"metadata": map[string]any{
			"org_id":     "org-demo",
			"invoice_id": inv.ID,
		},
	})

	rr := postWebhook(router, payload, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "processed" {
		t.Fatalf("expected status=processed, got %v", resp["status"])
	}

	// Verify ledger has a payment_succeeded event.
	events := billingSvc.LedgerEvents()
	var found bool
	for _, ev := range events {
		if ev.EventType == "payment_succeeded" && ev.InvoiceID == inv.ID {
			found = true
			if ev.AmountCents != -5000 {
				t.Fatalf("expected ledger amount=-5000, got %d", ev.AmountCents)
			}
		}
	}
	if !found {
		t.Fatal("expected a payment_succeeded ledger event after webhook processing")
	}
}

// TestStripeWebhookPaymentIntentSucceededIdempotent ensures that sending the
// same payment_intent.succeeded event twice only creates one payment.
func TestStripeWebhookPaymentIntentSucceededIdempotent(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	billingSvc := billing.NewService()

	p := adminPrincipal()
	inv, _ := billingSvc.CreateInvoice("org-demo", "actor-1", p, billing.CreateInvoiceInput{
		HouseholdID:   "hh-001",
		InvoiceNumber: "INV-IDEMP",
	})
	billingSvc.AddInvoiceItem("org-demo", "actor-1", p, inv.ID, billing.AddInvoiceItemInput{
		Description:     "Fee",
		Quantity:        1,
		UnitAmountCents: 3000,
		Category:        "program_fee",
	})
	billingSvc.IssueInvoice("org-demo", "actor-1", p, inv.ID)

	router := newStripeRouter(t, billingSvc, stub)

	payload := buildWebhookPayload("evt_idemp", "payment_intent.succeeded", map[string]any{
		"id":     "pi_idemp",
		"amount": float64(3000),
		"metadata": map[string]any{
			"org_id":     "org-demo",
			"invoice_id": inv.ID,
		},
	})

	postWebhook(router, payload, "")
	postWebhook(router, payload, "") // duplicate

	// Should only have one payment_succeeded event.
	count := 0
	for _, ev := range billingSvc.LedgerEvents() {
		if ev.EventType == "payment_succeeded" && ev.InvoiceID == inv.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 payment_succeeded ledger event, got %d", count)
	}
}

// ─── Tests: payment_intent.payment_failed ────────────────────────────────────

func TestStripeWebhookPaymentIntentFailed(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)

	// payment_intent.payment_failed for a PI that has no matching payment is a no-op.
	payload := buildWebhookPayload("evt_pi_failed", "payment_intent.payment_failed", map[string]any{
		"id": "pi_unknown",
	})

	rr := postWebhook(router, payload, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "processed" {
		t.Fatalf("expected status=processed, got %v", resp["status"])
	}
}

// ─── Tests: charge.refunded ───────────────────────────────────────────────────

func TestStripeWebhookChargeRefundedProcessesRefund(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	billingSvc := billing.NewService()

	p := adminPrincipal()
	inv, _ := billingSvc.CreateInvoice("org-demo", "actor-1", p, billing.CreateInvoiceInput{
		HouseholdID:   "hh-001",
		InvoiceNumber: "INV-REFUNDWH",
	})
	billingSvc.AddInvoiceItem("org-demo", "actor-1", p, inv.ID, billing.AddInvoiceItemInput{
		Description:     "Fee",
		Quantity:        1,
		UnitAmountCents: 8000,
		Category:        "program_fee",
	})
	billingSvc.IssueInvoice("org-demo", "actor-1", p, inv.ID)

	// Simulate payment via webhook.
	router := newStripeRouter(t, billingSvc, stub)

	// First: payment_intent.succeeded
	piPayload := buildWebhookPayload("evt_pi_succ_refund", "payment_intent.succeeded", map[string]any{
		"id":     "pi_refundtest",
		"amount": float64(8000),
		"metadata": map[string]any{
			"org_id":     "org-demo",
			"invoice_id": inv.ID,
		},
	})
	rr := postWebhook(router, piPayload, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("payment succeeded webhook: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Then: charge.refunded
	refundPayload := buildWebhookPayload("evt_charge_refunded", "charge.refunded", map[string]any{
		"payment_intent":  "pi_refundtest",
		"amount_refunded": float64(8000),
		"refunds": map[string]any{
			"data": []any{
				map[string]any{"id": "re_test123"},
			},
		},
		"metadata": map[string]any{
			"org_id":     "org-demo",
			"invoice_id": inv.ID,
		},
	})
	rr2 := postWebhook(router, refundPayload, "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("charge.refunded webhook: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// Verify invoice is now refunded_full.
	updatedInv, _ := billingSvc.GetInvoice("org-demo", p, inv.ID)
	if updatedInv.Status != "refunded_full" {
		t.Fatalf("expected invoice status=refunded_full after refund webhook, got %q", updatedInv.Status)
	}
}

// ─── Tests: invalid signature ─────────────────────────────────────────────────

// TestStripeWebhookInvalidSignatureRejectedByRealVerifier tests the standalone
// VerifyStripeSignature function, which is separate from the stub client path.
func TestStripeWebhookInvalidSignatureRejectedByRealVerifier(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"id":"evt_bad","type":"payment_intent.succeeded"}`)
	err := stripelib.VerifyStripeSignature(payload, "t=1,v1=badhex", "whsec_test", 5*time.Minute)
	if err == nil {
		t.Fatal("expected signature verification to fail with bad signature")
	}
}

func TestStripeWebhookValidSignatureAcceptedByRealVerifier(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_valid","type":"payment_intent.succeeded"}`)
	sigHeader := signWebhookPayload(payload, secret)

	err := stripelib.VerifyStripeSignature(payload, sigHeader, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected valid signature to pass verification, got: %v", err)
	}
}

func TestStripeWebhookExpiredTimestampRejected(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_expired"
	payload := []byte(`{"id":"evt_old","type":"payment_intent.succeeded"}`)

	// Forge a timestamp 10 minutes in the past.
	oldTS := time.Now().Add(-10 * time.Minute).Unix()
	signed := fmt.Sprintf("%d.%s", oldTS, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	sigHeader := fmt.Sprintf("t=%d,v1=%s", oldTS, sig)

	err := stripelib.VerifyStripeSignature(payload, sigHeader, secret, 5*time.Minute)
	if err == nil {
		t.Fatal("expected expired timestamp to be rejected")
	}
}

// ─── Tests: create payment intent ─────────────────────────────────────────────

func TestCreatePaymentIntentSuccess(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	body, _ := json.Marshal(map[string]any{
		"invoice_id":   "inv-001",
		"amount_cents": 15000,
		"description":  "Annual fee",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/payment-intents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		PaymentIntent struct {
			ID           string `json:"id"`
			ClientSecret string `json:"client_secret"`
			Status       string `json:"status"`
			AmountCents  int    `json:"amount_cents"`
		} `json:"payment_intent"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.PaymentIntent.ID == "" {
		t.Fatal("expected payment_intent.id to be set")
	}
	if resp.PaymentIntent.ClientSecret == "" {
		t.Fatal("expected payment_intent.client_secret to be set")
	}
	if resp.PaymentIntent.AmountCents != 15000 {
		t.Fatalf("expected amount_cents=15000, got %d", resp.PaymentIntent.AmountCents)
	}

	// Verify stub recorded the intent.
	if len(stub.CreatedIntents()) != 1 {
		t.Fatalf("expected 1 created intent in stub, got %d", len(stub.CreatedIntents()))
	}
}

func TestCreatePaymentIntentForbiddenForParent(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	body, _ := json.Marshal(map[string]any{
		"invoice_id":   "inv-001",
		"amount_cents": 5000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/payment-intents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for parent role, got %d", rr.Code)
	}
}

func TestCreatePaymentIntentRequiresAuth(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)

	body, _ := json.Marshal(map[string]any{"invoice_id": "x", "amount_cents": 1000})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/payment-intents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rr.Code)
	}
}

// ─── Tests: checkout sessions ─────────────────────────────────────────────────

func TestCreateCheckoutSessionSuccess(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	body, _ := json.Marshal(map[string]any{
		"invoice_id":   "inv-cs-001",
		"amount_cents": 20000,
		"success_url":  "https://example.com/success",
		"cancel_url":   "https://example.com/cancel",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/checkout-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		CheckoutSession struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"checkout_session"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.CheckoutSession.ID == "" {
		t.Fatal("expected checkout_session.id to be set")
	}
	if !strings.HasPrefix(resp.CheckoutSession.URL, "https://checkout.stripe.com/pay/") {
		t.Fatalf("expected checkout URL prefix, got %q", resp.CheckoutSession.URL)
	}
}

// ─── Tests: reconciliation ────────────────────────────────────────────────────

func TestStripeReconciliationNoMismatch(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	// Stub returns 0 balance by default — reconciliation should show no mismatch
	// since ledger also starts at 0.
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stripe/reconciliation", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Reconciliation struct {
			HasMismatch    bool `json:"has_mismatch"`
			AlertTriggered bool `json:"alert_triggered"`
			MismatchCents  int  `json:"mismatch_cents"`
		} `json:"reconciliation"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Reconciliation.HasMismatch {
		t.Fatalf("expected no mismatch, got mismatch_cents=%d", resp.Reconciliation.MismatchCents)
	}
}

func TestStripeReconciliationAlertTriggeredOnMismatch(t *testing.T) {
	t.Parallel()
	billingSvc := billing.NewService()

	// Record a Stripe payment in the ledger.
	p := adminPrincipal()
	inv, _ := billingSvc.CreateInvoice("org-demo", "actor-1", p, billing.CreateInvoiceInput{
		HouseholdID:   "hh-001",
		InvoiceNumber: "INV-RECON",
	})
	billingSvc.AddInvoiceItem("org-demo", "actor-1", p, inv.ID, billing.AddInvoiceItemInput{
		Description:     "Fee",
		Quantity:        1,
		UnitAmountCents: 5000,
		Category:        "program_fee",
	})
	billingSvc.IssueInvoice("org-demo", "actor-1", p, inv.ID)
	billingSvc.RecordPayment("org-demo", "actor-1", p, inv.ID, billing.RecordPaymentInput{
		AmountCents:           5000,
		Method:                "stripe",
		StripePaymentIntentID: "pi_recon1",
	})

	// Stub returns 0 from Stripe — causing a mismatch of 5000 cents which is above threshold of 100.
	stub := stripelib.NewStubClient()
	stub.BalanceCents = 0

	router := newStripeRouter(t, billingSvc, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stripe/reconciliation", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Reconciliation struct {
			HasMismatch            bool `json:"has_mismatch"`
			AlertTriggered         bool `json:"alert_triggered"`
			MismatchCents          int  `json:"mismatch_cents"`
			LedgerStripeTotalCents int  `json:"ledger_stripe_total_cents"`
		} `json:"reconciliation"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if !resp.Reconciliation.HasMismatch {
		t.Fatal("expected has_mismatch=true")
	}
	if !resp.Reconciliation.AlertTriggered {
		t.Fatal("expected alert_triggered=true when mismatch > threshold")
	}
	if resp.Reconciliation.LedgerStripeTotalCents != 5000 {
		t.Fatalf("expected ledger_stripe_total_cents=5000, got %d", resp.Reconciliation.LedgerStripeTotalCents)
	}
}

func TestStripeReconciliationForbiddenForParent(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stripe/reconciliation", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for parent role, got %d", rr.Code)
	}
}

// ─── Tests: list webhook events ───────────────────────────────────────────────

func TestListWebhookEventsAdminCanRead(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	billingSvc := billing.NewService()
	router := newStripeRouter(t, billingSvc, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Send one webhook to populate the store. Include org_id in metadata so
	// the event is scoped to "org-demo" and visible in the filtered list.
	payload := buildWebhookPayload("evt_list001", "payment_intent.payment_failed", map[string]any{
		"id": "pi_list1",
		"metadata": map[string]any{
			"org_id": "org-demo",
		},
	})
	postWebhook(router, payload, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stripe/webhook-events", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Count  int `json:"count"`
		Events []struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
		} `json:"events"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Count < 1 {
		t.Fatalf("expected at least 1 webhook event, got %d", resp.Count)
	}
}

func TestListWebhookEventsParentForbidden(t *testing.T) {
	t.Parallel()
	stub := stripelib.NewStubClient()
	router := newStripeRouter(t, nil, stub)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stripe/webhook-events", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// adminPrincipal returns a Principal with the "admin" role for use in direct
// service calls within tests (bypasses HTTP layer).
func adminPrincipal() *appmiddleware.Principal {
	return &appmiddleware.Principal{
		UserID: "actor-1",
		OrgID:  "org-demo",
		Roles:  []string{"admin"},
	}
}
