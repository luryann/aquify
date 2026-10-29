package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// DocumentHandlers handles document types and document submission endpoints.
type DocumentHandlers struct {
	service      *documents.Service
	auditService *audit.Service
	store        storage.ObjectStorage
	validate     *validator.Validate
}

// NewDocumentHandlers returns a new DocumentHandlers.
func NewDocumentHandlers(svc *documents.Service, auditSvc *audit.Service, store storage.ObjectStorage) *DocumentHandlers {
	return &DocumentHandlers{
		service:      svc,
		auditService: auditSvc,
		store:        store,
		validate:     httpx.NewValidator(),
	}
}

// RegisterRoutes mounts all document center routes on the protected router.
func (h *DocumentHandlers) RegisterRoutes(router chi.Router) {
	// Document type management (admin)
	router.Get("/document-types", h.listDocumentTypes)
	router.Post("/document-types", h.createDocumentType)
	router.Put("/document-types/{typeID}", h.updateDocumentType)
	router.Delete("/document-types/{typeID}", h.deleteDocumentType)

	// Document submissions (parent sees own household; admin sees all)
	router.Get("/documents", h.listSubmissions)
	router.Post("/documents", h.createSubmission)
	router.Get("/documents/{id}", h.getSubmission)
	router.Post("/documents/{id}/upload-url", h.requestUploadURL)
	router.Post("/documents/{id}/attach", h.attachFile)
	router.Post("/documents/{id}/verify", h.verifySubmission)
	router.Post("/documents/{id}/reject", h.rejectSubmission)
}

// ─── Document Types ───────────────────────────────────────────────────────────

// listDocumentTypes handles GET /api/v1/document-types
func (h *DocumentHandlers) listDocumentTypes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	activeOnly := r.URL.Query().Get("active_only") != "false"
	types, err := h.service.ListDocumentTypes(orgID, activeOnly)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list document types.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"document_types": types})
}

type createDocumentTypeBody struct {
	Name         string `json:"name"        validate:"required"`
	Category     string `json:"category"    validate:"required"`
	Description  string `json:"description"`
	IsRequired   bool   `json:"is_required"`
	AppliesTo    string `json:"applies_to"`
	ExpiryMonths *int   `json:"expiry_months"`
	IsActive     bool   `json:"is_active"`
}

// createDocumentType handles POST /api/v1/document-types
func (h *DocumentHandlers) createDocumentType(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may create document types.", nil)
		return
	}

	var body createDocumentTypeBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}

	appliesTo := body.AppliesTo
	if appliesTo == "" {
		appliesTo = "swimmer"
	}

	dt, err := h.service.CreateDocumentType(
		orgID, body.Name, body.Category, body.Description, appliesTo,
		body.IsRequired, body.IsActive, body.ExpiryMonths, principal.UserID,
	)
	if err != nil {
		if errors.Is(err, documents.ErrDuplicateName) {
			httpx.WriteError(w, r, http.StatusConflict, "conflict", "A document type with this name already exists.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create document type.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document_type.create", "document_type", dt.ID, nil, dt, nil)

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"document_type": dt})
}

type updateDocumentTypeBody struct {
	Name         *string `json:"name"`
	Category     *string `json:"category"`
	Description  *string `json:"description"`
	IsRequired   *bool   `json:"is_required"`
	AppliesTo    *string `json:"applies_to"`
	ExpiryMonths *int    `json:"expiry_months"`
	IsActive     *bool   `json:"is_active"`
}

// updateDocumentType handles PUT /api/v1/document-types/{typeID}
func (h *DocumentHandlers) updateDocumentType(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may update document types.", nil)
		return
	}
	typeID := chi.URLParam(r, "typeID")

	var body updateDocumentTypeBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}

	updates := map[string]interface{}{}
	if body.Name != nil {
		updates["name"] = *body.Name
	}
	if body.Category != nil {
		updates["category"] = *body.Category
	}
	if body.Description != nil {
		updates["description"] = *body.Description
	}
	if body.IsRequired != nil {
		updates["is_required"] = *body.IsRequired
	}
	if body.AppliesTo != nil {
		updates["applies_to"] = *body.AppliesTo
	}
	if body.ExpiryMonths != nil {
		updates["expiry_months"] = body.ExpiryMonths
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}

	dt, err := h.service.UpdateDocumentType(orgID, typeID, updates, principal.UserID)
	if err != nil {
		if errors.Is(err, documents.ErrTypeNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document type not found.", nil)
			return
		}
		if errors.Is(err, documents.ErrDuplicateName) {
			httpx.WriteError(w, r, http.StatusConflict, "conflict", "A document type with this name already exists.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update document type.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document_type.updated", "document_type", dt.ID, nil, dt, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"document_type": dt})
}

