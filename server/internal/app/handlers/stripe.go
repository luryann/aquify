package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ─── StripeHandlers ───────────────────────────────────────────────────────────

// StripeHandlers handles HTTP requests for Stripe payment and webhook endpoints.
type StripeHandlers struct {
	client                    stripelib.Client
	billing                   *billing.Service
	webhookSecret             string
	currency                  string
	reconcileAlertThresholdCents int
	validate                  *validator.Validate
	logger                    *slog.Logger
}

// NewStripeHandlers returns a new StripeHandlers.
func NewStripeHandlers(
	client stripelib.Client,
	billingSvc *billing.Service,
	webhookSecret string,
	currency string,
	reconcileAlertThresholdCents int,
	logger *slog.Logger,
) *StripeHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	if currency == "" {
		currency = "usd"
	}
	return &StripeHandlers{
		client:                       client,
		billing:                      billingSvc,
		webhookSecret:                webhookSecret,
		currency:                     currency,
		reconcileAlertThresholdCents: reconcileAlertThresholdCents,
		validate:                     validator.New(),
		logger:                       logger,
	}
}

// RegisterPublicRoutes mounts routes that do NOT require auth middleware.
// The Stripe webhook endpoint must be public because Stripe cannot provide
// a Bearer token; security is enforced via HMAC signature verification instead.
func (h *StripeHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Post("/webhooks/stripe", h.handleWebhook)
}

// RegisterProtectedRoutes mounts routes that require auth + org scope middleware.
func (h *StripeHandlers) RegisterProtectedRoutes(router chi.Router) {
	router.Post("/stripe/payment-intents", h.createPaymentIntent)
	router.Post("/stripe/checkout-sessions", h.createCheckoutSession)
	router.Get("/stripe/webhook-events", h.listWebhookEvents)
	router.Get("/stripe/reconciliation", h.reconcile)
}

// ─── Request/response types ───────────────────────────────────────────────────

type createPaymentIntentRequest struct {
	InvoiceID   string            `json:"invoice_id"   validate:"required"`
	AmountCents int               `json:"amount_cents" validate:"required,gt=0"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

type createCheckoutSessionRequest struct {
	InvoiceID   string            `json:"invoice_id"   validate:"required"`
	AmountCents int               `json:"amount_cents" validate:"required,gt=0"`
	Description string            `json:"description"`
	SuccessURL  string            `json:"success_url"  validate:"required,url"`
	CancelURL   string            `json:"cancel_url"   validate:"required,url"`
	Metadata    map[string]string `json:"metadata"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (h *StripeHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return false
	}
	if err := h.validate.Struct(dst); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return false
	}
	return true
}

func stripeScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return nil, "", false
	}
	orgID, ok := appmiddleware.OrgIDFromContext(r.Context())
	if !ok || strings.TrimSpace(orgID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing X-Org-Id header.", nil)
		return nil, "", false
	}
	if strings.TrimSpace(principal.OrgID) != strings.TrimSpace(orgID) {
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Principal org_id must match X-Org-Id scope.", nil)
		return nil, "", false
	}
	return principal, orgID, true
}

// ─── Protected handlers ───────────────────────────────────────────────────────

// createPaymentIntent creates a Stripe PaymentIntent for an invoice.
func (h *StripeHandlers) createPaymentIntent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := stripeScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.billing.RequireBillingWriteAccess(principal); err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		return
	}

	var req createPaymentIntentRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	// Build metadata for Stripe so webhook handlers can route the event back.
	meta := map[string]string{
		"invoice_id": req.InvoiceID,
		"org_id":     orgID,
	}
	for k, v := range req.Metadata {
		meta[k] = v
	}

	result, err := h.client.CreatePaymentIntent(r.Context(), stripelib.PaymentIntentParams{
		AmountCents: req.AmountCents,
		Currency:    h.currency,
		Description: req.Description,
		Metadata:    meta,
	})
	if err != nil {
		h.logger.Error("stripe: create payment intent failed", "error", err)
		httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Failed to create payment intent.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"payment_intent": result,
	})
}

