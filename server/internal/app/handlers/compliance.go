package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ComplianceHandlers handles HTTP requests for compliance data-export and
// data-deletion requests.
type ComplianceHandlers struct {
	service      *compliance.Service
	auditService *audit.Service
	validate     *validator.Validate
	// queue runs the export/erasure work (compliance:data_export and
	// compliance:data_deletion); without it requests never leave "pending".
	queue workers.Queue
}

// SetJobQueue wires the worker queue used to process export and erasure jobs.
func (h *ComplianceHandlers) SetJobQueue(queue workers.Queue) {
	h.queue = queue
}

func (h *ComplianceHandlers) enqueue(ctx context.Context, taskType string, payload any) error {
	if h.queue == nil {
		return errors.New("job queue is not configured")
	}
	task, err := workers.NewTask(taskType, payload)
	if err != nil {
		return err
	}
	_, err = h.queue.Enqueue(ctx, task)
	return err
}

type complianceJobPayload struct {
	RequestID   string `json:"request_id"`
	OrgID       string `json:"org_id"`
	HouseholdID string `json:"household_id"`
}

// NewComplianceHandlers returns a new ComplianceHandlers.
func NewComplianceHandlers(svc *compliance.Service, auditSvc *audit.Service) *ComplianceHandlers {
	return &ComplianceHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     httpx.NewValidator(),
	}
}

// RegisterRoutes mounts all compliance routes on the given protected router.
func (h *ComplianceHandlers) RegisterRoutes(router chi.Router) {
	// Data export requests.
	router.Post("/compliance/data-export-requests", h.createDataExportRequest)
	router.Get("/compliance/data-export-requests", h.listDataExportRequests)
	router.Get("/compliance/data-export-requests/{requestID}", h.getDataExportRequest)

	// Data deletion requests.
	router.Post("/compliance/data-deletion-requests", h.createDataDeletionRequest)
	router.Get("/compliance/data-deletion-requests", h.listDataDeletionRequests)
	router.Get("/compliance/data-deletion-requests/{requestID}", h.getDataDeletionRequest)
	router.Post("/compliance/data-deletion-requests/{requestID}/approve", h.approveDataDeletionRequest)
	router.Post("/compliance/data-deletion-requests/{requestID}/reject", h.rejectDataDeletionRequest)
}

// ─── Data export requests ─────────────────────────────────────────────────────

type createDataExportRequestBody struct {
	HouseholdID string `json:"household_id" validate:"required"`
}

// createDataExportRequest handles POST /api/v1/compliance/data-export-requests.
func (h *ComplianceHandlers) createDataExportRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	// admin can request for any household; parent can request for their own household.
	if !hasAnyRole(principal, "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to request data export.", nil)
		return
	}

	var body createDataExportRequestBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Request validation failed.") {
		return
	}

	// Parents may only request data for their own household.
	if parentOutsideHousehold(principal, body.HouseholdID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parent may only request a data export for their own household.", nil)
		return
	}

	req, err := h.service.CreateDataExportRequest(orgID, body.HouseholdID, principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create data export request.", err)
		return
	}
	// The request is saved, so audit it before queueing: it stays "pending" until
	// the worker builds the archive, and if the enqueue fails an admin can retry.
	recordAudit(h.auditService, principal, orgID, audit.ActionComplianceDataExportRequest, "data_export_request", req.ID, nil, req, map[string]string{"household_id": body.HouseholdID})
	if err := h.enqueue(r.Context(), workers.TypeProcessDataExport, complianceJobPayload{RequestID: req.ID, OrgID: orgID, HouseholdID: req.HouseholdID}); err != nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "export_queue_unavailable", "The export request was saved but could not be queued for processing. Try again shortly.", err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, req)
}

// listDataExportRequests handles GET /api/v1/compliance/data-export-requests.
func (h *ComplianceHandlers) listDataExportRequests(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Listing data export requests requires admin role.", nil)
		return
	}

	householdID := strings.TrimSpace(r.URL.Query().Get("household_id"))
	reqs, err := h.service.ListDataExportRequests(orgID, householdID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list data export requests.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"requests": reqs})
}

// getDataExportRequest handles GET /api/v1/compliance/data-export-requests/{requestID}.
func (h *ComplianceHandlers) getDataExportRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	requestID := chi.URLParam(r, "requestID")

	req, err := h.service.GetDataExportRequest(orgID, requestID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data export request not found.", nil)
		return
	}

	// admin can view any; requester can view their own.
	// For non-admin callers, return 404 in both the "not found" and
	// "unauthorized" cases to prevent resource-existence enumeration.
	if !hasAnyRole(principal, "admin") && req.RequestedBy != principal.UserID {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data export request not found.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, req)
}

