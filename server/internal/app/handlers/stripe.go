package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ─── StripeHandlers ───────────────────────────────────────────────────────────

// StripeHandlers handles HTTP requests for Stripe payment and webhook endpoints.
type StripeHandlers struct {
	client                       stripelib.Client
	billing                      *billing.Service
	webhookSecret                string
	currency                     string
	reconcileAlertThresholdCents int
	validate                     *validator.Validate
	logger                       *slog.Logger
	// writeLimiter is an optional extra rate-limiting middleware applied only
	// to cost-driving write endpoints (payment-intents, checkout-sessions).
	writeLimiter func(http.Handler) http.Handler
	// allowedReturnOrigins restricts Stripe Checkout success_url/cancel_url to
	// this app's own trusted origins, so a tampered API call cannot redirect
	// a post-payment browser to an attacker-controlled host.
	allowedReturnOrigins []string
	// metrics, when set, receives webhook receipt-to-processing lag.
	metrics *telemetry.Metrics
}

// SetMetrics enables recording of dare_webhook_processing_lag_seconds.
func (h *StripeHandlers) SetMetrics(m *telemetry.Metrics) { h.metrics = m }

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
		validate:                     httpx.NewValidator(),
		logger:                       logger,
	}
}

// RegisterPublicRoutes mounts routes that do NOT require auth middleware.
// The Stripe webhook endpoint must be public because Stripe cannot provide
// a Bearer token; security is enforced via HMAC signature verification instead.
func (h *StripeHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Post("/webhooks/stripe", h.handleWebhook)
}

// SetWriteLimiter injects an extra rate-limiting middleware applied only to
// cost-driving write routes (payment-intents, checkout-sessions).
func (h *StripeHandlers) SetWriteLimiter(limiter func(http.Handler) http.Handler) {
	h.writeLimiter = limiter
}

// SetAllowedReturnOrigins configures the origin allowlist enforced against
// Checkout Session success_url/cancel_url.
func (h *StripeHandlers) SetAllowedReturnOrigins(origins []string) {
	h.allowedReturnOrigins = origins
}

// isAllowedReturnURL reports whether rawURL is an absolute https(s) URL whose
// origin (scheme+host) matches one of the configured allowed origins.
func isAllowedReturnURL(rawURL string, allowedOrigins []string) bool {
	if len(allowedOrigins) == 0 {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	origin := u.Scheme + "://" + u.Host
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return true
		}
	}
	return false
}

// RegisterProtectedRoutes mounts routes that require auth + org scope middleware.
func (h *StripeHandlers) RegisterProtectedRoutes(router chi.Router) {
	router.Get("/stripe/webhook-events", h.listWebhookEvents)
	router.Get("/stripe/reconciliation", h.reconcile)

	// Payment write routes receive an additional, tighter rate limiter to
	// constrain authenticated abuse that would drive Stripe API costs.
	if h.writeLimiter != nil {
		router.Group(func(r chi.Router) {
			r.Use(h.writeLimiter)
			r.Post("/stripe/payment-intents", h.createPaymentIntent)
			r.Post("/stripe/checkout-sessions", h.createCheckoutSession)
		})
	} else {
		router.Post("/stripe/payment-intents", h.createPaymentIntent)
		router.Post("/stripe/checkout-sessions", h.createCheckoutSession)
	}
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

const stripeIdempotencyHeader = "Idempotency-Key"

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (h *StripeHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateUnprocessable(w, r, dst, h.validate)
}

// stripeMetadata merges caller metadata under the trusted routing keys, which
// always win so a caller cannot redirect a payment to another invoice or org.
func stripeMetadata(extra map[string]string, invoiceID, orgID string) map[string]string {
	meta := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		meta[k] = v
	}
	meta["invoice_id"] = invoiceID
	meta["org_id"] = orgID
	return meta
}

func stripeIdempotencyKeyFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.Header.Get(stripeIdempotencyHeader))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required.", nil)
		return "", false
	}
	return key, true
}

// errCheckoutDenied means the principal may not pay this invoice for this amount.
var errCheckoutDenied = errors.New("checkout denied")

