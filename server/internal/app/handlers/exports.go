package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/sdif"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
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
	auditService   *audit.Service
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
	// SD3 entry file for the host meet (admin/coach). POST because it moves
	// approved entries to "exported".
	router.Post("/meets/{meetID}/entries/sd3", h.exportEntriesSD3)

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

// entryIsExportable: reports and the SD3 file include approved (and already
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

// ─── SD3 entry file ───────────────────────────────────────────────────────────

// sd3ExportableMeetStatuses: entries are exported once finalized (and may be
// re-downloaded after the meet completes).
var sd3ExportableMeetStatuses = map[string]bool{"entry_finalized": true, "completed": true}

func (h *ExportsHandlers) exportEntriesSD3(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin or coach can export entry files.", nil)
		return
	}
	if h.meetsService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Meet export is not configured.", nil)
		return
	}
	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	variant := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("variant")))
	if variant == "" {
		variant = string(sdif.ExportVariantExtended)
	}
	if variant != string(sdif.ExportVariantStandard) && variant != string(sdif.ExportVariantExtended) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_variant", "variant must be 'standard' or 'extended'.", nil)
		return
	}

	meet, err := h.meetsService.GetMeet(principal, orgID, meetID)
	if err != nil {
		writeSD3MeetError(w, r, err)
		return
	}
	if !sd3ExportableMeetStatuses[meet.Status] {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entries_not_finalized", "Finalize entries before exporting the SD3 entry file.", nil)
		return
	}
	events, err := h.meetsService.ListMeetEvents(principal, orgID, meetID)
	if err != nil {
		writeSD3MeetError(w, r, err)
		return
	}
	entries, err := h.meetsService.ListEntries(principal, orgID, meetID)
	if err != nil {
		writeSD3MeetError(w, r, err)
		return
	}

	swimmerByID := h.swimmersByID(orgID)
	eventByID := eventsByID(events)

	input := sdif.ExportInput{
		MeetName:      meet.Name,
		MeetStartDate: parseSD3Date(meet.StartDate),
		MeetEndDate:   parseSD3Date(meet.EndDate),
		Course:        meetfiles.Course(strings.ToUpper(meet.Course)),
	}
	if h.orgSettings != nil {
		if settings, err := h.orgSettings.Get(orgID); err == nil && settings != nil {
			input.HostTeamCode, input.HostTeamName = strings.TrimSpace(settings.TeamCode), strings.TrimSpace(settings.TeamName)
		}
	}
	// The host's meet software attributes entries by the C1 team record, so a
	// file without a team code cannot be imported.
	if input.HostTeamCode == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "team_code_required", "Set your club's team code in Settings before exporting an entry file.", nil)
		return
	}

	exportedIDs, skippedRelays := appendSD3IndividualEntries(&input, entries, eventByID, swimmerByID)
	if len(input.IndividualEntries) == 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "no_exportable_entries", "There are no approved individual entries to export.", nil)
		return
	}

	var payload []byte
	if variant == string(sdif.ExportVariantStandard) {
		payload, err = sdif.ExportStandardSD3(input)
	} else {
		payload, err = sdif.ExportExtendedSD3(input)
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "sd3_export_failed", err.Error(), nil)
		return
	}

	marked, _, err := h.meetsService.MarkEntriesExported(principal, orgID, meetID, exportedIDs)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_state_failed", "Failed to record exported entries.", err)
		return
	}
	h.emitAudit(principal, "meet.entries_sd3_exported", "meet", meetID, map[string]string{
		"variant":        variant,
		"entry_count":    fmt.Sprintf("%d", len(exportedIDs)),
		"newly_exported": fmt.Sprintf("%d", marked),
		"skipped_relays": fmt.Sprintf("%d", skippedRelays),
	})

	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sd3Filename(meet.Name, meet.StartDate)))
	w.Header().Set("X-Entry-Count", fmt.Sprintf("%d", len(exportedIDs)))
	w.Header().Set("X-Skipped-Relays", fmt.Sprintf("%d", skippedRelays))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// appendSD3IndividualEntries adds every approved/exported individual entry to input and
