package handlers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
)

// Audit actions for club-finance writes and exports.
const (
	auditFinanceScheduleCreate   = "billing.fee_schedule.create"
	auditFinanceScheduleUpdate   = "billing.fee_schedule.update"
	auditFinanceScheduleArchive  = "billing.fee_schedule.archive"
	auditFinanceRuleCreate       = "billing.discount_rule.create"
	auditFinanceRuleUpdate       = "billing.discount_rule.update"
	auditFinanceRuleArchive      = "billing.discount_rule.archive"
	auditFinanceAdjustmentCreate = "billing.adjustment.create"
	auditFinanceAdjustmentVoid   = "billing.adjustment.void"
	auditFinanceSettingsUpdate   = "billing.settings.update"
	auditFinanceRunCommit        = "billing.run.commit"
	auditFinanceLateFeesApply    = "billing.late_fees.apply"
	auditFinanceCreditCreate     = "billing.credit.create"
	auditFinanceRefundToCredit   = "billing.refund_to_credit.create"
	auditFinanceReportExport     = "billing.report.export"
)

// RegisterFinanceRoutes mounts fee schedules, discount rules, adjustments,
// billing runs, late fees, credits, statements, and finance reports.
func (h *BillingHandlers) RegisterFinanceRoutes(router chi.Router) {
	router.Get("/billing/fee-schedules", h.listFeeSchedules)
	router.Post("/billing/fee-schedules", h.createFeeSchedule)
	router.Get("/billing/fee-schedules/{scheduleID}", h.getFeeSchedule)
	router.Put("/billing/fee-schedules/{scheduleID}", h.updateFeeSchedule)
	router.Delete("/billing/fee-schedules/{scheduleID}", h.archiveFeeSchedule)

	router.Get("/billing/discount-rules", h.listDiscountRules)
	router.Post("/billing/discount-rules", h.createDiscountRule)
	router.Put("/billing/discount-rules/{ruleID}", h.updateDiscountRule)
	router.Delete("/billing/discount-rules/{ruleID}", h.archiveDiscountRule)

	router.Get("/billing/adjustments", h.listAdjustments)
	router.Post("/billing/adjustments", h.createAdjustment)
	router.Post("/billing/adjustments/{adjustmentID}/void", h.voidAdjustment)

	router.Get("/billing/settings", h.getBillingSettings)
	router.Put("/billing/settings", h.updateBillingSettings)

	router.Post("/billing/runs/preview", h.previewBillingRun)
	router.Post("/billing/runs", h.commitBillingRun)
	router.Get("/billing/runs", h.listBillingRuns)
	router.Get("/billing/runs/{runID}", h.getBillingRun)

	router.Post("/billing/late-fees/apply", h.applyLateFees)

	router.Get("/households/{householdID}/credits", h.getHouseholdCredit)
	router.Post("/households/{householdID}/credits", h.addHouseholdCredit)
	router.Post("/invoices/{invoiceID}/refunds-to-credit", h.refundToCredit)
	router.Get("/households/{householdID}/statement", h.getHouseholdStatement)
	router.Get("/households/{householdID}/upcoming-charges", h.getUpcomingCharges)

	router.Get("/billing/reports/aging", h.agingReport)
	router.Get("/billing/reports/revenue", h.revenueReport)
	router.Get("/billing/reports/outstanding", h.outstandingReport)
	router.Get("/billing/reports/deposits", h.depositsReport)
	router.Get("/billing/reports/quickbooks", h.quickBooksExport)
}