// authorizeCheckoutSessionPrincipal returns the invoice when the principal may
// pay it for req.AmountCents. Errors are either errCheckoutDenied, the billing
// lookup error (ErrInvoiceNotFound or an infrastructure failure).
func (h *StripeHandlers) authorizeCheckoutSessionPrincipal(principal *appmiddleware.Principal, staff bool, orgID string, req *createCheckoutSessionRequest) (*billing.Invoice, error) {
	invoice, err := h.billing.GetInvoice(orgID, principal, req.InvoiceID)
	if err != nil {
		return nil, err
	}
	if staff {
		// Staff may pay down part of a balance, but never more than is due. This
		// is a precheck only: invoice state can change before the session pays, and
		// the webhook routes any resulting overpayment to household account credit.
		if invoice.AmountDueCents > 0 && req.AmountCents <= invoice.AmountDueCents {
			return invoice, nil
		}
		return nil, errCheckoutDenied
	}
	if invoice.AmountDueCents <= 0 || req.AmountCents != invoice.AmountDueCents {
		return nil, errCheckoutDenied
	}
	return invoice, nil
}

// ─── Protected handlers ───────────────────────────────────────────────────────

// createPaymentIntent creates a Stripe PaymentIntent for an invoice.
func (h *StripeHandlers) createPaymentIntent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
	idempotencyKey, ok := stripeIdempotencyKeyFromRequest(w, r)
	if !ok {
		return
	}

	invoice, err := h.billing.GetInvoice(orgID, principal, req.InvoiceID)
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrInvoiceNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "invoice_not_found", "Invoice not found.", nil)
		case errors.Is(err, billing.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "invoice_lookup_failed", "Failed to load invoice.", err)
		}
		return
	}
	if invoice.AmountDueCents <= 0 || req.AmountCents > invoice.AmountDueCents {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_amount", "amount_cents cannot exceed the invoice's remaining balance.", nil)
		return
	}

	// Build metadata for Stripe so webhook handlers can route the event back.
	meta := stripeMetadata(req.Metadata, req.InvoiceID, orgID)

	result, err := h.client.CreatePaymentIntent(r.Context(), stripelib.PaymentIntentParams{
		AmountCents:    req.AmountCents,
		Currency:       h.currency,
		Description:    req.Description,
		Metadata:       meta,
		IdempotencyKey: idempotencyKey,
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req createCheckoutSessionRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}
	idempotencyKey, ok := stripeIdempotencyKeyFromRequest(w, r)
	if !ok {
		return
	}
	staff := h.billing.RequireBillingWriteAccess(principal) == nil
	invoice, err := h.authorizeCheckoutSessionPrincipal(principal, staff, orgID, &req)
	if err != nil {
		// Staff may learn that an invoice doesn't exist; other roles get a
		// uniform 403 so invoice IDs can't be probed.
		switch {
		case staff && errors.Is(err, billing.ErrInvoiceNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "invoice_not_found", "Invoice not found.", nil)
		case errors.Is(err, errCheckoutDenied), !staff:
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "invoice_lookup_failed", "Failed to load invoice.", err)
		}
		return
	}
	if !isAllowedReturnURL(req.SuccessURL, h.allowedReturnOrigins) || !isAllowedReturnURL(req.CancelURL, h.allowedReturnOrigins) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_return_url", "success_url and cancel_url must belong to a trusted application origin.", nil)
		return
	}

	meta := stripeMetadata(req.Metadata, req.InvoiceID, orgID)

	description := req.Description
	if invoice != nil {
		description = strings.TrimSpace(invoice.Notes)
		if description == "" {
			description = invoice.InvoiceNumber
		}
	}
	result, err := h.client.CreateCheckoutSession(r.Context(), stripelib.CheckoutSessionParams{
		AmountCents:    req.AmountCents,
		Currency:       h.currency,
		Description:    description,
		Metadata:       meta,
		SuccessURL:     req.SuccessURL,
		CancelURL:      req.CancelURL,
		IdempotencyKey: idempotencyKey,
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	events, err := h.billing.ListWebhookEvents(orgID, principal)
	if err != nil {
		if errors.Is(err, billing.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list webhook events.", err)
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Reconciliation failed.", err)
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

	receivedAt := time.Now()
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
	if errors.Is(err, billing.ErrWebhookInFlight) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "webhook_in_flight", "Webhook processing is in progress; retry later.", nil)
		return
	}
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
	if h.metrics != nil {
		h.metrics.WebhookProcessingLag.WithLabelValues(event.Type).Observe(time.Since(receivedAt).Seconds())
	}

	// Permanent failures (bad metadata, unknown/unpayable invoice) are stored
	// and acknowledged: a retry could never succeed. Anything else is treated
	// as transient so Stripe redelivers and the event is processed again.
	retryable := processErr != nil && !isPermanentWebhookError(processErr, event.Created)
	if err := h.billing.MarkWebhookProcessed("stripe", event.ID, processErr, retryable); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "webhook_outcome_failed", "Failed to persist webhook outcome; retry later.", err)
		return
	}

	if processErr != nil {
		h.logger.Error("stripe webhook: processing failed",
			"event_id", event.ID, "event_type", event.Type, "retryable", retryable, "error", processErr)
		if retryable {
			httpx.WriteError(w, r, http.StatusInternalServerError, "webhook_processing_failed", "Webhook processing failed; retry later.", processErr)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "persisted_with_error",
		})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "processed"})
}

