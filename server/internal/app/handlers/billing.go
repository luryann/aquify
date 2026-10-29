package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// BillingHandlers handles HTTP requests for the billing domain.
type BillingHandlers struct {
	service      *billing.Service
	auditService *audit.Service
	stripeClient stripelib.Client
	validate     *validator.Validate
}

// NewBillingHandlers returns a new BillingHandlers.
func NewBillingHandlers(svc *billing.Service) *BillingHandlers {
	return &BillingHandlers{
		service:  svc,
		validate: validator.New(),
	}
}

// NewBillingHandlersWithAudit returns a BillingHandlers with audit logging enabled.
func NewBillingHandlersWithAudit(svc *billing.Service, auditSvc *audit.Service) *BillingHandlers {
	return &BillingHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     validator.New(),
	}
}

// NewBillingHandlersWithAuditAndStripe returns a BillingHandlers with audit
// logging and optional Stripe runtime refund execution.
func NewBillingHandlersWithAuditAndStripe(svc *billing.Service, auditSvc *audit.Service, stripeClient stripelib.Client) *BillingHandlers {
	return &BillingHandlers{
		service:      svc,
		auditService: auditSvc,
		stripeClient: stripeClient,
		validate:     validator.New(),
	}
}

// RegisterRoutes mounts all billing routes on the given router.
func (h *BillingHandlers) RegisterRoutes(router chi.Router) {
	// Invoice CRUD
	router.Post("/invoices", h.createInvoice)
	router.Get("/invoices", h.listInvoices)
	router.Get("/invoices/{invoiceID}", h.getInvoice)

	// Invoice items
	router.Post("/invoices/{invoiceID}/items", h.addInvoiceItem)
	router.Delete("/invoices/{invoiceID}/items/{itemID}", h.removeInvoiceItem)
	router.Get("/invoices/{invoiceID}/items", h.listInvoiceItems)

	// Invoice lifecycle
	router.Post("/invoices/{invoiceID}/issue", h.issueInvoice)
	router.Post("/invoices/{invoiceID}/void", h.voidInvoice)
	router.Post("/invoices/{invoiceID}/write-off", h.writeOffInvoice)

	// Payments
	router.Get("/payments", h.listOrgPayments)
	router.Post("/invoices/{invoiceID}/payments", h.recordPayment)
	router.Get("/invoices/{invoiceID}/payments", h.listPayments)

	// Refunds
	router.Post("/invoices/{invoiceID}/refunds", h.createRefund)
	router.Get("/invoices/{invoiceID}/refunds", h.listRefunds)

	// Household invoices
	router.Get("/households/{householdID}/invoices", h.listHouseholdInvoices)
	router.Get("/households/{householdID}/autopay", h.getHouseholdAutopay)
	router.Put("/households/{householdID}/autopay", h.upsertHouseholdAutopay)
	router.Post("/households/{householdID}/autopay/opt-out", h.optOutHouseholdAutopay)
	router.Get("/autopay/households", h.listHouseholdAutopay)

	// Ledger
	router.Get("/ledger-events", h.listLedgerEvents)
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func (h *BillingHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateUnprocessable(w, r, dst, h.validate)
}

func mapBillingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, billing.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
	case errors.Is(err, billing.ErrInvoiceNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "invoice_not_found", "Invoice not found.", nil)
	case errors.Is(err, billing.ErrInvoiceItemNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "item_not_found", "Invoice item not found.", nil)
	case errors.Is(err, billing.ErrPaymentNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "payment_not_found", "Payment not found.", nil)
	case errors.Is(err, billing.ErrRefundNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "refund_not_found", "Refund not found.", nil)
	case errors.Is(err, billing.ErrInvoiceNotDraft):
		httpx.WriteError(w, r, http.StatusConflict, "invoice_not_draft", "Invoice must be in draft status.", nil)
	case errors.Is(err, billing.ErrInvoiceNotIssuable):
		httpx.WriteError(w, r, http.StatusConflict, "invoice_not_issuable", "Invoice must be in draft status to issue.", nil)
	case errors.Is(err, billing.ErrInvoiceEmpty):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invoice_empty", "Add at least one line item before issuing this invoice.", nil)
	case errors.Is(err, billing.ErrInvoiceNotVoidable):
		httpx.WriteError(w, r, http.StatusConflict, "invoice_not_voidable", "Invoice must be in issued status to void.", nil)
	case errors.Is(err, billing.ErrInvoiceNotWriteOffable):
		httpx.WriteError(w, r, http.StatusConflict, "invoice_not_write_offable", "Invoice must be in issued or partially_paid status to write off.", nil)
	case errors.Is(err, billing.ErrInvalidTransition):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "This state transition is not allowed.", nil)
	case errors.Is(err, billing.ErrPaymentExceedsBalance):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "payment_exceeds_balance", "Payment amount exceeds outstanding balance.", nil)
	case errors.Is(err, billing.ErrRefundExceedsPayment):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "refund_exceeds_payment", "Refund amount exceeds original payment amount.", nil)
	case errors.Is(err, billing.ErrRefundRequiresReason):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "refund_requires_reason", "A reason is required for refund requests.", nil)
	case errors.Is(err, billing.ErrDuplicateInvoiceNumber):
		httpx.WriteError(w, r, http.StatusConflict, "duplicate_invoice_number", "Invoice number already exists for this organisation.", nil)
	case errors.Is(err, billing.ErrAutopayPaymentMethodRequired):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "autopay_payment_method_required", "Payment method details are required to enable autopay.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.", err)
	}
}

