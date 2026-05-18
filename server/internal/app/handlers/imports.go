package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ImportsHandlers handles HTTP requests for file import job management.
type ImportsHandlers struct {
	service      *imports.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewImportsHandlers returns a new ImportsHandlers.
func NewImportsHandlers(service *imports.Service) *ImportsHandlers {
	return &ImportsHandlers{
		service:  service,
		validate: validator.New(),
	}
}

// NewImportsHandlersWithAudit returns an ImportsHandlers with audit logging enabled.
func NewImportsHandlersWithAudit(service *imports.Service, auditSvc *audit.Service) *ImportsHandlers {
	return &ImportsHandlers{
		service:      service,
		auditService: auditSvc,
		validate:     validator.New(),
	}
}

// RegisterRoutes mounts all import routes on the given router.
func (h *ImportsHandlers) RegisterRoutes(router chi.Router) {
	router.Post("/imports/upload-url", h.requestUploadURL)
	router.Get("/imports", h.listImports)
	router.Get("/imports/{importID}", h.getImport)
	router.Post("/imports/{importID}/finalize", h.finalizeImport)
	router.Get("/imports/{importID}/conflicts", h.listConflicts)
	router.Post("/imports/{importID}/conflicts/{conflictID}/resolve", h.resolveConflict)
	router.Get("/imports/{importID}/report", h.getImportReport)
}

// ─── Request body types ───────────────────────────────────────────────────────

type requestUploadURLBody struct {
	Filename  string `json:"filename" validate:"required"`
	FileType  string `json:"file_type" validate:"omitempty,oneof=cl2 sd3 zip hy3 hyv ev3"`
	MeetID    string `json:"meet_id"`
	SizeBytes int64  `json:"size_bytes" validate:"required,gt=0"`
	MIMEType  string `json:"mime_type" validate:"required"`
}

type resolveConflictBody struct {
	ResolvedSwimmerID string `json:"resolved_swimmer_id" validate:"required"`
	ResolutionNote    string `json:"resolution_note"`
}

// ─── Scope / decode helpers ───────────────────────────────────────────────────

func importScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

func (h *ImportsHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := decodeJSONBody(r, dst); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return false
	}
	if err := h.validate.Struct(dst); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_failed", "One or more request fields are invalid.", httpx.FormatValidationErrors(err))
		return false
	}
	return true
}

func importErrorToHTTP(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, imports.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to perform this action.", nil)
	case errors.Is(err, imports.ErrImportNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "import_not_found", "Import job not found.", nil)
	case errors.Is(err, imports.ErrConflictNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "conflict_not_found", "Match conflict not found.", nil)
	case errors.Is(err, imports.ErrInvalidFileType):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_file_type", "Unsupported file type. Use cl2, sd3, zip, hy3, hyv, or ev3.", nil)
	case errors.Is(err, imports.ErrInvalidFilename):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_filename", "Filename must be a single safe basename with a supported extension.", nil)
	case errors.Is(err, imports.ErrFileTooLarge):
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "Upload exceeds the configured file size cap.", nil)
	case errors.Is(err, imports.ErrInvalidMIMEType):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_mime_type", "Upload MIME type is not allowed for meet imports.", nil)
	case errors.Is(err, imports.ErrMalwareDetected):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "malware_detected", "Upload failed malware scanning and was rejected.", nil)
	case errors.Is(err, imports.ErrMalwareScanRequired):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "malware_scan_required", "A compliant malware scanner must be configured before uploads can be finalized in this environment.", nil)
	case errors.Is(err, imports.ErrInvalidTransition):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", err.Error(), nil)
	case errors.Is(err, imports.ErrConflictNotOpen):
		httpx.WriteError(w, r, http.StatusConflict, "conflict_not_open", "Conflict is not open and cannot be resolved.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.", nil)
	}
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// POST /imports/upload-url
func (h *ImportsHandlers) requestUploadURL(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req requestUploadURLBody
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	imp, audit, err := h.service.RequestUploadURL(principal, orgID, imports.RequestUploadURLInput{
		Filename:  req.Filename,
		FileType:  req.FileType,
		MeetID:    req.MeetID,
		SizeBytes: req.SizeBytes,
		MIMEType:  req.MIMEType,
	})
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"import":      imp,
		"audit_event": audit,
	})
}

// POST /imports/{importID}/finalize
func (h *ImportsHandlers) finalizeImport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}
	importID := strings.TrimSpace(chi.URLParam(r, "importID"))
	if importID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Import ID is required.", nil)
		return
	}

	imp, audit, err := h.service.FinalizeImport(principal, orgID, importID, imports.FinalizeImportInput{})
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"import":      imp,
		"audit_event": audit,
	})
}

// GET /imports
func (h *ImportsHandlers) listImports(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}

	list, err := h.service.ListImports(principal, orgID)
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}
	if list == nil {
		list = []imports.FileImport{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(list),
		"imports": list,
	})
}

// GET /imports/{importID}
func (h *ImportsHandlers) getImport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}
	importID := strings.TrimSpace(chi.URLParam(r, "importID"))
	if importID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Import ID is required.", nil)
		return
	}

	imp, err := h.service.GetImport(principal, orgID, importID)
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"import": imp,
	})
}

// GET /imports/{importID}/conflicts
func (h *ImportsHandlers) listConflicts(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}
	importID := strings.TrimSpace(chi.URLParam(r, "importID"))
	if importID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Import ID is required.", nil)
		return
	}

	list, err := h.service.ListConflicts(principal, orgID, importID)
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}
	if list == nil {
		list = []imports.MatchConflict{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":     len(list),
		"conflicts": list,
	})
}

// POST /imports/{importID}/conflicts/{conflictID}/resolve
func (h *ImportsHandlers) resolveConflict(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}
	importID := strings.TrimSpace(chi.URLParam(r, "importID"))
	conflictID := strings.TrimSpace(chi.URLParam(r, "conflictID"))
	if importID == "" || conflictID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Import ID and conflict ID are required.", nil)
		return
	}

	var req resolveConflictBody
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	conflict, internalAudit, err := h.service.ResolveConflict(principal, orgID, importID, conflictID, imports.ResolveConflictInput{
		ResolvedSwimmerID: req.ResolvedSwimmerID,
		ResolutionNote:    req.ResolutionNote,
	})
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}

	if h.auditService != nil {
		meta := map[string]string{"import_id": importID}
		if req.ResolutionNote != "" {
			meta["resolution_note"] = req.ResolutionNote
		}
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionImportConflictResolve,
			ResourceType: "match_conflict",
			ResourceID:   conflictID,
			After:        conflict,
			Metadata:     meta,
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"conflict":    conflict,
		"audit_event": internalAudit,
	})
}

// GET /imports/{importID}/report
func (h *ImportsHandlers) getImportReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := importScopeFromRequest(w, r)
	if !ok {
		return
	}
	importID := strings.TrimSpace(chi.URLParam(r, "importID"))
	if importID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Import ID is required.", nil)
		return
	}

	imp, err := h.service.GetImportReport(principal, orgID, importID)
	if err != nil {
		importErrorToHTTP(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"import": imp,
		"report": imp.ImportReport,
	})
}