// createCheckoutSession creates a Stripe Checkout Session for an invoice.
func (h *StripeHandlers) createCheckoutSession(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := stripeScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.billing.RequireBillingWriteAccess(principal); err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		return
	}

	var req createCheckoutSessionRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meta := map[string]string{
		"invoice_id": req.InvoiceID,
		"org_id":     orgID,
	}
	for k, v := range req.Metadata {
		meta[k] = v
	}

	result, err := h.client.CreateCheckoutSession(r.Context(), stripelib.CheckoutSessionParams{
		AmountCents: req.AmountCents,
		Currency:    h.currency,
		Description: req.Description,
		Metadata:    meta,
		SuccessURL:  req.SuccessURL,
		CancelURL:   req.CancelURL,
	})
	if err != nil {
		h.logger.Error("stripe: create checkout session failed", "error", err)
		httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Failed to create checkout session.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"checkout_session": result,
	})
}

// listWebhookEvents returns persisted Stripe webhook event records.
func (h *StripeHandlers) listWebhookEvents(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := stripeScopeFromRequest(w, r)
	if !ok {
		return
	}

	events, err := h.billing.ListWebhookEvents(orgID, principal)
	if err != nil {
		if errors.Is(err, billing.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list webhook events.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

// reconcile runs an on-demand reconciliation between the internal billing ledger
// and the Stripe balance report. Returns a ReconciliationReport.
func (h *StripeHandlers) reconcile(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := stripeScopeFromRequest(w, r)
	if !ok {
		return
	}

	stripeCents, err := h.client.FetchBalanceCents(r.Context())
	if err != nil {
		h.logger.Error("stripe: fetch balance failed", "error", err)
		httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Failed to fetch Stripe balance.", nil)
		return
	}

	report, err := h.billing.GetReconciliationReport(orgID, principal, stripeCents, h.reconcileAlertThresholdCents)
	if err != nil {
		if errors.Is(err, billing.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Reconciliation failed.", nil)
		return
	}

	if report.AlertTriggered {
		h.logger.Warn("stripe: reconciliation mismatch alert triggered",
			"org_id", orgID,
			"ledger_cents", report.LedgerStripeTotalCents,
			"stripe_cents", report.StripeReportedCents,
			"mismatch_cents", report.MismatchCents,
			"threshold_cents", report.AlertThresholdCents,
		)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"reconciliation": report,
	})
}

// ─── Webhook handler (public) ─────────────────────────────────────────────────

// handleWebhook receives and processes incoming Stripe webhook events.
// This endpoint is intentionally unauthenticated; security is enforced via the
// Stripe-Signature HMAC verification performed by h.client.ParseWebhookEvent.
//
// Processing is idempotent: duplicate event IDs are acknowledged (200) but
// not re-processed.
func (h *StripeHandlers) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read raw body first — must happen before any JSON decoder touches it,
	// because signature verification requires the exact original bytes.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB limit
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "read_error", "Failed to read request body.", nil)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")

	// Parse + verify webhook. Stub client skips signature check; real client enforces it.
	event, err := h.client.ParseWebhookEvent(body, sigHeader, h.webhookSecret)
	if err != nil {
		switch {
		case errors.Is(err, stripelib.ErrInvalidSignature),
			errors.Is(err, stripelib.ErrExpiredTimestamp),
			errors.Is(err, stripelib.ErrMalformedHeader):
			httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_signature", "Webhook signature verification failed.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "webhook_parse_error", "Failed to parse webhook payload.", nil)
		}
		return
	}

	// Persist the webhook event and check for duplicates.
	// orgID may be empty for events without metadata; RecordWebhookEvent accepts this.
	eventOrgID := event.Metadata["org_id"]
	isNew, err := h.billing.RecordWebhookEvent("stripe", eventOrgID, event.ID, event.Type, event.RawPayload)
	if err != nil {
		h.logger.Error("stripe webhook: failed to record event", "event_id", event.ID, "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to persist webhook event.", nil)
		return
	}
	if !isNew {
		// Duplicate — acknowledge safely without re-processing.
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "already_processed"})
		return
	}

	// Dispatch to the appropriate domain handler.
	processErr := h.dispatchWebhookEvent(r.Context(), event)

	// Mark the event as processed (or failed) in the store.
	h.billing.MarkWebhookProcessed("stripe", event.ID, processErr)

	if processErr != nil {
		h.logger.Error("stripe webhook: processing failed",
			"event_id", event.ID, "event_type", event.Type, "error", processErr)
		// Still return 200 to prevent Stripe from retrying events we already
		// persisted (avoid flooding the webhook queue). The error is stored
		// server-side only; Stripe does not use the response body.
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "persisted_with_error",
		})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "processed"})
}