// ─── Invoice handlers ─────────────────────────────────────────────────────────

func (h *BillingHandlers) createInvoice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var input billing.CreateInvoiceInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	inv, err := h.service.CreateInvoice(orgID, principal.UserID, principal, input)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"invoice": inv})
}

func (h *BillingHandlers) getInvoice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	inv, err := h.service.GetInvoice(orgID, principal, invoiceID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoice": inv})
}

func (h *BillingHandlers) listInvoices(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	filters := billing.ListInvoicesFilters{
		HouseholdID: r.URL.Query().Get("household_id"),
		Status:      r.URL.Query().Get("status"),
	}

	invoices, err := h.service.ListInvoices(orgID, principal, filters)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(invoices), "invoices": invoices})
}

// ─── Invoice item handlers ────────────────────────────────────────────────────

func (h *BillingHandlers) addInvoiceItem(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	var input billing.AddInvoiceItemInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	item, err := h.service.AddInvoiceItem(orgID, principal.UserID, principal, invoiceID, input)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (h *BillingHandlers) removeInvoiceItem(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	itemID := chi.URLParam(r, "itemID")

	if err := h.service.RemoveInvoiceItem(orgID, principal.UserID, principal, invoiceID, itemID); err != nil {
		mapBillingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *BillingHandlers) listInvoiceItems(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	items, err := h.service.ListInvoiceItems(orgID, principal, invoiceID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoice_id": invoiceID, "count": len(items), "items": items})
}

// ─── Invoice lifecycle handlers ───────────────────────────────────────────────

// privilegedBillingAction gates an invoice transition behind a permission
// (and MFA) check; verb completes "You are not allowed to ...".
type privilegedBillingAction struct {
	action appmiddleware.Action
	verb   string
}

// invoiceTransition runs an issue/void/write-off style action on the invoice in
// the URL and records the before/after invoice in the audit log.
func (h *BillingHandlers) invoiceTransition(
	w http.ResponseWriter,
	r *http.Request,
	auditAction string,
	gate *privilegedBillingAction,
	do func(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string) (*billing.Invoice, *billing.LedgerEvent, error),
) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if gate != nil && !requirePrivilegedBillingAction(w, r, principal, gate.action, gate.verb) {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	// Capture the invoice before the change for the audit diff.
	var beforeInv interface{}
	if h.auditService != nil {
		if before, err := h.service.GetInvoice(orgID, principal, invoiceID); err == nil {
			beforeInv = before
		}
	}

	inv, ledgerEv, err := do(orgID, principal.UserID, principal, invoiceID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}

	recordAudit(h.auditService, principal, orgID, auditAction, "invoice", invoiceID, beforeInv, inv, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoice": inv, "ledger_event": ledgerEv})
}

func (h *BillingHandlers) issueInvoice(w http.ResponseWriter, r *http.Request) {
	h.invoiceTransition(w, r, audit.ActionBillingInvoiceIssue, nil, h.service.IssueInvoice)
}

func (h *BillingHandlers) voidInvoice(w http.ResponseWriter, r *http.Request) {
	h.invoiceTransition(w, r, audit.ActionBillingInvoiceVoid, &privilegedBillingAction{appmiddleware.ActionBillingVoid, "void invoices"}, h.service.VoidInvoice)
}

func (h *BillingHandlers) writeOffInvoice(w http.ResponseWriter, r *http.Request) {
	h.invoiceTransition(w, r, audit.ActionBillingInvoiceWriteOff, &privilegedBillingAction{appmiddleware.ActionBillingWriteOff, "write off invoices"}, h.service.WriteOffInvoice)
}

// ─── Payment handlers ─────────────────────────────────────────────────────────

func (h *BillingHandlers) recordPayment(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !requirePrivilegedBillingAction(w, r, principal, appmiddleware.ActionBillingPaymentRecord, "record payments") {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	var input billing.RecordPaymentInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	payment, ledgerEv, err := h.service.RecordPayment(orgID, principal.UserID, principal, invoiceID, input)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	recordAudit(h.auditService, principal, orgID, audit.ActionBillingPaymentRecord, "payment", payment.ID, nil, payment,
		map[string]string{"invoice_id": invoiceID, "method": payment.Method})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"payment": payment, "ledger_event": ledgerEv})
}

func (h *BillingHandlers) listPayments(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	payments, err := h.service.ListPayments(orgID, principal, invoiceID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoice_id": invoiceID, "count": len(payments), "payments": payments})
}

func (h *BillingHandlers) listOrgPayments(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	payments, err := h.service.ListPaymentsForOrg(orgID, principal, billing.ListPaymentsFilters{
		InvoiceID: r.URL.Query().Get("invoice_id"),
		Status:    r.URL.Query().Get("status"),
	})
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(payments), "payments": payments})
}

// ─── Refund handlers ──────────────────────────────────────────────────────────

func (h *BillingHandlers) createRefund(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !requirePrivilegedBillingAction(w, r, principal, appmiddleware.ActionBillingRefund, "create refunds") {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	var input billing.CreateRefundInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	preflight, err := h.service.ValidateRefundPreflight(orgID, principal, invoiceID, input)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}

	stripeRefundIssued := false
	if preflight.Payment.Method == "stripe" && preflight.Payment.StripePaymentIntentID != "" && h.stripeClient != nil {
		refundResult, err := h.stripeClient.CreateRefund(r.Context(), stripelib.RefundParams{
			PaymentIntentID: preflight.Payment.StripePaymentIntentID,
			AmountCents:     input.AmountCents,
			Reason:          input.Reason,
			IdempotencyKey:  fmt.Sprintf("refund:%s:%s:%s:%d", orgID, invoiceID, preflight.Payment.ID, input.AmountCents),
		})
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Failed to create Stripe refund.", err)
			return
		}
		input.StripeRefundID = refundResult.ID
		stripeRefundIssued = true
	}

	refund, ledgerEv, err := h.service.CreateRefund(orgID, principal.UserID, principal, invoiceID, input)
	if err != nil {
		if stripeRefundIssued {
			slog.Default().Error("stripe refund succeeded but local persistence failed",
				"org_id", orgID,
				"invoice_id", invoiceID,
				"payment_id", input.PaymentID,
				"stripe_refund_id", input.StripeRefundID,
				"error", err,
			)
			httpx.WriteError(w, r, http.StatusBadGateway, "refund_persistence_failed", "Stripe refund succeeded but local persistence failed. Operators must reconcile this refund.", nil)
			return
		}
		mapBillingError(w, r, err)
		return
	}

	if h.auditService != nil {
		meta := map[string]string{"invoice_id": invoiceID}
		if input.Reason != "" {
			meta["reason"] = input.Reason
		}
		recordAudit(h.auditService, principal, orgID, audit.ActionBillingRefundCreate, "refund", refund.ID, nil, refund, meta)
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"refund": refund, "ledger_event": ledgerEv})
}

