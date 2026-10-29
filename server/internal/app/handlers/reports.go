package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
		r.Get("/invoice-detail", h.runInvoiceDetail)
		r.Get("/payment-reconciliation", h.runPaymentReconciliation)
		r.Get("/registration-status", h.runRegistrationStatus)
	})
}

func reportFormat(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
}

func (h *ReportsHandlers) appendAudit(principal *appmiddleware.Principal, orgID, action, resourceID string, metadata map[string]string) {
	recordAudit(h.auditService, principal, orgID, action, "report", resourceID, nil, nil, metadata)
}

func mapReportError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, reports.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to run reports.", nil)
		return true
	case errors.Is(err, reports.ErrInvoiceIDRequired):
		httpx.WriteError(w, r, http.StatusBadRequest, "invoice_id_required", "invoice_id is required.", nil)
		return true
	case errors.Is(err, reports.ErrInvoiceNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "invoice_not_found", "Invoice not found.", nil)
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

// reportOutput is everything a report endpoint needs to audit and render a result.
type reportOutput struct {
	kind        string // audit action's resource and the stored CSV artifact kind
	auditAction string
	auditID     string // audit resource id; defaults to kind
	auditExtra  map[string]string
	report      any
	generatedAt time.Time
	rowCount    int
	csvName     string
	csvHeader   []string
	csvRows     func() [][]string
}

// respond audits the run, then answers with the report JSON, or stores and
// returns a CSV artifact when ?format=csv.
func (h *ReportsHandlers) respond(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, orgID string, out reportOutput) {
	metadata := map[string]string{
		"format": defaultReportFormat(reportFormat(r)),
		"rows":   strconv.Itoa(out.rowCount),
	}
	for k, v := range out.auditExtra {
		metadata[k] = v
	}
	auditID := out.auditID
	if auditID == "" {
		auditID = out.kind
	}
	h.appendAudit(principal, orgID, out.auditAction, auditID, metadata)

	if reportFormat(r) != "csv" {
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": out.report})
		return
	}
	doc, err := reports.BuildCSV(out.csvName, out.csvHeader, out.csvRows())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "csv_generation_failed", "Failed to build CSV report.", err)
		return
	}
	artifact, err := h.service.StoreCSVArtifact(orgID, out.kind, out.generatedAt, doc)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "artifact_persist_failed", "Failed to persist CSV report artifact.", err)
		return
	}
	writeCSVArtifactResponse(w, out.report, artifact)
}

// failReport writes the mapped service error, or a generic 500 naming the report.
func failReport(w http.ResponseWriter, r *http.Request, err error, label string) {
	if !mapReportError(w, r, err) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate "+label+" report.", err)
	}
}

func csvFilename(slug string, generatedAt time.Time) string {
	return fmt.Sprintf("%s-%s.csv", slug, generatedAt.Format("20060102"))
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (h *ReportsHandlers) runMembershipStatus(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunMembershipStatusReport(orgID, principal)
	if err != nil {
		failReport(w, r, err, "membership status")
		return
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "membership_status", auditAction: audit.ActionReportMembershipStatusRun,
		report: report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   csvFilename("membership-status", report.GeneratedAt),
		csvHeader: []string{"household_id", "household_name", "household_status", "primary_email", "primary_phone_e164", "swimmer_id", "swimmer_name", "preferred_name", "registration_status", "primary_group_code", "primary_group_name", "group_count", "last_profile_update_at"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.HouseholdID, row.HouseholdName, row.HouseholdStatus, row.PrimaryEmail, row.PrimaryPhoneE164, row.SwimmerID, row.SwimmerName, row.PreferredName, row.RegistrationStatus, row.PrimaryGroupCode, row.PrimaryGroupName, strconv.Itoa(row.GroupCount), row.LastProfileUpdateAt.Format(time.RFC3339)})
			}
			return rows
		},
	})
}

func (h *ReportsHandlers) runMeetFees(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunMeetFeesReport(orgID, principal)
	if err != nil {
		failReport(w, r, err, "meet fees")
		return
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "meet_fees", auditAction: audit.ActionReportMeetFeesRun,
		report: report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   csvFilename("meet-fees", report.GeneratedAt),
		csvHeader: []string{"invoice_id", "invoice_number", "invoice_status", "household_id", "meet_id", "meet_name", "swimmer_id", "swimmer_name", "line_item_id", "line_description", "quantity", "unit_amount_cents", "total_cents", "due_date", "issued_at"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.InvoiceStatus, row.HouseholdID, row.MeetID, row.MeetName, row.SwimmerID, row.SwimmerName, row.LineItemID, row.LineDescription, strconv.Itoa(row.Quantity), strconv.Itoa(row.UnitAmountCents), strconv.Itoa(row.TotalCents), row.DueDate, formatOptionalTime(row.IssuedAt)})
			}
			return rows
		},
	})
}

