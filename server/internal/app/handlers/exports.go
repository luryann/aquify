package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ExportsHandlers handles HTTP requests for SD3/SDIF file export generation.
type ExportsHandlers struct {
	service      *exports.Service
	meetsService *meets.Service
	validate     *validator.Validate
}

// NewExportsHandlers returns a new ExportsHandlers.
func NewExportsHandlers(service *exports.Service) *ExportsHandlers {
	return NewExportsHandlersWithMeetAccess(service, nil)
}

// NewExportsHandlersWithMeetAccess returns export handlers with optional
// meet-scoped portal/swimmer read authorization support.
func NewExportsHandlersWithMeetAccess(service *exports.Service, meetsService *meets.Service) *ExportsHandlers {
	return &ExportsHandlers{
		service:      service,
		meetsService: meetsService,
		validate:     validator.New(),
	}
}

// RegisterRoutes mounts all export routes on the given router.
func (h *ExportsHandlers) RegisterRoutes(router chi.Router) {
	// Trigger and list exports scoped to a meet
	router.Post("/meets/{meetID}/exports", h.generateExport)
	router.Get("/meets/{meetID}/exports", h.listMeetExports)

	// Export-scoped operations (not meet-scoped for cross-meet listing)
	router.Get("/exports/{exportID}", h.getExport)
	router.Get("/exports/{exportID}/download", h.downloadExport)
}

// ─── Request body types ───────────────────────────────────────────────────────

type exportSwimmerBody struct {
	LastName  string `json:"last_name"  validate:"required"`
	FirstName string `json:"first_name"`
	USSNumber string `json:"uss_number"`
	DOB       string `json:"dob"` // MMDDYYYY
	Gender    string `json:"gender" validate:"omitempty,oneof=M F"`
	TeamCode  string `json:"team_code"`
}

type exportEntryBody struct {
	Swimmer     exportSwimmerBody `json:"swimmer"  validate:"required"`
	EventNumber string            `json:"event_number" validate:"required"`
	Distance    int               `json:"distance"     validate:"required,min=1"`
	Stroke      string            `json:"stroke"       validate:"required,oneof=free back breast fly im free_relay medley_relay"`
	EventGender string            `json:"event_gender" validate:"omitempty,oneof=M F X"`
	AgeGroup    string            `json:"age_group"`
	Course      string            `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	SeedTimeMs  int               `json:"seed_time_ms"`
	Heat        int               `json:"heat"`
	Lane        int               `json:"lane"`
}

type generateExportBody struct {
	ExportType    string            `json:"export_type"     validate:"required,oneof=standard_sd3 extended_sd3 sdif_roster"`
	MeetName      string            `json:"meet_name"       validate:"required"`
	MeetStartDate string            `json:"meet_start_date"`
	MeetEndDate   string            `json:"meet_end_date"`
	Course        string            `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	HostTeamCode  string            `json:"host_team_code"`
	HostTeamName  string            `json:"host_team_name"`
	Entries       []exportEntryBody `json:"entries"`
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func exportScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

func exportPrincipalHasAnyRole(principal *appmiddleware.Principal, roles ...string) bool {
	if principal == nil {
		return false
	}
	for _, role := range principal.Roles {
		if slices.Contains(roles, role) {
			return true
		}
	}
	return false
}

func (h *ExportsHandlers) authorizeMeetExportListing(principal *appmiddleware.Principal, orgID, meetID string) error {
	if exportPrincipalHasAnyRole(principal, "owner", "admin", "coach") {
		return nil
	}
	if !exportPrincipalHasAnyRole(principal, "parent", "swimmer") {
		return exports.ErrForbidden
	}
	if h.meetsService == nil {
		return exports.ErrForbidden
	}

	commitments, err := h.meetsService.ListCommitments(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrForbidden):
			return exports.ErrForbidden
		case errors.Is(err, meets.ErrMeetNotFound):
			return exports.ErrExportNotFound
		default:
			return err
		}
	}
	if len(commitments) == 0 {
		return exports.ErrForbidden
	}
	return nil
}

