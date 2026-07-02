package handlers

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ComplianceHandlers handles HTTP requests for compliance data-export and
// data-deletion requests.
type ComplianceHandlers struct {
	service      *compliance.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewComplianceHandlers returns a new ComplianceHandlers.
func NewComplianceHandlers(svc *compliance.Service, auditSvc *audit.Service) *ComplianceHandlers {
	return &ComplianceHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     validator.New(),
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
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func complianceScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

// ─── Data export requests ─────────────────────────────────────────────────────

type createDataExportRequestBody struct {
	HouseholdID string `json:"household_id" validate:"required"`
}

// createDataExportRequest handles POST /api/v1/compliance/data-export-requests.
func (h *ComplianceHandlers) createDataExportRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	// owner/admin can request for any household; parent can request for their own household.
	if !hasAnyRole(principal, "owner", "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to request data export.", nil)
		return
	}

	var body createDataExportRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	// Parents may only request data for their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin") {
		if principal.HouseholdID == "" || principal.HouseholdID != body.HouseholdID {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parent may only request a data export for their own household.", nil)
			return
		}
	}

	req, err := h.service.CreateDataExportRequest(orgID, body.HouseholdID, principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create data export request.", err)
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionComplianceDataExportRequest,
			ResourceType: "data_export_request",
			ResourceID:   req.ID,
			After:        req,
			Metadata:     map[string]string{"household_id": body.HouseholdID},
		})
	}

	httpx.WriteJSON(w, http.StatusCreated, req)
}

// listDataExportRequests handles GET /api/v1/compliance/data-export-requests.
func (h *ComplianceHandlers) listDataExportRequests(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Listing data export requests requires owner or admin role.", nil)
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
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	requestID := chi.URLParam(r, "requestID")

	req, err := h.service.GetDataExportRequest(orgID, requestID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data export request not found.", nil)
		return
	}

	// owner/admin can view any; requester can view their own.
	// For non-owner/non-admin callers, return 404 in both the "not found" and
	// "unauthorized" cases to prevent resource-existence enumeration.
	if !hasAnyRole(principal, "owner", "admin") && req.RequestedBy != principal.UserID {
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
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to request data deletion.", nil)
		return
	}

	var body createDataDeletionRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	// Parents may only request deletion for their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin") {
		if principal.HouseholdID == "" || principal.HouseholdID != body.HouseholdID {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parent may only request data deletion for their own household.", nil)
			return
		}
	}

	req, err := h.service.CreateDataDeletionRequest(orgID, body.HouseholdID, principal.UserID, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create data deletion request.", err)
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionComplianceDataDeletionRequest,
			ResourceType: "data_deletion_request",
			ResourceID:   req.ID,
			After:        req,
			Metadata:     map[string]string{"household_id": body.HouseholdID, "reason": body.Reason},
		})
	}

	httpx.WriteJSON(w, http.StatusCreated, req)
}

// listDataDeletionRequests handles GET /api/v1/compliance/data-deletion-requests.
func (h *ComplianceHandlers) listDataDeletionRequests(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Listing deletion requests requires owner or admin role.", nil)
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
	principal, orgID, ok := complianceScopeFromRequest(w, r)
	if !ok {
		return
	}
	requestID := chi.URLParam(r, "requestID")

	req, err := h.service.GetDataDeletionRequest(orgID, requestID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data deletion request not found.", nil)
		return
	}

	// For non-owner/non-admin callers, return 404 in both the "not found" and
	// "unauthorized" cases to prevent resource-existence enumeration.
	if !hasAnyRole(principal, "owner", "admin") && req.RequestedBy != principal.UserID {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data deletion request not found.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, req)
}
