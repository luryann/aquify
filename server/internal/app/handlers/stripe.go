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

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/httpx"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/telemetry"
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
	// Stripe Connect: each club transacts on its own connected account.
	accounts *payments.Service
	// orgCurrency returns the club's own billing currency ("" = the deployment default).
	orgCurrency          func(orgID string) string
	connectRequired      bool
	connectWebhookSecret string
}

// SetConnect wires per-club connected accounts. With required set, a club that
// has not finished onboarding cannot take payments (it never falls back to the
// platform account); connectWebhookSecret verifies the Connect webhook endpoint.
func (h *StripeHandlers) SetConnect(accounts *payments.Service, required bool, connectWebhookSecret string) {
	h.accounts = accounts
	h.connectRequired = required
	h.connectWebhookSecret = connectWebhookSecret
}

// SetOrgCurrency lets each club bill in its own currency.
func (h *StripeHandlers) SetOrgCurrency(fn func(orgID string) string) { h.orgCurrency = fn }

// currencyFor is the club's currency, else the deployment default.
func (h *StripeHandlers) currencyFor(orgID string) string {
	if h.orgCurrency != nil {
		if c := strings.TrimSpace(h.orgCurrency(orgID)); c != "" {
			return c
		}
	}
	return h.currency
}

var errPaymentsNotEnabled = errors.New("payments are not enabled for this organization")

