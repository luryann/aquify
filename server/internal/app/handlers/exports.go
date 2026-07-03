package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// ExportsHandlers handles HTTP requests for meet PDF report generation. Reports
// are assembled server-side from the meet's approved (non-scratched) entries.
type ExportsHandlers struct {
	service        *exports.Service
	meetsService   *meets.Service
	membersService *members.Service
	orgSettings    *orgsettings.Service
	validate       *validator.Validate
}

// NewExportsHandlers returns a new ExportsHandlers.
func NewExportsHandlers(service *exports.Service) *ExportsHandlers {
	return NewExportsHandlersWithMeetAccess(service, nil, nil, nil)
}

// NewExportsHandlersWithMeetAccess returns export handlers wired for
// server-side report assembly (meets + members + org settings) and meet-scoped
// portal/swimmer read authorization.
func NewExportsHandlersWithMeetAccess(service *exports.Service, meetsService *meets.Service, membersService *members.Service, orgSettings *orgsettings.Service) *ExportsHandlers {
	return &ExportsHandlers{
		service:        service,
		meetsService:   meetsService,
		membersService: membersService,
		orgSettings:    orgSettings,
		validate:       validator.New(),
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

// generateReportBody is all the client sends: the report type and (for entry
// reports) the grouping. All report data is assembled server-side from the meet.
type generateReportBody struct {
	ReportType string `json:"report_type" validate:"required,oneof=eligibility_report entry_report event_report"`
	Grouping   string `json:"grouping"     validate:"omitempty,oneof=session group alphabetical"`
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
	// Gate on role before touching meet data so unauthorized callers can't probe
	// meet existence via the report endpoint.
	if !exportPrincipalHasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions to generate reports.", nil)
		return
	}
	meetID := chi.URLParam(r, "meetID")
	if strings.TrimSpace(meetID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_meet_id", "meetID path parameter is required.", nil)
		return
	}

	var body generateReportBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	input, err := h.assembleReportInput(principal, orgID, meetID, body.ReportType, body.Grouping)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions to generate reports.", nil)
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "report_assembly_failed", fmt.Sprintf("Failed to assemble report: %s", err.Error()), nil)
		}
		return
	}

	job, audit, err := h.service.GenerateExport(principal, orgID, meetID, input)
	if err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions to generate reports.", nil)
		case errors.Is(err, exports.ErrInvalidExportType):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_report_type", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "report_failed", fmt.Sprintf("Report generation failed: %s", err.Error()), nil)
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

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// ─── Server-side report assembly ──────────────────────────────────────────────