// deleteDocumentType handles DELETE /api/v1/document-types/{typeID}
func (h *DocumentHandlers) deleteDocumentType(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may delete document types.", nil)
		return
	}
	typeID := chi.URLParam(r, "typeID")

	if err := h.service.DeleteDocumentType(orgID, typeID, principal.UserID); err != nil {
		if errors.Is(err, documents.ErrTypeNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document type not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to delete document type.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document_type.deleted", "document_type", typeID, nil, nil, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// ─── Document Submissions ─────────────────────────────────────────────────────

// listSubmissions handles GET /api/v1/documents
func (h *DocumentHandlers) listSubmissions(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	householdID := strings.TrimSpace(r.URL.Query().Get("household_id"))

	// Parents may only query their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "admin", "coach") {
		if principal.HouseholdID == "" {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parent session has no household.", nil)
			return
		}
		if householdID != "" && householdID != principal.HouseholdID {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parents may only view their own household documents.", nil)
			return
		}
		householdID = principal.HouseholdID
	}

	subs, err := h.service.ListSubmissions(orgID, householdID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list document submissions.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"documents": subs})
}

type createSubmissionBody struct {
	DocumentTypeID string `json:"document_type_id" validate:"required"`
	HouseholdID    string `json:"household_id"     validate:"required"`
	SwimmerID      string `json:"swimmer_id"`
}

// createSubmission handles POST /api/v1/documents
func (h *DocumentHandlers) createSubmission(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	var body createSubmissionBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}

	// Parents may only create submissions for their own household.
	if parentOutsideHousehold(principal, body.HouseholdID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Parents may only submit documents for their own household.", nil)
		return
	}

	sub, err := h.service.CreateSubmission(orgID, body.DocumentTypeID, body.HouseholdID, body.SwimmerID, principal.UserID)
	if err != nil {
		if errors.Is(err, documents.ErrTypeNotFound) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Document type not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create document submission.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document.create", "document_submission", sub.ID, nil, sub, nil)

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"document": sub})
}

// getSubmission handles GET /api/v1/documents/{id}
func (h *DocumentHandlers) getSubmission(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	submissionID := chi.URLParam(r, "id")

	sub, err := h.service.GetSubmission(orgID, submissionID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document submission not found.", nil)
		return
	}

	// Parent can only see their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "admin", "coach") {
		if sub.HouseholdID != principal.HouseholdID {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document submission not found.", nil)
			return
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"document": sub})
}