// stripeAccountForOrg returns the connected account to transact on ("" = platform
// account, only when Connect is not required).
func (h *StripeHandlers) stripeAccountForOrg(ctx context.Context, orgID string) (string, error) {
	if h.accounts == nil {
		return "", nil
	}
	a, err := h.accounts.Get(ctx, orgID)
	if errors.Is(err, payments.ErrNotFound) {
		if h.connectRequired {
			return "", errPaymentsNotEnabled
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !a.ChargesEnabled {
		return "", errPaymentsNotEnabled
	}
	return a.StripeAccountID, nil
}

func (h *StripeHandlers) writeAccountError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errPaymentsNotEnabled) {
		httpx.WriteError(w, r, http.StatusConflict, "payments_not_enabled", "Online payments are not set up for this club yet. A club administrator must finish Stripe onboarding.", nil)
		return
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve the club's payment account.", err)
}

// SetMetrics enables recording of aquify_webhook_processing_lag_seconds.
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

// stripeIdempotencyKeyFromRequest reads the caller's Idempotency-Key and scopes
// it to the org. Stripe keys are unique per Stripe account, so without the org
// prefix two clubs choosing the same key would replay each other's payments.
func stripeIdempotencyKeyFromRequest(w http.ResponseWriter, r *http.Request, orgID string) (string, bool) {
	key := strings.TrimSpace(r.Header.Get(stripeIdempotencyHeader))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required.", nil)
		return "", false
	}
	if len(key) > 128 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must be at most 128 characters.", nil)
		return "", false
	}
	return orgID + ":" + key, true
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
	if invoice.Status != "issued" && invoice.Status != "partially_paid" {
		return nil, errCheckoutDenied
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
	idempotencyKey, ok := stripeIdempotencyKeyFromRequest(w, r, orgID)
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
	if invoice.Status != "issued" && invoice.Status != "partially_paid" {
		httpx.WriteError(w, r, http.StatusConflict, "invoice_not_payable", "Invoice is not open for payment.", nil)
		return
	}
	if invoice.AmountDueCents <= 0 || req.AmountCents > invoice.AmountDueCents {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_amount", "amount_cents cannot exceed the invoice's remaining balance.", nil)
		return
	}

	// Build metadata for Stripe so webhook handlers can route the event back.
	meta := stripeMetadata(req.Metadata, req.InvoiceID, orgID)
	stripeAccount, err := h.stripeAccountForOrg(r.Context(), orgID)
	if err != nil {
		h.writeAccountError(w, r, err)
		return
	}

	result, err := h.client.CreatePaymentIntent(r.Context(), stripelib.PaymentIntentParams{
		StripeAccount:  stripeAccount,
		AmountCents:    req.AmountCents,
		Currency:       h.currencyFor(orgID),
		Description:    req.Description,
		Metadata:       meta,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.logger.Error("stripe: create payment intent failed", "error", err)
		writeStripeError(w, r, err, "Failed to create payment intent.")
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
	idempotencyKey, ok := stripeIdempotencyKeyFromRequest(w, r, orgID)
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
	stripeAccount, err := h.stripeAccountForOrg(r.Context(), orgID)
	if err != nil {
		h.writeAccountError(w, r, err)
		return
	}

	description := req.Description
	if invoice != nil {
		description = strings.TrimSpace(invoice.Notes)
		if description == "" {
			description = invoice.InvoiceNumber
		}
	}
	result, err := h.client.CreateCheckoutSession(r.Context(), stripelib.CheckoutSessionParams{
		StripeAccount:  stripeAccount,
		AmountCents:    req.AmountCents,
		Currency:       h.currencyFor(orgID),
		Description:    description,
		Metadata:       meta,
		SuccessURL:     req.SuccessURL,
		CancelURL:      req.CancelURL,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.logger.Error("stripe: create checkout session failed", "error", err)
		writeStripeError(w, r, err, "Failed to create checkout session.")
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

	if err := h.billing.RequireBillingWriteAccess(principal); err != nil {
		if errors.Is(err, billing.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Reconciliation failed.", err)
		return
	}

	stripeAccount, err := h.billing.StripeAccountFor(orgID)
	if err != nil {
		if errors.Is(err, billing.ErrPaymentAccountNotConnected) {
			err = errPaymentsNotEnabled
		}
		h.writeAccountError(w, r, err)
		return
	}
	stripeCents, err := h.client.FetchBalanceCents(r.Context(), stripeAccount)
	if err != nil {
		h.logger.Error("stripe: fetch balance failed", "error", err)
		writeStripeError(w, r, err, "Failed to fetch Stripe balance.")
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
	if err != nil && errors.Is(err, stripelib.ErrInvalidSignature) && h.connectWebhookSecret != "" {
		// The Connect endpoint (events on connected accounts) has its own secret.
		event, err = h.client.ParseWebhookEvent(body, sigHeader, h.connectWebhookSecret)
	}
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
	// Connect events are attributed by the connected account the event came from,
	// never by metadata alone: the account row is the authority for the org.
	attributionErr := h.attributeConnectEvent(r.Context(), event)
	// Only trust the org once attribution succeeded; a rejected Connect event
	// must not be stored under an org named by forged metadata.
	eventOrgID := ""
	if attributionErr == nil {
		eventOrgID = event.Metadata["org_id"]
	}
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
	processErr := attributionErr
	if processErr == nil {
		processErr = h.dispatchWebhookEvent(r.Context(), event)
	}
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
	errWebhookOrgMismatch,
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
	case "account.updated":
		return h.onAccountUpdated(ctx, event)
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

	_, _, err = h.billing.HandleStripePaymentIntentSucceededOn(
		orgID, event.Account, paymentIntentID, invoiceID, amountCents, "stripe-webhook",
	)
	return err
}

// onPaymentIntentFailed handles a payment_intent.payment_failed event.
func (h *StripeHandlers) onPaymentIntentFailed(_ context.Context, event *stripelib.WebhookEventData) error {
	paymentIntentID, _ := event.Object["id"].(string)
	if paymentIntentID == "" {
		return errors.New("payment_intent.payment_failed: missing payment intent ID")
	}
	return h.billing.HandleStripePaymentIntentFailedFor(event.Metadata["org_id"], paymentIntentID)
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

var errWebhookOrgMismatch = errors.New("webhook metadata org does not match the connected account's org")

// attributeConnectEvent makes the connected account authoritative for the org of a
// Connect event: metadata naming a different org is rejected (a forged or
// mis-routed event), and metadata is overwritten with the account's org. Platform
// events (no account) are left to the metadata routing used before Connect.
func (h *StripeHandlers) attributeConnectEvent(ctx context.Context, event *stripelib.WebhookEventData) error {
	if event.Account == "" {
		return nil
	}
	if h.accounts == nil {
		return fmt.Errorf("%w: connect event for account %s but connected accounts are not configured", errWebhookUnattributable, event.Account)
	}
	acct, err := h.accounts.ByStripeAccount(ctx, event.Account)
	if errors.Is(err, payments.ErrNotFound) {
		return fmt.Errorf("%w: connected account %s belongs to no organization", errWebhookUnattributable, event.Account)
	}
	if err != nil {
		return err
	}
	if m := event.Metadata["org_id"]; m != "" && m != acct.OrgID {
		h.logger.Error("stripe webhook: metadata org differs from the connected account's org",
			"event_id", event.ID, "account", event.Account, "metadata_org_id", m, "account_org_id", acct.OrgID)
		return errWebhookOrgMismatch
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}
	event.Metadata["org_id"] = acct.OrgID
	return nil
}

// onAccountUpdated keeps a club's capability flags in step with Stripe.
func (h *StripeHandlers) onAccountUpdated(ctx context.Context, event *stripelib.WebhookEventData) error {
	id, _ := event.Object["id"].(string)
	if id == "" {
		return nil
	}
	if h.accounts == nil {
		return nil
	}
	// Stripe does not guarantee event ordering, so ignore the payload flags and
	// apply the account's live state; a failed fetch is retried by Stripe.
	live, err := h.client.GetConnectedAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("account.updated: fetch live account %s: %w", id, err)
	}
	_, err = h.accounts.UpdateStatus(ctx, id, live.ChargesEnabled, live.PayoutsEnabled, live.DetailsSubmitted)
	if errors.Is(err, payments.ErrNotFound) {
		return fmt.Errorf("%w: account.updated for unknown account %s", errWebhookUnattributable, id)
	}
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

// writeStripeError maps a Stripe client failure to a response: transport
// timeouts become 503 stripe_unavailable, everything else stays 502 stripe_error.
func writeStripeError(w http.ResponseWriter, r *http.Request, err error, message string) {
	if stripelib.IsTimeout(err) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "stripe_unavailable", "Payment provider is temporarily unavailable. Please retry.", err)
		return
	}
	httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", message, nil)
}