// assembleReportInput pulls the meet, its events/sessions, approved
// (non-scratched) entries, swimmers, primary groups, and org export identity,
// then builds the fully-resolved ReportInput the PDF renderer consumes.
func (h *ExportsHandlers) assembleReportInput(principal *appmiddleware.Principal, orgID, meetID, reportType, grouping string) (exports.ReportInput, error) {
	if h.meetsService == nil {
		return exports.ReportInput{}, fmt.Errorf("report assembly is not configured")
	}

	meet, err := h.meetsService.GetMeet(principal, orgID, meetID)
	if err != nil {
		return exports.ReportInput{}, err
	}
	events, err := h.meetsService.ListMeetEvents(principal, orgID, meetID)
	if err != nil {
		return exports.ReportInput{}, err
	}
	sessions, err := h.meetsService.ListMeetSessions(principal, orgID, meetID)
	if err != nil {
		return exports.ReportInput{}, err
	}
	entries, err := h.meetsService.ListEntries(principal, orgID, meetID)
	if err != nil {
		return exports.ReportInput{}, err
	}

	swimmerByID := map[string]members.Swimmer{}
	primaryGroup := map[string]string{}
	if h.membersService != nil {
		for _, sw := range h.membersService.SnapshotSwimmers(orgID) {
			swimmerByID[sw.ID] = sw
		}
		for _, link := range h.membersService.SnapshotSwimmerGroupLinks(orgID) {
			if link.IsPrimary {
				primaryGroup[link.SwimmerID] = link.GroupCode
			} else if _, ok := primaryGroup[link.SwimmerID]; !ok {
				primaryGroup[link.SwimmerID] = link.GroupCode
			}
		}
	}

	sessionName := make(map[string]string, len(sessions))
	for _, s := range sessions {
		sessionName[s.ID] = s.Name
	}
	eventByID := make(map[string]meets.MeetEvent, len(events))
	for _, ev := range events {
		eventByID[ev.ID] = ev
	}

	var team exports.ReportTeam
	if h.orgSettings != nil {
		if settings, err := h.orgSettings.Get(orgID); err == nil && settings != nil {
			team = exports.ReportTeam{TeamCode: settings.TeamCode, TeamName: settings.TeamName, LSCCode: settings.LSCCode}
		}
	}

	input := exports.ReportInput{
		ReportType: reportType,
		Grouping:   grouping,
		Team:       team,
		Meet: exports.ReportMeet{
			Name:      meet.Name,
			StartDate: meet.StartDate,
			EndDate:   meet.EndDate,
			Course:    meet.Course,
			Location:  meet.Location,
			HostOrg:   meet.HostOrg,
		},
	}

	usedEvents := map[string]bool{}
	for _, e := range entries {
		// Reports include approved, non-scratched entries only.
		if e.Status != "approved" {
			continue
		}
		ev, ok := eventByID[e.MeetEventID]
		if !ok {
			continue
		}
		sw := swimmerByID[e.SwimmerID]
		eligible, note := evaluateReportEligibility(sw, ev, meet.StartDate)
		input.Entries = append(input.Entries, exports.ReportEntry{
			SwimmerFirst:    sw.FirstName,
			SwimmerLast:     sw.LastName,
			USASwimmingID:   sw.USASwimmingID,
			Gender:          sw.Gender,
			AgeAtMeet:       exports.AgeAtMeet(sw.DateOfBirth, meet.StartDate),
			GroupCode:       primaryGroup[e.SwimmerID],
			MeetEventID:     ev.ID,
			EventNumber:     ev.EventNumber,
			SessionID:       ev.SessionID,
			SessionName:     sessionName[ev.SessionID],
			Stroke:          ev.Stroke,
			Distance:        ev.Distance,
			Course:          ev.Course,
			SeedTimeMs:      e.SeedTimeMs,
			Eligible:        eligible,
			EligibilityNote: note,
		})
		usedEvents[ev.ID] = true
	}

	for _, ev := range events {
		if !usedEvents[ev.ID] {
			continue
		}
		input.Events = append(input.Events, exports.ReportEvent{
			MeetEventID: ev.ID,
			EventNumber: ev.EventNumber,
			SessionID:   ev.SessionID,
			SessionName: sessionName[ev.SessionID],
			Stroke:      ev.Stroke,
			Distance:    ev.Distance,
			Course:      ev.Course,
			Gender:      ev.Gender,
			MinAge:      ev.MinAge,
			MaxAge:      ev.MaxAge,
			IsRelay:     ev.IsRelay,
		})
	}

	return input, nil
}

// evaluateReportEligibility mirrors the entry-creation gate: swimmer gender maps
// to the event gender vocabulary (female→F, male→M; non_binary/undisclosed only
// for open/mixed X/O events), and age-as-of-meet-start must fall in the event's band.
func evaluateReportEligibility(sw members.Swimmer, ev meets.MeetEvent, meetStart string) (bool, string) {
	if ev.IsRelay {
		return true, ""
	}
	eventGender := strings.ToUpper(strings.TrimSpace(ev.Gender))
	switch strings.ToLower(strings.TrimSpace(sw.Gender)) {
	case "female":
		if eventGender != "F" && eventGender != "X" && eventGender != "O" {
			return false, "swimmer sex (F) does not match event"
		}
	case "male":
		if eventGender != "M" && eventGender != "X" && eventGender != "O" {
			return false, "swimmer sex (M) does not match event"
		}
	default: // non_binary / undisclosed
		if eventGender != "X" && eventGender != "O" {
			return false, "non-binary/undisclosed eligible for open/mixed events only"
		}
	}
	age := exports.AgeAtMeet(sw.DateOfBirth, meetStart)
	if ev.MinAge != nil && age < *ev.MinAge {
		return false, fmt.Sprintf("age %d below event minimum %d", age, *ev.MinAge)
	}
	if ev.MaxAge != nil && age > *ev.MaxAge {
		return false, fmt.Sprintf("age %d above event maximum %d", age, *ev.MaxAge)
	}
	return true, ""
}