func mapFinanceError(w http.ResponseWriter, r *http.Request, err error) {
	var fieldErr *billing.FinanceValidationError
	var stale *billing.StalePreviewError
	switch {
	case errors.As(err, &fieldErr):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", fieldErr.Error(), map[string]any{"field": fieldErr.Field})
	case errors.As(err, &stale):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "preview_stale", "message": "The roster or schedules changed since this preview. Review the refreshed preview before committing."},
			"preview": stale.Preview,
		})
	case errors.Is(err, billing.ErrFeeScheduleNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "fee_schedule_not_found", "Fee schedule not found.", nil)
	case errors.Is(err, billing.ErrDiscountRuleNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "discount_rule_not_found", "Discount rule not found.", nil)
	case errors.Is(err, billing.ErrAdjustmentNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "adjustment_not_found", "Adjustment not found.", nil)
	case errors.Is(err, billing.ErrRunNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "billing_run_not_found", "Billing run not found.", nil)
	case errors.Is(err, billing.ErrAdjustmentNotPending):
		httpx.WriteError(w, r, http.StatusConflict, "adjustment_not_pending", "Only pending adjustments can be voided.", nil)
	case errors.Is(err, billing.ErrScheduleLocked):
		httpx.WriteError(w, r, http.StatusConflict, "fee_schedule_locked", "This schedule has billed charges; its kind, group, season, frequency and start date cannot change. Archive it and create a new schedule instead.", nil)
	case errors.Is(err, billing.ErrNothingToBill):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "nothing_to_bill", "Every eligible charge for this period is already billed.", nil)
	case errors.Is(err, billing.ErrInsufficientCredit):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "insufficient_credit", "The household does not have enough account credit.", nil)
	case errors.Is(err, billing.ErrHouseholdRequired):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "household_required", "This action requires an invoice linked to a household.", nil)
	case errors.Is(err, billing.ErrCreditPaymentRefundToCredit):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "credit_payment_refund_to_credit", "Account-credit payments can only be refunded back to account credit.", nil)
	case errors.Is(err, billing.ErrRosterUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "roster_unavailable", "Roster data is unavailable.", nil)
	default:
		mapBillingError(w, r, err)
	}
}

func (h *BillingHandlers) audit(r *http.Request, principal *appmiddleware.Principal, orgID, action, resourceType, resourceID string, before, after any, meta map[string]string) {
	if h.auditService == nil {
		return
	}
	_, _ = h.auditService.Append(audit.AppendRequest{
		OrgID: orgID, ActorUserID: principal.UserID, ActorRoles: principal.Roles, Action: action,
		ResourceType: resourceType, ResourceID: resourceID, Before: before, After: after, Metadata: meta,
	})
}

func requireFinanceMFA(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, verb string) bool {
	if principal == nil || !financeHasRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to "+verb+".", nil)
		return false
	}
	if !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA step-up is required to "+verb+".", nil)
		return false
	}
	return true
}

func financeHasRole(principal *appmiddleware.Principal, role string) bool {
	for _, r := range principal.Roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

func queryDate(r *http.Request, key string, fallback time.Time) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, true
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func writeCSV(w http.ResponseWriter, filename string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	_ = cw.WriteAll(rows)
}

func wantsCSV(r *http.Request) bool {
	return strings.EqualFold(r.URL.Query().Get("format"), "csv")
}

// ─── Fee schedules ───────────────────────────────────────────────────────────

func (h *BillingHandlers) listFeeSchedules(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	items, err := h.service.ListFeeSchedules(orgID, principal)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(items), "fee_schedules": items})
}

func (h *BillingHandlers) getFeeSchedule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	sc, err := h.service.GetFeeSchedule(orgID, principal, chi.URLParam(r, "scheduleID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"fee_schedule": sc})
}

func (h *BillingHandlers) createFeeSchedule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.FeeScheduleInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	sc, err := h.service.CreateFeeSchedule(orgID, principal.UserID, principal, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceScheduleCreate, "fee_schedule", sc.ID, nil, sc, nil)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"fee_schedule": sc})
}

func (h *BillingHandlers) updateFeeSchedule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.FeeScheduleInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	before, sc, err := h.service.UpdateFeeSchedule(orgID, principal.UserID, principal, chi.URLParam(r, "scheduleID"), input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceScheduleUpdate, "fee_schedule", sc.ID, before, sc, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"fee_schedule": sc})
}

func (h *BillingHandlers) archiveFeeSchedule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	sc, err := h.service.ArchiveFeeSchedule(orgID, principal.UserID, principal, chi.URLParam(r, "scheduleID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceScheduleArchive, "fee_schedule", sc.ID, nil, sc, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"fee_schedule": sc})
}

// ─── Discount rules ──────────────────────────────────────────────────────────

func (h *BillingHandlers) listDiscountRules(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	items, err := h.service.ListDiscountRules(orgID, principal)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(items), "discount_rules": items})
}

func (h *BillingHandlers) createDiscountRule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.DiscountRuleInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	rule, err := h.service.CreateDiscountRule(orgID, principal.UserID, principal, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceRuleCreate, "discount_rule", rule.ID, nil, rule, nil)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"discount_rule": rule})
}

func (h *BillingHandlers) updateDiscountRule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.DiscountRuleInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	before, rule, err := h.service.UpdateDiscountRule(orgID, principal.UserID, principal, chi.URLParam(r, "ruleID"), input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceRuleUpdate, "discount_rule", rule.ID, before, rule, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"discount_rule": rule})
}