func (h *ReportsHandlers) runInvoiceAging(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
		failReport(w, r, err, "invoice aging")
		return
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "invoice_aging", auditAction: audit.ActionReportInvoiceAgingRun,
		auditExtra: map[string]string{"as_of": report.AsOfDate},
		report:     report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   csvFilename("invoice-aging", report.GeneratedAt),
		csvHeader: []string{"invoice_id", "invoice_number", "household_id", "status", "due_date", "days_past_due", "aging_bucket", "total_cents", "amount_paid_cents", "amount_due_cents", "is_outstanding"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.HouseholdID, row.Status, row.DueDate, strconv.Itoa(row.DaysPastDue), row.AgingBucket, strconv.Itoa(row.TotalCents), strconv.Itoa(row.AmountPaidCents), strconv.Itoa(row.AmountDueCents), fmt.Sprintf("%t", row.IsOutstanding)})
			}
			return rows
		},
	})
}

func (h *ReportsHandlers) runInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	invoiceID := strings.TrimSpace(r.URL.Query().Get("invoice_id"))
	report, err := h.service.RunInvoiceDetailReport(orgID, principal, invoiceID)
	if err != nil {
		failReport(w, r, err, "invoice detail")
		return
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "invoice_detail", auditAction: audit.ActionReportInvoiceDetailRun, auditID: invoiceID,
		report: report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   fmt.Sprintf("invoice-%s-%s.csv", report.InvoiceNumber, report.GeneratedAt.Format("20060102")),
		csvHeader: []string{"record_type", "description", "swimmer_name", "quantity", "amount_cents", "status", "date"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.RecordType, row.Description, row.SwimmerName, strconv.Itoa(row.Quantity), strconv.Itoa(row.AmountCents), row.Status, row.Date})
			}
			return rows
		},
	})
}

func (h *ReportsHandlers) runPaymentReconciliation(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
		failReport(w, r, err, "payment reconciliation")
		return
	}
	var extra map[string]string
	if stripeReportedCents != nil {
		extra = map[string]string{"stripe_reported_cents": fmt.Sprintf("%d", *stripeReportedCents)}
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "payment_reconciliation", auditAction: audit.ActionReportPaymentReconciliationRun,
		auditExtra: extra,
		report:     report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   csvFilename("payment-reconciliation", report.GeneratedAt),
		csvHeader: []string{"invoice_id", "invoice_number", "payment_id", "method", "payment_status", "payment_amount_cents", "refunded_cents", "net_collected_cents", "ledger_payment_cents", "ledger_refund_cents", "is_reconciled", "reconciliation_notes"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.InvoiceID, row.InvoiceNumber, row.PaymentID, row.Method, row.PaymentStatus, strconv.Itoa(row.PaymentAmountCents), strconv.Itoa(row.RefundedCents), strconv.Itoa(row.NetCollectedCents), strconv.Itoa(row.LedgerPaymentCents), strconv.Itoa(row.LedgerRefundCents), fmt.Sprintf("%t", row.IsReconciled), row.ReconciliationNotes})
			}
			return rows
		},
	})
}

func (h *ReportsHandlers) runRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	report, err := h.service.RunRegistrationStatusReport(orgID, principal)
	if err != nil {
		failReport(w, r, err, "registration status")
		return
	}
	h.respond(w, r, principal, orgID, reportOutput{
		kind: "registration_status", auditAction: audit.ActionReportRegistrationStatusRun,
		report: report, generatedAt: report.GeneratedAt, rowCount: report.RowCount,
		csvName:   csvFilename("registration-status", report.GeneratedAt),
		csvHeader: []string{"registration_id", "season", "status", "registration_date", "group_id", "group_code", "group_name", "household_id", "household_name", "swimmer_id", "swimmer_name", "waiver_count", "last_waiver_accepted_at"},
		csvRows: func() [][]string {
			rows := make([][]string, 0, len(report.Rows))
			for _, row := range report.Rows {
				rows = append(rows, []string{row.RegistrationID, row.Season, row.Status, row.RegistrationDate, row.GroupID, row.GroupCode, row.GroupName, row.HouseholdID, row.HouseholdName, row.SwimmerID, row.SwimmerName, strconv.Itoa(row.WaiverCount), formatOptionalTime(row.LastWaiverAcceptedAt)})
			}
			return rows
		},
	})
}

func defaultReportFormat(format string) string {
	if format == "" {
		return "json"
	}
	return format
}
