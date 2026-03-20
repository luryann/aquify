package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/go-chi/chi/v5"
)

type ReportsHandlers struct {
	service      *reports.Service
	auditService *audit.Service
}

func NewReportsHandlers(service *reports.Service, auditService *audit.Service) *ReportsHandlers {
	return &ReportsHandlers{service: service, auditService: auditService}
}

func (h *ReportsHandlers) RegisterRoutes(router chi.Router) {
	router.Route("/reports", func(r chi.Router) {
		r.Get("/membership-status", h.runMembershipStatus)
		r.Get("/meet-fees", h.runMeetFees)
		r.Get("/invoice-aging", h.runInvoiceAging)
		r.Get("/payment-reconciliation", h.runPaymentReconciliation)
		r.Get("/registration-status", h.runRegistrationStatus)
	})
}

func reportScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
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

func reportFormat(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
}

func (h *ReportsHandlers) appendAudit(principal *appmiddleware.Principal, orgID, action, resourceID string, metadata map[string]string) {
	if h.auditService == nil {
		return
	}
	_, _ = h.auditService.Append(audit.AppendRequest{
		OrgID:        orgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   principal.Roles,
		Action:       action,
		ResourceType: "report",
		ResourceID:   resourceID,
		Metadata:     metadata,
	})
}

func mapReportError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, reports.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to run reports.", nil)
		return true
	default:
		return false
	}
}

func writeCSVArtifactResponse(w http.ResponseWriter, report any, artifact *reports.StoredArtifact) {
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"report":   report,
		"artifact": artifact,
		"format":   "csv",
		"delivery": "artifact",
	})
}