type attachFileBody struct {
	FileObjectKey string `json:"file_object_key" validate:"required"`
	FileName      string `json:"file_name"       validate:"required"`
	FileMIME      string `json:"file_mime"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

type documentUploadURLBody struct {
	FileName      string `json:"file_name"       validate:"required"`
	FileMIME      string `json:"file_mime"       validate:"required"`
	FileSizeBytes int64  `json:"file_size_bytes" validate:"required,gt=0"`
}

func sanitizeDocumentFilename(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("filename is required")
	}
	base := filepath.Base(trimmed)
	if base == "." || base == "/" || base == `\` || base != trimmed {
		return "", errors.New("filename must be a safe basename")
	}
	if strings.Contains(base, "..") {
		return "", errors.New("filename must not contain traversal segments")
	}
	return base, nil
}

func (h *DocumentHandlers) validateUploadAccess(principal *appmiddleware.Principal, orgID, submissionID string) (*documents.DocumentSubmission, bool) {
	existing, err := h.service.GetSubmission(orgID, submissionID)
	if err != nil {
		return nil, false
	}
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "admin") {
		if existing.HouseholdID != principal.HouseholdID {
			return nil, false
		}
	}
	return existing, true
}

// Document uploads are household-submitted registration paperwork (waivers,
// medical forms, ID scans). These caps/allowlist bound storage abuse and
// downstream content-handling risk from client-declared metadata that does
// not match the actual uploaded object.
const maxDocumentUploadBytes int64 = 20 << 20 // 20 MiB

var allowedDocumentMIMETypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
	"image/heic":      true,
}

func documentObjectKey(orgID, submissionID, fileName string) string {
	stamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	return fmt.Sprintf("documents/%s/%s/%s-%s", orgID, submissionID, stamp, fileName)
}

func validateDocumentObjectKey(orgID, submissionID, objectKey string) error {
	prefix := fmt.Sprintf("documents/%s/%s/", strings.TrimSpace(orgID), strings.TrimSpace(submissionID))
	if !strings.HasPrefix(strings.TrimSpace(objectKey), prefix) {
		return fmt.Errorf("object key %q does not belong to this submission", objectKey)
	}
	return nil
}

// requestUploadURL handles POST /api/v1/documents/{id}/upload-url
func (h *DocumentHandlers) requestUploadURL(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	submissionID := chi.URLParam(r, "id")

	if _, ok := h.validateUploadAccess(principal, orgID, submissionID); !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document submission not found.", nil)
		return
	}

	if h.store == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Object storage is not configured.", nil)
		return
	}

	var body documentUploadURLBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}
	fileName, err := sanitizeDocumentFilename(body.FileName)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_filename", err.Error(), nil)
		return
	}
	if !allowedDocumentMIMETypes[strings.ToLower(strings.TrimSpace(body.FileMIME))] {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_mime_type", "Uploaded file type is not permitted.", nil)
		return
	}
	if body.FileSizeBytes > maxDocumentUploadBytes {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "file_too_large", "Declared file size exceeds the maximum allowed size.", nil)
		return
	}

	objectKey := documentObjectKey(orgID, submissionID, fileName)
	expiresIn := 15 * time.Minute
	uploadURL, err := h.store.PresignedPutURL(objectKey, expiresIn)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create upload URL.", err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"upload_url":         uploadURL,
		"object_key":         objectKey,
		"upload_expires_at":  time.Now().UTC().Add(expiresIn),
		"submission_id":      submissionID,
		"expected_file_name": fileName,
	})
}

// attachFile handles POST /api/v1/documents/{id}/attach
func (h *DocumentHandlers) attachFile(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	submissionID := chi.URLParam(r, "id")

	// Verify access before update.
	if _, ok := h.validateUploadAccess(principal, orgID, submissionID); !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document submission not found.", nil)
		return
	}

	var body attachFileBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}
	fileName, err := sanitizeDocumentFilename(body.FileName)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_filename", err.Error(), nil)
		return
	}
	if err := validateDocumentObjectKey(orgID, submissionID, body.FileObjectKey); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_object_key", "Attached file object key is not valid for this submission.", nil)
		return
	}
	declaredMIME := strings.ToLower(strings.TrimSpace(body.FileMIME))
	if !allowedDocumentMIMETypes[declaredMIME] {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_mime_type", "Uploaded file type is not permitted.", nil)
		return
	}
	if h.store == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Object storage is not configured.", nil)
		return
	}
	exists, err := h.store.Exists(r.Context(), body.FileObjectKey)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to validate uploaded file.", err)
		return
	}
	if !exists {
		httpx.WriteError(w, r, http.StatusConflict, "missing_uploaded_file", "Uploaded file was not found in object storage.", nil)
		return
	}
	// The presigned PUT URL is not size/type-constrained, so caller-declared
	// metadata cannot be trusted. Enforce the actual stored object size
	// before accepting the submission.
	actualSize, err := h.store.ObjectSize(r.Context(), body.FileObjectKey)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to validate uploaded file.", err)
		return
	}
	if actualSize > maxDocumentUploadBytes {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "file_too_large", "Uploaded file exceeds the maximum allowed size.", nil)
		return
	}

	sub, err := h.service.AttachFile(orgID, submissionID, body.FileObjectKey, fileName, declaredMIME, actualSize, principal.UserID)
	if err != nil {
		if errors.Is(err, documents.ErrInvalidTransition) {
			httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "Cannot attach file to a submission in its current state.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to attach file.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document.submit", "document_submission", sub.ID, nil, sub, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"document": sub})
}

type reviewBody struct {
	ReviewNotes string `json:"review_notes"`
}

// verifySubmission handles POST /api/v1/documents/{id}/verify
func (h *DocumentHandlers) verifySubmission(w http.ResponseWriter, r *http.Request) {
	h.reviewSubmission(w, r, "verify", h.service.VerifySubmission)
}

// rejectSubmission handles POST /api/v1/documents/{id}/reject
func (h *DocumentHandlers) rejectSubmission(w http.ResponseWriter, r *http.Request) {
	h.reviewSubmission(w, r, "reject", h.service.RejectSubmission)
}

// reviewSubmission is the admin-only verify/reject flow; verb names the action in messages and the audit trail.
func (h *DocumentHandlers) reviewSubmission(w http.ResponseWriter, r *http.Request, verb string, review func(orgID, submissionID, reviewerID, notes string) (*documents.DocumentSubmission, error)) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may "+verb+" documents.", nil)
		return
	}
	submissionID := chi.URLParam(r, "id")

	var body reviewBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}

	sub, err := review(orgID, submissionID, principal.UserID, body.ReviewNotes)
	if err != nil {
		if errors.Is(err, documents.ErrSubmissionNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Document submission not found.", nil)
			return
		}
		if errors.Is(err, documents.ErrInvalidTransition) {
			httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "Cannot "+verb+" a submission in its current state.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to "+verb+" submission.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "document."+verb, "document_submission", sub.ID, nil, sub, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"document": sub})
}