// returns their IDs plus the number of relay entries skipped.
func appendSD3IndividualEntries(input *sdif.ExportInput, entries []meets.Entry, eventByID map[string]meets.MeetEvent, swimmerByID map[string]members.Swimmer) (exportedIDs []string, skippedRelays int) {
	for _, e := range entries {
		if !entryIsExportable(e) {
			continue
		}
		ev, ok := eventByID[e.MeetEventID]
		if !ok {
			continue
		}
		if e.IsRelay || ev.IsRelay {
			skippedRelays++
			continue
		}
		sw, ok := swimmerByID[e.SwimmerID]
		if !ok {
			continue
		}
		course := meetfiles.Course(strings.ToUpper(ev.Course))
		if course == "" {
			course = input.Course
		}
		seed := meetfiles.Time{Raw: "NT"}
		if e.SeedTimeMs != nil && *e.SeedTimeMs > 0 {
			seed = meetfiles.Time{Valid: true, Seconds: float64(*e.SeedTimeMs) / 1000}
		}
		input.IndividualEntries = append(input.IndividualEntries, sdif.ExportIndividualEntry{
			Swimmer: sdif.ExportSwimmer{
				LastName:  sw.LastName,
				FirstName: sw.FirstName,
				USSNumber: sw.USASwimmingID,
				DOB:       parseSD3Date(sw.DateOfBirth),
				Gender:    sd3SwimmerGender(sw.Gender),
				TeamCode:  input.HostTeamCode,
			},
			EventNumber: ev.EventNumber,
			Distance:    ev.Distance,
			Stroke:      sd3Stroke(ev.Stroke),
			EventGender: sd3EventGender(ev.Gender),
			AgeGroup:    sd3AgeGroup(ev.MinAge, ev.MaxAge),
			Course:      course,
			SeedTime:    seed,
		})
		exportedIDs = append(exportedIDs, e.ID)
	}
	return exportedIDs, skippedRelays
}

func writeSD3MeetError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, meets.ErrMeetNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
	case errors.Is(err, meets.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to export this meet.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "sd3_export_failed", "Failed to load meet data for export.", err)
	}
}

func parseSD3Date(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 10 {
		raw = raw[:10]
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &t
}

func sd3Stroke(stroke string) meetfiles.Stroke {
	switch strings.ToLower(strings.TrimSpace(stroke)) {
	case "freestyle", "free":
		return meetfiles.StrokeFreestyle
	case "backstroke", "back":
		return meetfiles.StrokeBackstroke
	case "breaststroke", "breast":
		return meetfiles.StrokeBreaststroke
	case "butterfly", "fly":
		return meetfiles.StrokeButterfly
	case "individual_medley", "im":
		return meetfiles.StrokeIM
	case "freestyle_relay", "free_relay":
		return meetfiles.StrokeFreestyleRelay
	case "medley_relay":
		return meetfiles.StrokeMedleyRelay
	}
	return meetfiles.StrokeUnknown
}

func sd3SwimmerGender(gender string) string {
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "female":
		return "F"
	case "male":
		return "M"
	}
	return ""
}

// sd3EventGender maps the meet vocabulary (M|F|X|O) to SDIF event sex; open
// events are written as mixed.
func sd3EventGender(gender string) string {
	switch strings.ToUpper(strings.TrimSpace(gender)) {
	case "M", "F":
		return strings.ToUpper(strings.TrimSpace(gender))
	}
	return "X"
}

// sd3AgeGroup encodes the SDIF 4-char event age code: lower+upper bound, with
// "UN" for no lower bound and "OV" for no upper bound (open = "UNOV").
func sd3AgeGroup(minAge, maxAge *int) string {
	lower, upper := "UN", "OV"
	if minAge != nil && *minAge > 0 {
		lower = fmt.Sprintf("%02d", *minAge)
	}
	if maxAge != nil && *maxAge > 0 && *maxAge < 100 {
		upper = fmt.Sprintf("%02d", *maxAge)
	}
	return lower + upper
}

func sd3Filename(meetName, startDate string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(meetName) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "meet"
	}
	if d := parseSD3Date(startDate); d != nil {
		name += "-" + d.Format("20060102")
	}
	return name + "-entries.sd3"
}