var errWebhookUnattributable = errors.New("webhook event cannot be attributed")

// webhookLagGrace bounds how long a not-found outcome stays retryable. In a
// multi-instance deployment the instance that receives a webhook may not have
// seen the invoice or payment yet, so those errors are retried. Once the event
// is older than this, the record is genuinely missing and the event is
// acknowledged instead of retried for Stripe's full redelivery window.
const webhookLagGrace = time.Hour

// permanentWebhookErrors are domain outcomes a redelivery can never change.
// Anything outside this set (database, storage, context errors) is treated as
// transient so Stripe redelivers it. Not-found errors are handled separately
// by isPermanentWebhookError, with the lag grace above.
var permanentWebhookErrors = []error{
	errWebhookUnattributable,
	billing.ErrInvoiceItemNotFound,
	billing.ErrRefundNotFound,
	billing.ErrInvalidTransition,
	billing.ErrInvoiceNotDraft,
	billing.ErrInvoiceNotIssuable,
	billing.ErrInvoiceNotVoidable,
	billing.ErrInvoiceNotWriteOffable,
	billing.ErrPaymentExceedsBalance,
	billing.ErrRefundExceedsPayment,
	billing.ErrRefundRequiresReason,
	billing.ErrHouseholdRequired,
	billing.ErrCreditPaymentRefundToCredit,
	billing.ErrInsufficientCredit,
}

func isPermanentWebhookError(err error, createdUnix int64) bool {
	if errors.Is(err, billing.ErrInvoiceNotFound) || errors.Is(err, billing.ErrPaymentNotFound) {
		return createdUnix > 0 && time.Since(time.Unix(createdUnix, 0)) > webhookLagGrace
	}
	for _, permanent := range permanentWebhookErrors {
		if errors.Is(err, permanent) {
			return true
		}
	}
	var validation *billing.FinanceValidationError
	return errors.As(err, &validation)
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

// webhookRouting returns the org and invoice a payment event was created for
// (the metadata set when the intent or session was made). Events without them
// cannot be attributed and are a permanent failure needing manual follow-up.
func (h *StripeHandlers) webhookRouting(event *stripelib.WebhookEventData, paymentIntentID string) (orgID, invoiceID string, err error) {
	orgID, invoiceID = event.Metadata["org_id"], event.Metadata["invoice_id"]
	if orgID == "" || invoiceID == "" {
		h.logger.Error("stripe webhook: event missing routing metadata; it cannot be attributed",
			"event_type", event.Type, "payment_intent_id", paymentIntentID)
		return "", "", fmt.Errorf("%w: %s event (payment_intent %s) missing org_id or invoice_id metadata — manual attribution required", errWebhookUnattributable, event.Type, paymentIntentID)
	}
	return orgID, invoiceID, nil
}

// onPaymentIntentSucceeded handles a payment_intent.succeeded event.
func (h *StripeHandlers) onPaymentIntentSucceeded(_ context.Context, event *stripelib.WebhookEventData) error {
	paymentIntentID, _ := event.Object["id"].(string)
	if paymentIntentID == "" {
		return errors.New("payment_intent.succeeded: missing payment intent ID in event object")
	}

	amountCents := int(objectInt64(event.Object, "amount"))
	orgID, invoiceID, err := h.webhookRouting(event, paymentIntentID)
	if err != nil {
		return err
	}

	_, _, err = h.billing.HandleStripePaymentIntentSucceeded(
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
	orgID, invoiceID, err := h.webhookRouting(event, paymentIntentID)
	if err != nil {
		return err
	}

	// Extract the latest refund ID from the refunds list (if present).
	stripeRefundID := extractLatestRefundID(event.Object)

	_, _, err = h.billing.HandleStripeChargeRefunded(
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