func (h *BillingHandlers) archiveDiscountRule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	rule, err := h.service.ArchiveDiscountRule(orgID, principal.UserID, principal, chi.URLParam(r, "ruleID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceRuleArchive, "discount_rule", rule.ID, nil, rule, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"discount_rule": rule})
}

// ─── Adjustments & settings ──────────────────────────────────────────────────

func (h *BillingHandlers) listAdjustments(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	items, err := h.service.ListAdjustments(orgID, principal, r.URL.Query().Get("status"), r.URL.Query().Get("household_id"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	page := parseListPagination(r, listPaginationOptions{})
	paged, total := paginateSlice(items, page)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(paged), "total": total, "limit": page.Limit, "offset": page.Offset, "adjustments": paged})
}

func (h *BillingHandlers) createAdjustment(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.AdjustmentInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	adj, err := h.service.CreateAdjustment(orgID, principal.UserID, principal, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceAdjustmentCreate, "billing_adjustment", adj.ID, nil, adj, map[string]string{"reason": adj.Reason})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"adjustment": adj})
}

func (h *BillingHandlers) voidAdjustment(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	adj, err := h.service.VoidAdjustment(orgID, principal.UserID, principal, chi.URLParam(r, "adjustmentID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceAdjustmentVoid, "billing_adjustment", adj.ID, nil, adj, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"adjustment": adj})
}

func (h *BillingHandlers) getBillingSettings(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetBillingSettings(orgID, principal)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"settings": st})
}

func (h *BillingHandlers) updateBillingSettings(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.BillingSettingsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	before, st, err := h.service.UpdateBillingSettings(orgID, principal.UserID, principal, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceSettingsUpdate, "billing_settings", orgID, before, st, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"settings": st})
}

// ─── Runs & late fees ────────────────────────────────────────────────────────

func (h *BillingHandlers) previewBillingRun(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.RunInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	preview, err := h.service.PreviewBillingRun(orgID, principal, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preview": preview})
}

func (h *BillingHandlers) commitBillingRun(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.RequireBillingWriteAccess(principal); err != nil {
		mapFinanceError(w, r, err)
		return
	}
	var input billing.RunInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	result, err := h.service.CommitBillingRun(orgID, principal.UserID, principal, input, billing.RunTriggerManual)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceRunCommit, "billing_run", result.Run.ID, nil, result.Run, map[string]string{
		"as_of": result.Run.AsOf, "invoice_count": strconv.Itoa(result.Run.InvoiceCount), "net_cents": strconv.Itoa(result.Run.NetCents),
	})
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *BillingHandlers) listBillingRuns(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	runs, err := h.service.ListBillingRuns(orgID, principal)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	page := parseListPagination(r, listPaginationOptions{})
	paged, total := paginateSlice(runs, page)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(paged), "total": total, "limit": page.Limit, "offset": page.Offset, "runs": paged})
}

func (h *BillingHandlers) getBillingRun(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	detail, err := h.service.GetBillingRun(orgID, principal, chi.URLParam(r, "runID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h *BillingHandlers) applyLateFees(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	var input struct {
		AsOf string `json:"as_of"`
	}
	if err := decodeOptionalJSONBody(w, r, &input); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", "Request body is invalid.", nil)
		return
	}
	asOf := time.Now().UTC()
	if strings.TrimSpace(input.AsOf) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.AsOf))
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "as_of must use YYYY-MM-DD.", nil)
			return
		}
		asOf = parsed
	}
	result, err := h.service.ApplyLateFees(orgID, principal.UserID, principal, asOf)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	if result.AppliedCount > 0 {
		h.audit(r, principal, orgID, auditFinanceLateFeesApply, "billing_late_fees", orgID, nil, result, nil)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"result": result})
}

// ─── Credits, statement, upcoming ────────────────────────────────────────────

func (h *BillingHandlers) getHouseholdCredit(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	summary, err := h.service.GetHouseholdCredit(orgID, principal, chi.URLParam(r, "householdID"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"credit": summary})
}