func (h *BillingHandlers) listRefunds(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")

	refunds, err := h.service.ListRefunds(orgID, principal, invoiceID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoice_id": invoiceID, "count": len(refunds), "refunds": refunds})
}

// ─── Household invoices handler ───────────────────────────────────────────────

func (h *BillingHandlers) listHouseholdInvoices(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID := chi.URLParam(r, "householdID")

	filters := billing.ListInvoicesFilters{
		HouseholdID: householdID,
		Status:      r.URL.Query().Get("status"),
	}

	invoices, err := h.service.ListInvoices(orgID, principal, filters)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"household_id": householdID, "count": len(invoices), "invoices": invoices})
}

// ─── Autopay handlers ─────────────────────────────────────────────────────────

// autopayBeforeState reads the household's autopay for the audit diff; nil when
// auditing is off or the read fails (the write then proceeds without a diff).
func (h *BillingHandlers) autopayBeforeState(orgID string, principal *appmiddleware.Principal, householdID string) any {
	if h.auditService == nil {
		return nil
	}
	before, err := h.service.GetHouseholdAutopayStatus(orgID, principal, householdID)
	if err != nil {
		return nil
	}
	return before
}

func (h *BillingHandlers) getHouseholdAutopay(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID := chi.URLParam(r, "householdID")

	status, err := h.service.GetHouseholdAutopayStatus(orgID, principal, householdID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"autopay": status})
}

