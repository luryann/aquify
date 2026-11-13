package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/httpx"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
)

// ExportsHandlers handles HTTP requests for meet PDF report generation. Reports
// are assembled server-side from the meet's approved (non-scratched) entries.
type ExportsHandlers struct {
	service        *exports.Service
	meetsService   *meets.Service
	membersService *members.Service
	orgSettings    *orgsettings.Service
	auditService   *audit.Service
	// bestTimes, when set, lets the entry bundle say which meet each seed
	// time was swum at.
	bestTimes BestTimeSourceLister
	validate  *validator.Validate
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
		validate:       httpx.NewValidator(),
	}
}

// NewExportsHandlersWithAudit returns export handlers additionally wired to
// the central audit log so report generation and download are recorded
// alongside every other privileged action in the product.
func NewExportsHandlersWithAudit(service *exports.Service, meetsService *meets.Service, membersService *members.Service, orgSettings *orgsettings.Service, auditService *audit.Service) *ExportsHandlers {
	h := NewExportsHandlersWithMeetAccess(service, meetsService, membersService, orgSettings)
	h.auditService = auditService
	return h
}

func (h *ExportsHandlers) emitAudit(principal *appmiddleware.Principal, action, resourceType, resourceID string, metadata map[string]string) {
	recordAudit(h.auditService, principal, principal.OrgID, action, resourceType, resourceID, nil, nil, metadata)
}

// RegisterRoutes mounts all export routes on the given router.
func (h *ExportsHandlers) RegisterRoutes(router chi.Router) {
	// Trigger and list exports scoped to a meet
	router.Post("/meets/{meetID}/exports", h.generateExport)
	router.Get("/meets/{meetID}/exports", h.listMeetExports)
	// Entry bundle for the host meet (admin/coach): a ZIP of the SD3 and HY3
	// entry files, both entry reports and the entry log. POST because it
	// moves approved entries to "exported".
	router.Post("/meets/{meetID}/entries/export", h.exportEntryBundle)

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

func (h *ExportsHandlers) authorizeMeetExportListing(principal *appmiddleware.Principal, orgID, meetID string) error {
	if hasAnyRole(principal, "admin", "coach") {
		return nil
	}
	if !hasAnyRole(principal, "parent", "swimmer") {
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
	return decodeAndValidateBadRequest(w, r, dst, h.validate)
}

// ─── POST /meets/{meetID}/exports ─────────────────────────────────────────────

func (h *ExportsHandlers) generateExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	// Gate on role before touching meet data so unauthorized callers can't probe
	// meet existence via the report endpoint.
	if !hasAnyRole(principal, "admin", "coach") {
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "report_assembly_failed", "Failed to assemble the report.", err)
		}
		return
	}

	job, auditEvent, err := h.service.GenerateExport(principal, orgID, meetID, input)
	if err != nil {
		switch {
		case errors.Is(err, exports.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions to generate reports.", nil)
		case errors.Is(err, exports.ErrInvalidExportType):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_report_type", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "report_failed", "Report generation failed.", err)
		}
		return
	}

	h.emitAudit(principal, "export.generated", "file_export", job.ID, map[string]string{
		"meet_id":     meetID,
		"report_type": body.ReportType,
	})

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"export":      job,
		"audit_event": auditEvent,
	})
}

// ─── GET /meets/{meetID}/exports ──────────────────────────────────────────────

func (h *ExportsHandlers) listMeetExports(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to authorize meet file access.", err)
		}
		return
	}

	list := h.service.ListExportsForViewer(principal, orgID, meetID)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(list),
		"exports": list,
	})
}

// ─── GET /exports/{exportID} ──────────────────────────────────────────────────

func (h *ExportsHandlers) getExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get export.", err)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"export": job})
}

// ─── GET /exports/{exportID}/download ────────────────────────────────────────

func (h *ExportsHandlers) downloadExport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to download export.", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// refreshMembers re-syncs the club's roster from Postgres before it is read.
// Snapshot reads never load anything themselves, so without this an export made
// on a freshly started or evicted cache (or after another instance wrote a
// swimmer) would silently see no swimmers. No-op when members is not wired.
func (h *ExportsHandlers) refreshMembers(orgID string) error {
	if h.membersService == nil {
		return nil
	}
	return h.membersService.RefreshOrg(context.Background(), orgID)
}

// swimmersByID indexes the org's swimmers; empty when members is not wired.
func (h *ExportsHandlers) swimmersByID(orgID string) map[string]members.Swimmer {
	out := map[string]members.Swimmer{}
	if h.membersService == nil {
		return out
	}
	for _, sw := range h.membersService.SnapshotSwimmers(orgID) {
		out[sw.ID] = sw
	}
	return out
}

func eventsByID(events []meets.MeetEvent) map[string]meets.MeetEvent {
	out := make(map[string]meets.MeetEvent, len(events))
	for _, ev := range events {
		out[ev.ID] = ev
	}
	return out
}

// entryIsExportable: reports and the entry bundle include approved (and already
// exported) entries only; scratched and unreviewed ones are left out.
func entryIsExportable(e meets.Entry) bool {
	return e.Status == "approved" || e.Status == "exported"
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

	if err := h.refreshMembers(orgID); err != nil {
		return exports.ReportInput{}, err
	}
	swimmerByID := h.swimmersByID(orgID)
	primaryGroup := map[string]string{}
	if h.membersService != nil {
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
	eventByID := eventsByID(events)

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
		if !entryIsExportable(e) {
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