// dispatchWebhookEvent routes a parsed Stripe event to the correct handler.
func (h *StripeHandlers) dispatchWebhookEvent(ctx context.Context, event *stripelib.WebhookEventData) error {
	switch event.Type {
	case "payment_intent.succeeded":
		return h.onPaymentIntentSucceeded(ctx, event)
	case "payment_intent.payment_failed":
		return h.onPaymentIntentFailed(ctx, event)
	case "charge.refunded":
		return h.onChargeRefunded(ctx, event)
	default:
		// Unknown event types are silently acknowledged.
		h.logger.Info("stripe webhook: unhandled event type", "type", event.Type)
		return nil
	}
}

// onPaymentIntentSucceeded handles a payment_intent.succeeded event.
func (h *StripeHandlers) onPaymentIntentSucceeded(_ context.Context, event *stripelib.WebhookEventData) error {
	paymentIntentID, _ := event.Object["id"].(string)
	if paymentIntentID == "" {
		return errors.New("payment_intent.succeeded: missing payment intent ID in event object")
	}

	amountCents := int(objectInt64(event.Object, "amount"))
	orgID := event.Metadata["org_id"]
	invoiceID := event.Metadata["invoice_id"]

	if orgID == "" || invoiceID == "" {
		h.logger.Error("stripe webhook: payment_intent.succeeded missing metadata; payment cannot be attributed",
			"payment_intent_id", paymentIntentID)
		return fmt.Errorf("payment_intent.succeeded event %s missing org_id or invoice_id metadata — manual attribution required", paymentIntentID)
	}

	_, _, err := h.billing.HandleStripePaymentIntentSucceeded(
		orgID, paymentIntentID, invoiceID, amountCents, "stripe-webhook",
	)
	return err
}

// onPaymentIntentFailed handles a payment_intent.payment_failed event.
func (h *StripeHandlers) onPaymentIntentFailed(_ context.Context, event *stripelib.WebhookEventData) error {
	paymentIntentID, _ := event.Object["id"].(string)
	if paymentIntentID == "" {
		return errors.New("payment_intent.payment_failed: missing payment intent ID")
	}
	return h.billing.HandleStripePaymentIntentFailed(paymentIntentID)
}

// onChargeRefunded handles a charge.refunded event.
func (h *StripeHandlers) onChargeRefunded(_ context.Context, event *stripelib.WebhookEventData) error {
	paymentIntentID, _ := event.Object["payment_intent"].(string)
	if paymentIntentID == "" {
		h.logger.Warn("stripe webhook: charge.refunded has no payment_intent; skipping")
		return nil
	}

	amountRefunded := int(objectInt64(event.Object, "amount_refunded"))
	orgID := event.Metadata["org_id"]
	invoiceID := event.Metadata["invoice_id"]

	if orgID == "" || invoiceID == "" {
		h.logger.Error("stripe webhook: charge.refunded missing metadata; refund cannot be attributed",
			"payment_intent_id", paymentIntentID)
		return fmt.Errorf("charge.refunded event (payment_intent %s) missing org_id or invoice_id metadata — manual attribution required", paymentIntentID)
	}

	// Extract the latest refund ID from the refunds list (if present).
	stripeRefundID := extractLatestRefundID(event.Object)

	_, _, err := h.billing.HandleStripeChargeRefunded(
		orgID, paymentIntentID, invoiceID, amountRefunded,
		stripeRefundID, "", "stripe-webhook",
	)
	return err
}

// ─── Object field helpers ─────────────────────────────────────────────────────

// objectInt64 safely extracts an int64 from a map[string]any field.
// JSON numbers are float64 in Go; we cast accordingly.
func objectInt64(obj map[string]any, key string) int64 {
	v, ok := obj[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// extractLatestRefundID pulls the most recent refund ID from
// charge.refunds.data[0].id if present, or returns an empty string.
func extractLatestRefundID(obj map[string]any) string {
	refunds, ok := obj["refunds"].(map[string]any)
	if !ok {
		return ""
	}
	data, ok := refunds["data"].([]any)
	if !ok || len(data) == 0 {
		return ""
	}
	first, ok := data[0].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := first["id"].(string)
	return id
}