func (h *BillingHandlers) listHouseholdAutopay(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	statuses, err := h.service.ListHouseholdAutopayStatuses(orgID, principal, r.URL.Query()["household_id"])
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(statuses), "autopay": statuses})
}

func (h *BillingHandlers) upsertHouseholdAutopay(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID := chi.URLParam(r, "householdID")

	beforeState := h.autopayBeforeState(orgID, principal, householdID)

	var input billing.UpsertHouseholdAutopayInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	status, err := h.service.UpsertHouseholdAutopay(orgID, principal.UserID, principal, householdID, input)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionBillingAutopayEnroll, "household_autopay", householdID, beforeState, status, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"autopay": status})
}

func (h *BillingHandlers) optOutHouseholdAutopay(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID := chi.URLParam(r, "householdID")

	beforeState := h.autopayBeforeState(orgID, principal, householdID)

	status, err := h.service.OptOutHouseholdAutopay(orgID, principal.UserID, principal, householdID)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionBillingAutopayOptOut, "household_autopay", householdID, beforeState, status, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"autopay": status})
}

// ─── Ledger handler ───────────────────────────────────────────────────────────

func (h *BillingHandlers) listLedgerEvents(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	filters := billing.ListLedgerFilters{
		InvoiceID: r.URL.Query().Get("invoice_id"),
		EventType: r.URL.Query().Get("event_type"),
	}

	events, err := h.service.ListLedgerEvents(orgID, principal, filters)
	if err != nil {
		mapBillingError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(events), "events": events})
}

func requirePrivilegedBillingAction(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, action appmiddleware.Action, verb string) bool {
	if err := appmiddleware.CheckPermission(principal, action); err != nil {
		switch {
		case errors.Is(err, appmiddleware.ErrMFARequired):
			httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA step-up is required to "+verb+".", nil)
		default:
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to "+verb+".", nil)
		}
		return false
	}
	return true
}