// ─── Data deletion requests ───────────────────────────────────────────────────

type createDataDeletionRequestBody struct {
	HouseholdID string `json:"household_id" validate:"required"`
	Reason      string `json:"reason"`
}

// createDataDeletionRequest handles POST /api/v1/compliance/data-deletion-requests.
func (h *ComplianceHandlers) createDataDeletionRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to request data deletion.", nil)
		return
	}

	var body createDataDeletionRequestBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Request validation failed.") {
		return
	}

	// Parents may only request deletion for their own household.
	if parentOutsideHousehold(principal, body.HouseholdID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parent may only request data deletion for their own household.", nil)
		return
	}

	req, err := h.service.CreateDataDeletionRequest(orgID, body.HouseholdID, principal.UserID, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create data deletion request.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionComplianceDataDeletionRequest, "data_deletion_request", req.ID, nil, req, map[string]string{"household_id": body.HouseholdID, "reason": body.Reason})

	httpx.WriteJSON(w, http.StatusCreated, req)
}

// listDataDeletionRequests handles GET /api/v1/compliance/data-deletion-requests.
func (h *ComplianceHandlers) listDataDeletionRequests(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Listing deletion requests requires admin role.", nil)
		return
	}

	householdID := strings.TrimSpace(r.URL.Query().Get("household_id"))
	reqs, err := h.service.ListDataDeletionRequests(orgID, householdID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list data deletion requests.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"requests": reqs})
}

// getDataDeletionRequest handles GET /api/v1/compliance/data-deletion-requests/{requestID}.
func (h *ComplianceHandlers) getDataDeletionRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	requestID := chi.URLParam(r, "requestID")

	req, err := h.service.GetDataDeletionRequest(orgID, requestID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data deletion request not found.", nil)
		return
	}

	// For non-admin callers, return 404 in both the "not found" and
	// "unauthorized" cases to prevent resource-existence enumeration.
	if !hasAnyRole(principal, "admin") && req.RequestedBy != principal.UserID {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data deletion request not found.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, req)
}

// ─── Data deletion review ─────────────────────────────────────────────────────

type reviewDataDeletionBody struct {
	ReviewNotes string `json:"review_notes"`
}

// approveDataDeletionRequest handles POST .../data-deletion-requests/{id}/approve.
// Approval irreversibly erases household data, so it is admin-only and
// requires an MFA-authenticated session.
func (h *ComplianceHandlers) approveDataDeletionRequest(w http.ResponseWriter, r *http.Request) {
	h.reviewDataDeletionRequest(w, r, true)
}

// rejectDataDeletionRequest handles POST .../data-deletion-requests/{id}/reject.
func (h *ComplianceHandlers) rejectDataDeletionRequest(w http.ResponseWriter, r *http.Request) {
	h.reviewDataDeletionRequest(w, r, false)
}

func (h *ComplianceHandlers) reviewDataDeletionRequest(w http.ResponseWriter, r *http.Request, approve bool) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins can review deletion requests.", nil)
		return
	}
	if approve && !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA authentication is required to approve data deletion.", nil)
		return
	}
	var body reviewDataDeletionBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	requestID := chi.URLParam(r, "requestID")

	var (
		req *compliance.DataDeletionRequest
		err error
	)
	if approve {
		req, err = h.service.ApproveDataDeletion(orgID, requestID, body.ReviewNotes)
	} else {
		req, err = h.service.RejectDataDeletion(orgID, requestID, body.ReviewNotes)
	}
	switch {
	case errors.Is(err, compliance.ErrDeletionNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data deletion request not found.", nil)
		return
	case errors.Is(err, compliance.ErrInvalidTransition):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "This request has already been reviewed.", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_review", err.Error(), nil)
		return
	}

	// The review is committed, so audit it before queueing the erasure.
	action := "compliance.data_deletion.reject"
	if approve {
		action = "compliance.data_deletion.approve"
	}
	recordAudit(h.auditService, principal, orgID, action, "data_deletion_request", req.ID, nil, req, map[string]string{"household_id": req.HouseholdID})

	if approve {
		if err := h.enqueue(r.Context(), workers.TypeProcessDataDeletion, complianceJobPayload{RequestID: req.ID, OrgID: orgID, HouseholdID: req.HouseholdID}); err != nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "deletion_queue_unavailable", "The request was approved but could not be queued for erasure. Approve again to retry.", err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, req)
}