func (h *ReportsHandlers) runMembershipStatus(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := reportScopeFromRequest(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunMembershipStatusReport(orgID, principal)
	if err != nil {
		if !mapReportError(w, r, err) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate membership status report.", nil)
		}
		return
	}
	h.appendAudit(principal, orgID, audit.ActionReportMembershipStatusRun, "membership_status", map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   fmt.Sprintf("%d", report.RowCount),
	})

	if reportFormat(r) == "csv" {
		doc, err := reports.BuildCSV(
			fmt.Sprintf("membership-status-%s.csv", report.GeneratedAt.Format("20060102")),
			[]string{"household_id", "household_name", "household_status", "primary_email", "primary_phone_e164", "swimmer_id", "swimmer_name", "preferred_name", "registration_status", "primary_group_code", "primary_group_name", "group_count", "last_profile_update_at"},
			func() [][]string {
				rows := make([][]string, 0, len(report.Rows))
				for _, row := range report.Rows {
					rows = append(rows, []string{row.HouseholdID, row.HouseholdName, row.HouseholdStatus, row.PrimaryEmail, row.PrimaryPhoneE164, row.SwimmerID, row.SwimmerName, row.PreferredName, row.RegistrationStatus, row.PrimaryGroupCode, row.PrimaryGroupName, fmt.Sprintf("%d", row.GroupCount), row.LastProfileUpdateAt.Format(time.RFC3339)})
				}
				return rows
			}(),
		)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", nil)
			return
		}
		artifact, err := h.service.StoreCSVArtifact(orgID, "membership_status", report.GeneratedAt, doc)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", nil)
			return
		}
		writeCSVArtifactResponse(w, report, artifact)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *ReportsHandlers) runMeetFees(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := reportScopeFromRequest(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunMeetFeesReport(orgID, principal)
	if err != nil {
		if !mapReportError(w, r, err) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate meet fees report.", nil)
		}
		return
	}
	h.appendAudit(principal, orgID, audit.ActionReportMeetFeesRun, "meet_fees", map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   fmt.Sprintf("%d", report.RowCount),
	})

	if reportFormat(r) == "csv" {
		doc, err := reports.BuildCSV(
			fmt.Sprintf("meet-fees-%s.csv", report.GeneratedAt.Format("20060102")),
			[]string{"invoice_id", "invoice_number", "invoice_status", "household_id", "meet_id", "meet_name", "swimmer_id", "swimmer_name", "line_item_id", "line_description", "quantity", "unit_amount_cents", "total_cents", "due_date", "issued_at"},
			func() [][]string {
				rows := make([][]string, 0, len(report.Rows))
				for _, row := range report.Rows {
					issuedAt := ""
					if row.IssuedAt != nil {
						issuedAt = row.IssuedAt.Format(time.RFC3339)
					}
					rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.InvoiceStatus, row.HouseholdID, row.MeetID, row.MeetName, row.SwimmerID, row.SwimmerName, row.LineItemID, row.LineDescription, fmt.Sprintf("%d", row.Quantity), fmt.Sprintf("%d", row.UnitAmountCents), fmt.Sprintf("%d", row.TotalCents), row.DueDate, issuedAt})
				}
				return rows
			}(),
		)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", nil)
			return
		}
		artifact, err := h.service.StoreCSVArtifact(orgID, "meet_fees", report.GeneratedAt, doc)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", nil)
			return
		}
		writeCSVArtifactResponse(w, report, artifact)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *ReportsHandlers) runInvoiceAging(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := reportScopeFromRequest(w, r)
	if !ok {
		return
	}
	asOf := time.Time{}
	if raw := strings.TrimSpace(r.URL.Query().Get("as_of")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_as_of", "as_of must use YYYY-MM-DD format.", nil)
			return
		}
		asOf = parsed.UTC()
	}
	report, err := h.service.RunInvoiceAgingReport(orgID, principal, asOf)
	if err != nil {
		if !mapReportError(w, r, err) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate invoice aging report.", nil)
		}
		return
	}
	h.appendAudit(principal, orgID, audit.ActionReportInvoiceAgingRun, "invoice_aging", map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   fmt.Sprintf("%d", report.RowCount),
		"as_of":  report.AsOfDate,
	})

	if reportFormat(r) == "csv" {
		doc, err := reports.BuildCSV(
			fmt.Sprintf("invoice-aging-%s.csv", report.GeneratedAt.Format("20060102")),
			[]string{"invoice_id", "invoice_number", "household_id", "status", "due_date", "days_past_due", "aging_bucket", "total_cents", "amount_paid_cents", "amount_due_cents", "is_outstanding"},
			func() [][]string {
				rows := make([][]string, 0, len(report.Rows))
				for _, row := range report.Rows {
					rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.HouseholdID, row.Status, row.DueDate, fmt.Sprintf("%d", row.DaysPastDue), row.AgingBucket, fmt.Sprintf("%d", row.TotalCents), fmt.Sprintf("%d", row.AmountPaidCents), fmt.Sprintf("%d", row.AmountDueCents), fmt.Sprintf("%t", row.IsOutstanding)})
				}
				return rows
			}(),
		)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", nil)
			return
		}
		artifact, err := h.service.StoreCSVArtifact(orgID, "invoice_aging", report.GeneratedAt, doc)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", nil)
			return
		}
		writeCSVArtifactResponse(w, report, artifact)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *ReportsHandlers) runPaymentReconciliation(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := reportScopeFromRequest(w, r)
	if !ok {
		return
	}
	stripeReportedCents, err := reports.ParseStripeReportedCents(r.URL.Query().Get("stripe_reported_cents"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_stripe_reported_cents", err.Error(), nil)
		return
	}
	report, err := h.service.RunPaymentReconciliationReport(orgID, principal, stripeReportedCents)
	if err != nil {
		if !mapReportError(w, r, err) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate payment reconciliation report.", nil)
		}
		return
	}
	metadata := map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   fmt.Sprintf("%d", report.RowCount),
	}
	if stripeReportedCents != nil {
		metadata["stripe_reported_cents"] = fmt.Sprintf("%d", *stripeReportedCents)
	}
	h.appendAudit(principal, orgID, audit.ActionReportPaymentReconciliationRun, "payment_reconciliation", metadata)

	if reportFormat(r) == "csv" {
		doc, err := reports.BuildCSV(
			fmt.Sprintf("payment-reconciliation-%s.csv", report.GeneratedAt.Format("20060102")),
			[]string{"invoice_id", "invoice_number", "payment_id", "method", "payment_status", "payment_amount_cents", "refunded_cents", "net_collected_cents", "ledger_payment_cents", "ledger_refund_cents", "is_reconciled", "reconciliation_notes"},
			func() [][]string {
				rows := make([][]string, 0, len(report.Rows))
				for _, row := range report.Rows {
					rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.PaymentID, row.Method, row.PaymentStatus, fmt.Sprintf("%d", row.PaymentAmountCents), fmt.Sprintf("%d", row.RefundedCents), fmt.Sprintf("%d", row.NetCollectedCents), fmt.Sprintf("%d", row.LedgerPaymentCents), fmt.Sprintf("%d", row.LedgerRefundCents), fmt.Sprintf("%t", row.IsReconciled), row.ReconciliationNotes})
				}
				return rows
			}(),
		)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", nil)
			return
		}
		artifact, err := h.service.StoreCSVArtifact(orgID, "payment_reconciliation", report.GeneratedAt, doc)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", nil)
			return
		}
		writeCSVArtifactResponse(w, report, artifact)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func (h *ReportsHandlers) runRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := reportScopeFromRequest(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunRegistrationStatusReport(orgID, principal)
	if err != nil {
		if !mapReportError(w, r, err) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate registration status report.", nil)
		}
		return
	}
	h.appendAudit(principal, orgID, audit.ActionReportRegistrationStatusRun, "registration_status", map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   fmt.Sprintf("%d", report.RowCount),
	})

	if reportFormat(r) == "csv" {
		doc, err := reports.BuildCSV(
			fmt.Sprintf("registration-status-%s.csv", report.GeneratedAt.Format("20060102")),
			[]string{"registration_id", "season", "status", "registration_date", "group_id", "group_code", "group_name", "household_id", "household_name", "swimmer_id", "swimmer_name", "waiver_count", "last_waiver_accepted_at"},
			func() [][]string {
				rows := make([][]string, 0, len(report.Rows))
				for _, row := range report.Rows {
					lastWaiver := ""
					if row.LastWaiverAcceptedAt != nil {
						lastWaiver = row.LastWaiverAcceptedAt.Format(time.RFC3339)
					}
					rows = append(rows, []string{row.RegistrationID, row.Season, row.Status, row.RegistrationDate, row.GroupID, row.GroupCode, row.GroupName, row.HouseholdID, row.HouseholdName, row.SwimmerID, row.SwimmerName, fmt.Sprintf("%d", row.WaiverCount), lastWaiver})
				}
				return rows
			}(),
		)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", nil)
			return
		}
		artifact, err := h.service.StoreCSVArtifact(orgID, "registration_status", report.GeneratedAt, doc)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", nil)
			return
		}
		writeCSVArtifactResponse(w, report, artifact)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

func defaultReportFormat(format string) string {
	if format == "" {
		return "json"
	}
	return format
}