func (h *ExportsHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateJSON(w, r, dst, h.validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusBadRequest,
		ValidationCode:    "validation_failed",
		ValidationMessage: "One or more request fields are invalid.",
	})
}

// ─── POST /meets/{meetID}/exports ─────────────────────────────────────────────

func (h *ExportsHandlers) generateExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := exportScopeFromRequest(w, r)
	if !ok {
		return
	}
	meetID := chi.URLParam(r, "meetID")
	if strings.TrimSpace(meetID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_meet_id", "meetID path parameter is required.", nil)
		return
	}

	var body generateExportBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	input := exports.GenerateExportInput{
		ExportType:    body.ExportType,
		MeetName:      body.MeetName,
		MeetStartDate: body.MeetStartDate,
		MeetEndDate:   body.MeetEndDate,
		Course:        body.Course,
		HostTeamCode:  body.HostTeamCode,
		HostTeamName:  body.HostTeamName,
	}
	for _, e := range body.Entries {
		input.Entries = append(input.Entries, exports.ExportEntry{
			Swimmer: exports.ExportSwimmer{
				LastName:  e.Swimmer.LastName,
				FirstName: e.Swimmer.FirstName,
				USSNumber: e.Swimmer.USSNumber,
				DOBRaw:    e.Swimmer.DOB,
				Gender:    e.Swimmer.Gender,
				TeamCode:  e.Swimmer.TeamCode,
			},
			EventNumber: e.EventNumber,
			Distance:    e.Distance,
			Stroke:      e.Stroke,
			EventGender: e.EventGender,
			AgeGroup:    e.AgeGroup,
			Course:      e.Course,
			SeedTimeMs:  e.SeedTimeMs,
			Heat:        e.Heat,
			Lane:        e.Lane,
		})
	}

	job, audit, err := h.service.GenerateExport(principal, orgID, meetID, input)
	if err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions to generate exports.", nil)
		case errors.Is(err, exports.ErrInvalidExportType):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_export_type", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "export_failed", fmt.Sprintf("Export generation failed: %s", err.Error()), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"export":      job,
		"audit_event": audit,
	})
}

// ─── GET /meets/{meetID}/exports ──────────────────────────────────────────────

func (h *ExportsHandlers) listMeetExports(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := exportScopeFromRequest(w, r)
	if !ok {
		return
	}
	meetID := chi.URLParam(r, "meetID")

	if err := h.authorizeMeetExportListing(principal, orgID, meetID); err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		case errors.Is(err, exports.ErrExportNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to authorize meet file access.", nil)
		}
		return
	}

	list := h.service.ListExportsForAuthorizedMeet(orgID, meetID)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(list),
		"exports": list,
	})
}

// ─── GET /exports/{exportID} ──────────────────────────────────────────────────

func (h *ExportsHandlers) getExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := exportScopeFromRequest(w, r)
	if !ok {
		return
	}
	exportID := chi.URLParam(r, "exportID")

	job, err := h.service.GetExport(principal, orgID, exportID)
	if err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		case errors.Is(err, exports.ErrExportNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "export_not_found", "Export job not found.", nil)
		case errors.Is(err, exports.ErrExportExpired):
			httpx.WriteError(w, r, http.StatusGone, "export_expired", "Export artifact has expired.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get export.", nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"export": job})
}

// ─── GET /exports/{exportID}/download ────────────────────────────────────────

func (h *ExportsHandlers) downloadExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := exportScopeFromRequest(w, r)
	if !ok {
		return
	}
	exportID := chi.URLParam(r, "exportID")

	job, content, err := h.service.DownloadExport(principal, orgID, exportID)
	if err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions.", nil)
		case errors.Is(err, exports.ErrExportNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "export_not_found", "Export job not found.", nil)
		case errors.Is(err, exports.ErrExportNotReady):
			httpx.WriteError(w, r, http.StatusConflict, "export_not_ready", "Export is not ready for download.", nil)
		case errors.Is(err, exports.ErrExportExpired):
			httpx.WriteError(w, r, http.StatusGone, "export_expired", "Export artifact has expired.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to download export.", nil)
		}
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