func (h *BillingHandlers) addHouseholdCredit(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !requireFinanceMFA(w, r, principal, "change account credit") {
		return
	}
	var input billing.ManualCreditInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	householdID := chi.URLParam(r, "householdID")
	entry, applied, err := h.service.AddManualCredit(orgID, principal.UserID, principal, householdID, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceCreditCreate, "household_credit", entry.ID, nil, entry, map[string]string{
		"household_id": householdID, "reason": entry.Reason, "applied_invoices": strings.Join(applied, ","),
	})
	if applied == nil {
		applied = []string{}
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"entry": entry, "applied_invoice_ids": applied})
}

func (h *BillingHandlers) refundToCredit(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !requirePrivilegedBillingAction(w, r, principal, appmiddleware.ActionBillingRefund, "create refunds") {
		return
	}
	var input billing.CreateRefundInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	refund, ledgerEv, err := h.service.RefundToCredit(orgID, principal.UserID, principal, invoiceID, input)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.audit(r, principal, orgID, auditFinanceRefundToCredit, "refund", refund.ID, nil, refund, map[string]string{"invoice_id": invoiceID, "reason": input.Reason})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"refund": refund, "ledger_event": ledgerEv})
}

func (h *BillingHandlers) getHouseholdStatement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	from, okFrom := queryDate(r, "from", time.Time{})
	to, okTo := queryDate(r, "to", time.Now().UTC())
	if !okFrom || !okTo {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "from and to must use YYYY-MM-DD.", nil)
		return
	}
	st, err := h.service.HouseholdStatement(orgID, principal, chi.URLParam(r, "householdID"), from, to)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"statement": st})
}

func (h *BillingHandlers) getUpcomingCharges(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	days := parsePositiveInt(r.URL.Query().Get("days"), 90)
	up, err := h.service.HouseholdUpcomingCharges(orgID, principal, chi.URLParam(r, "householdID"), time.Now().UTC(), days)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"upcoming": up})
}

// ─── Reports ─────────────────────────────────────────────────────────────────

func (h *BillingHandlers) auditExport(r *http.Request, principal *appmiddleware.Principal, orgID, report string) {
	h.audit(r, principal, orgID, auditFinanceReportExport, "billing_report", report, nil, nil, map[string]string{"report": report, "query": r.URL.RawQuery})
}

func reportRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	from, okFrom := queryDate(r, "from", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0))
	to, okTo := queryDate(r, "to", now)
	if !okFrom || !okTo || to.Before(from) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "from/to must use YYYY-MM-DD with from on or before to.", nil)
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func (h *BillingHandlers) agingReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	asOf, okDate := queryDate(r, "as_of", time.Now().UTC())
	if !okDate {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "as_of must use YYYY-MM-DD.", nil)
		return
	}
	report, err := h.service.AgingReport(orgID, principal, asOf)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	if wantsCSV(r) {
		h.auditExport(r, principal, orgID, "aging")
		writeCSV(w, "ar-aging-"+report.AsOf+".csv", report.CSV())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *BillingHandlers) revenueReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	from, to, ok := reportRange(w, r)
	if !ok {
		return
	}
	report, err := h.service.RevenueReport(orgID, principal, from, to)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	if wantsCSV(r) {
		h.auditExport(r, principal, orgID, "revenue")
		writeCSV(w, "revenue-"+report.From+"-"+report.To+".csv", report.CSV())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *BillingHandlers) outstandingReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	report, err := h.service.OutstandingReport(orgID, principal)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	if wantsCSV(r) {
		h.auditExport(r, principal, orgID, "outstanding")
		writeCSV(w, "outstanding-balances.csv", report.CSV())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *BillingHandlers) depositsReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	from, to, ok := reportRange(w, r)
	if !ok {
		return
	}
	report, err := h.service.DepositsReport(orgID, principal, from, to, r.URL.Query().Get("period"))
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	if wantsCSV(r) {
		h.auditExport(r, principal, orgID, "deposits")
		writeCSV(w, "deposits-"+report.From+"-"+report.To+".csv", report.CSV())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *BillingHandlers) quickBooksExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := billingScopeFromRequest(w, r)
	if !ok {
		return
	}
	from, to, ok := reportRange(w, r)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "payments" {
		kind = "invoices"
	}
	rows, err := h.service.QuickBooksCSV(orgID, principal, kind, from, to)
	if err != nil {
		mapFinanceError(w, r, err)
		return
	}
	h.auditExport(r, principal, orgID, "quickbooks_"+kind)
	writeCSV(w, fmt.Sprintf("quickbooks-%s-%s-%s.csv", kind, from.Format("2006-01-02"), to.Format("2006-01-02")), rows)
}
