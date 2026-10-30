package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// MeetsHandlers handles HTTP requests for meets, commitments, and entries.
type MeetsHandlers struct {
	service      *meets.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewMeetsHandlers returns a new MeetsHandlers.
func NewMeetsHandlers(service *meets.Service) *MeetsHandlers {
	return &MeetsHandlers{
		service:  service,
		validate: httpx.NewValidator(),
	}
}

// NewMeetsHandlersWithAudit returns a MeetsHandlers with audit logging enabled.
func NewMeetsHandlersWithAudit(service *meets.Service, auditSvc *audit.Service) *MeetsHandlers {
	return &MeetsHandlers{
		service:      service,
		auditService: auditSvc,
		validate:     httpx.NewValidator(),
	}
}

// RegisterRoutes mounts all meet routes on the given router.
func (h *MeetsHandlers) RegisterRoutes(router chi.Router) {
	// Meet CRUD and lifecycle.
	router.Post("/meets", h.createMeet)
	router.Post("/meets/parse-entry-file", h.parseEntryFile)
	router.Get("/meets", h.listMeets)
	router.Get("/meets/{meetID}", h.getMeet)
	router.Post("/meets/{meetID}/sessions", h.createMeetSession)
	router.Get("/meets/{meetID}/sessions", h.listMeetSessions)
	router.Post("/meets/{meetID}/events", h.createMeetEvent)
	router.Get("/meets/{meetID}/events", h.listMeetEvents)
	router.Post("/meets/{meetID}/transition", h.transitionMeet)

	// Commitment request and response.
	router.Post("/meets/{meetID}/commitment-request", h.publishCommitmentRequest)
	router.Get("/meets/{meetID}/commitments", h.listCommitments)
	router.Post("/meets/{meetID}/commitments/{commitmentID}/respond", h.respondToCommitment)

	// Parent event selections (pre-entry).
	router.Post("/meets/{meetID}/commitments/{commitmentID}/event-selections", h.selectCommitmentEvent)
	router.Get("/meets/{meetID}/commitments/{commitmentID}/event-selections", h.listCommitmentEventSelections)
	router.Delete("/meets/{meetID}/commitments/{commitmentID}/event-selections/{selectionID}", h.removeCommitmentEventSelection)

	// Commitment locking.
	router.Post("/meets/{meetID}/lock-commitments", h.lockCommitments)

	// Entries.
	router.Get("/meets/{meetID}/entries", h.listEntries)
	router.Post("/meets/{meetID}/entries", h.proposeEntry)
	router.Post("/meets/{meetID}/entries/{entryID}/override", h.overrideEntry)
	router.Post("/meets/{meetID}/entries/{entryID}/approve", h.approveEntry)
	router.Delete("/meets/{meetID}/entries/{entryID}", h.scratchEntry)
}

// ─── Meets ────────────────────────────────────────────────────────────────────

type createMeetRequest struct {
	Name               string                           `json:"name" validate:"required"`
	MeetType           string                           `json:"meet_type" validate:"omitempty,oneof=invitational dual_meet time_trial championship other"`
	Course             string                           `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	StartDate          string                           `json:"start_date"`
	EndDate            string                           `json:"end_date"`
	EntryDeadline      string                           `json:"entry_deadline"`
	CommitmentDeadline string                           `json:"commitment_deadline"`
	HostOrg            string                           `json:"host_org"`
	Location           string                           `json:"location"`
	Sessions           []createMeetSessionRequest       `json:"sessions" validate:"omitempty,dive"`
	Events             []createMeetEventDraftRequest    `json:"events" validate:"omitempty,dive"`
	SignupSettings     *createMeetSignupSettingsRequest `json:"signup_settings"`
}

type createMeetEventDraftRequest struct {
	SessionNumber int    `json:"session_number"`
	EventNumber   string `json:"event_number" validate:"required"`
	Stroke        string `json:"stroke" validate:"required,oneof=freestyle backstroke breaststroke butterfly individual_medley medley_relay freestyle_relay other"`
	Distance      int    `json:"distance" validate:"required,min=1"`
	Course        string `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	Gender        string `json:"gender" validate:"omitempty,oneof=M F X O"`
	MinAge        *int   `json:"min_age"`
	MaxAge        *int   `json:"max_age"`
	EntryFeeCents int    `json:"entry_fee_cents"`
	IsRelay       bool   `json:"is_relay"`
}

type createMeetSignupSettingsRequest struct {
	SelfSignupEnabled    bool `json:"self_signup_enabled"`
	MaxEntriesPerSwimmer int  `json:"max_entries_per_swimmer"`
}

func mapCreateMeetSessions(items []createMeetSessionRequest) []meets.CreateMeetSessionInput {
	if len(items) == 0 {
		return nil
	}
	out := make([]meets.CreateMeetSessionInput, 0, len(items))
	for _, item := range items {
		out = append(out, meets.CreateMeetSessionInput{
			SessionNumber: item.SessionNumber,
			Name:          item.Name,
			SessionDate:   item.SessionDate,
			WarmupTime:    item.WarmupTime,
			StartTime:     item.StartTime,
			Notes:         item.Notes,
		})
	}
	return out
}

func mapCreateMeetEvents(items []createMeetEventDraftRequest) []meets.CreateMeetEventDraftInput {
	if len(items) == 0 {
		return nil
	}
	out := make([]meets.CreateMeetEventDraftInput, 0, len(items))
	for _, item := range items {
		out = append(out, meets.CreateMeetEventDraftInput{
			SessionNumber: item.SessionNumber,
			EventNumber:   item.EventNumber,
			Stroke:        item.Stroke,
			Distance:      item.Distance,
			Course:        item.Course,
			Gender:        item.Gender,
			MinAge:        item.MinAge,
			MaxAge:        item.MaxAge,
			EntryFeeCents: item.EntryFeeCents,
			IsRelay:       item.IsRelay,
		})
	}
	return out
}

func mapCreateMeetSignupSettings(item *createMeetSignupSettingsRequest) *meets.CreateMeetSignupSettingsInput {
	if item == nil {
		return nil
	}
	return &meets.CreateMeetSignupSettingsInput{
		SelfSignupEnabled:    item.SelfSignupEnabled,
		MaxEntriesPerSwimmer: item.MaxEntriesPerSwimmer,
	}
}

func (h *MeetsHandlers) parseEntryFile(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to parse meet files.", nil)
		return
	}

	const maxBytes = 25 << 20
	// Bound the whole request so an oversized upload cannot fill memory/temp disk
	// in ParseMultipartForm before the file-size check below runs.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "Meet file exceeds the 25 MB upload cap.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_file", "Multipart field 'file' is required.", nil)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_file", "Failed to read uploaded file.", err)
		return
	}
	if int64(len(data)) > maxBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "Meet file exceeds the 25 MB upload cap.", nil)
		return
	}

	parsed, err := h.service.ParseMeetFile(principal, orgID, meets.ParseMeetFileInput{
		Filename: header.Filename,
		Data:     data,
	})
	if err != nil {
		if errors.Is(err, meets.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to parse meet files.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_meet_file", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"parsed": parsed,
	})
}

func (h *MeetsHandlers) createMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req createMeetRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meet, auditEvent, err := h.service.CreateMeet(principal, orgID, meets.CreateMeetInput{
		Name:               req.Name,
		MeetType:           req.MeetType,
		Course:             req.Course,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		EntryDeadline:      req.EntryDeadline,
		CommitmentDeadline: req.CommitmentDeadline,
		HostOrg:            req.HostOrg,
		Location:           req.Location,
		Sessions:           mapCreateMeetSessions(req.Sessions),
		Events:             mapCreateMeetEvents(req.Events),
		SignupSettings:     mapCreateMeetSignupSettings(req.SignupSettings),
	})
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to create meets.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_meet_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"meet":        meet,
		"audit_event": auditEvent,
	})
}

func (h *MeetsHandlers) listMeets(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	result, err := h.service.ListMeets(principal, orgID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list meets.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred.", err)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	result, total := paginateSlice(result, pagination)

	writePagedList(w, "meets", result, total, pagination)
}

func (h *MeetsHandlers) getMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	meet, err := h.service.GetMeet(principal, orgID, meetID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view this meet.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred.", err)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"meet":   meet,
	})
}

// writeMeetNotFound answers the 404 for any missing meets-domain record and reports whether it did.
func writeMeetNotFound(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, meets.ErrMeetNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
	case errors.Is(err, meets.ErrCommitmentNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
	case errors.Is(err, meets.ErrEventNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "event_not_found", "Meet event was not found.", nil)
	case errors.Is(err, meets.ErrSelectionNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "selection_not_found", "Event selection was not found.", nil)
	case errors.Is(err, meets.ErrSessionNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "session_not_found", "Session was not found.", nil)
	case errors.Is(err, meets.ErrEntryNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "entry_not_found", "Entry was not found.", nil)
	default:
		return false
	}
	return true
}

// meetIDParam reads the {meetID} route parameter, answering 400 when it is blank.
func meetIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return "", false
	}
	return meetID, true
}

// commitmentIDParams reads {meetID} and {commitmentID}, answering 400 when either is blank.
func commitmentIDParams(w http.ResponseWriter, r *http.Request) (meetID, commitmentID string, ok bool) {
	meetID = strings.TrimSpace(chi.URLParam(r, "meetID"))
	commitmentID = strings.TrimSpace(chi.URLParam(r, "commitmentID"))
	if !validateID(meetID) || !validateID(commitmentID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and commitment ID are required.", nil)
		return "", "", false
	}
	return meetID, commitmentID, true
}

// entryIDParams reads {meetID} and {entryID}, answering 400 when either is blank.
func entryIDParams(w http.ResponseWriter, r *http.Request) (meetID, entryID string, ok bool) {
	meetID = strings.TrimSpace(chi.URLParam(r, "meetID"))
	entryID = strings.TrimSpace(chi.URLParam(r, "entryID"))
	if !validateID(meetID) || !validateID(entryID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and entry ID are required.", nil)
		return "", "", false
	}
	return meetID, entryID, true
}

type transitionMeetRequest struct {
	Status string `json:"status" validate:"required"`
}

func (h *MeetsHandlers) transitionMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req transitionMeetRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	meet, auditEvent, err := h.service.TransitionMeetStatus(principal, orgID, meetID, req.Status)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to transition this meet.", nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_transition_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "updated",
		"meet":        meet,
		"audit_event": auditEvent,
	})
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

type createMeetSessionRequest struct {
	SessionNumber int    `json:"session_number" validate:"required,min=1"`
	Name          string `json:"name" validate:"required"`
	SessionDate   string `json:"session_date"`
	WarmupTime    string `json:"warmup_time"`
	StartTime     string `json:"start_time"`
	Notes         string `json:"notes"`
}

func (h *MeetsHandlers) createMeetSession(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req createMeetSessionRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	session, auditEvent, err := h.service.CreateMeetSession(principal, orgID, meetID, meets.CreateMeetSessionInput{
		SessionNumber: req.SessionNumber,
		Name:          req.Name,
		SessionDate:   req.SessionDate,
		WarmupTime:    req.WarmupTime,
		StartTime:     req.StartTime,
		Notes:         req.Notes,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to add sessions.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_session_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"session":     session,
		"audit_event": auditEvent,
	})
}

// listMeetChildren serves the paginated list routes under /meets/{meetID}
// (sessions, events, commitments, entries). noun names the collection in the
// forbidden message and is also the JSON key of the returned page.
func listMeetChildren[T any](
	w http.ResponseWriter,
	r *http.Request,
	noun string,
	list func(principal *appmiddleware.Principal, orgID, meetID string) ([]T, error),
) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}

	result, err := list(principal, orgID, meetID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list "+noun+".", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred.", err)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	result, total := paginateSlice(result, pagination)

	writePagedList(w, noun, result, total, pagination)
}

func (h *MeetsHandlers) listMeetSessions(w http.ResponseWriter, r *http.Request) {
	listMeetChildren(w, r, "sessions", h.service.ListMeetSessions)
}

// ─── Events ───────────────────────────────────────────────────────────────────

type createMeetEventRequest struct {
	SessionID     string `json:"session_id"`
	EventNumber   string `json:"event_number" validate:"required"`
	Stroke        string `json:"stroke" validate:"required,oneof=freestyle backstroke breaststroke butterfly individual_medley medley_relay freestyle_relay other"`
	Distance      int    `json:"distance" validate:"required,min=1"`
	Course        string `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	Gender        string `json:"gender" validate:"omitempty,oneof=M F X O"`
	MinAge        *int   `json:"min_age"`
	MaxAge        *int   `json:"max_age"`
	EntryFeeCents int    `json:"entry_fee_cents"`
	IsRelay       bool   `json:"is_relay"`
}

func (h *MeetsHandlers) createMeetEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req createMeetEventRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	meetEvent, auditEvent, err := h.service.CreateMeetEvent(principal, orgID, meetID, meets.CreateMeetEventInput{
		SessionID:     req.SessionID,
		EventNumber:   req.EventNumber,
		Stroke:        req.Stroke,
		Distance:      req.Distance,
		Course:        req.Course,
		Gender:        req.Gender,
		MinAge:        req.MinAge,
		MaxAge:        req.MaxAge,
		EntryFeeCents: req.EntryFeeCents,
		IsRelay:       req.IsRelay,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrEventNumberConflict):
			httpx.WriteError(w, r, http.StatusConflict, "event_number_conflict", err.Error(), nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to add events.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_event_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"event":       meetEvent,
		"audit_event": auditEvent,
	})
}

func (h *MeetsHandlers) listMeetEvents(w http.ResponseWriter, r *http.Request) {
	listMeetChildren(w, r, "events", h.service.ListMeetEvents)
}

// ─── Commitment request ───────────────────────────────────────────────────────

func (h *MeetsHandlers) publishCommitmentRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	meet, commitments, auditEvent, err := h.service.PublishCommitmentRequest(principal, orgID, meetID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to publish commitment requests.", nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "commitment_request_failed", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "commitment_request_published",
		"meet":        meet,
		"commitments": commitments,
		"audit_event": auditEvent,
	})
}

// ─── Commitments ─────────────────────────────────────────────────────────────

func (h *MeetsHandlers) listCommitments(w http.ResponseWriter, r *http.Request) {
	listMeetChildren(w, r, "commitments", h.service.ListCommitments)
}

type respondToCommitmentRequest struct {
	Response string `json:"response" validate:"required,oneof=committed_yes committed_no"`
	Notes    string `json:"notes"`
}

func (h *MeetsHandlers) respondToCommitment(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req respondToCommitmentRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, commitmentID, ok := commitmentIDParams(w, r)
	if !ok {
		return
	}
	commitment, auditEvent, err := h.service.RespondToCommitment(principal, orgID, meetID, commitmentID, meets.CommitmentRespondInput{
		Response: req.Response,
		Notes:    req.Notes,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "commitment_response_failed", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "updated",
		"commitment":  commitment,
		"audit_event": auditEvent,
	})
}

// ─── Event selections ─────────────────────────────────────────────────────────

type selectCommitmentEventRequest struct {
	MeetEventID string `json:"meet_event_id" validate:"required"`
	SeedTimeMs  *int   `json:"seed_time_ms"`
}

func (h *MeetsHandlers) selectCommitmentEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req selectCommitmentEventRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, commitmentID, ok := commitmentIDParams(w, r)
	if !ok {
		return
	}
	selection, err := h.service.SelectCommitmentEvent(principal, orgID, meetID, commitmentID, meets.SelectEventInput{
		MeetEventID: req.MeetEventID,
		SeedTimeMs:  req.SeedTimeMs,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		case errors.Is(err, meets.ErrCommitmentLocked):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "commitment_locked", err.Error(), nil)
		case errors.Is(err, meets.ErrNotCommittedYes):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "not_committed_yes", err.Error(), nil)
		case errors.Is(err, meets.ErrDuplicateSelection):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_selection", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "selection_failed", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":    "selected",
		"selection": selection,
	})
}

func (h *MeetsHandlers) listCommitmentEventSelections(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, commitmentID, ok := commitmentIDParams(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListCommitmentEventSelections(principal, orgID, meetID, commitmentID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred.", err)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	result, total := paginateSlice(result, pagination)

	writePagedList(w, "selections", result, total, pagination)
}

func (h *MeetsHandlers) removeCommitmentEventSelection(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	commitmentID := strings.TrimSpace(chi.URLParam(r, "commitmentID"))
	selectionID := strings.TrimSpace(chi.URLParam(r, "selectionID"))
	if !validateID(meetID) || !validateID(commitmentID) || !validateID(selectionID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID, commitment ID, and selection ID are required.", nil)
		return
	}

	err := h.service.RemoveCommitmentEventSelection(principal, orgID, meetID, commitmentID, selectionID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		case errors.Is(err, meets.ErrCommitmentLocked):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "commitment_locked", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "removal_failed", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "removed",
	})
}

// ─── Lock commitments ─────────────────────────────────────────────────────────

func (h *MeetsHandlers) lockCommitments(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	meet, auditEvent, err := h.service.LockCommitments(principal, orgID, meetID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to lock commitments.", nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "lock_failed", err.Error(), nil)
		}
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionMeetCommitmentsLock, "meet", meetID, nil, meet, nil)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "commitments_locked",
		"meet":        meet,
		"audit_event": auditEvent,
	})
}

// ─── Entries ──────────────────────────────────────────────────────────────────

func (h *MeetsHandlers) listEntries(w http.ResponseWriter, r *http.Request) {
	listMeetChildren(w, r, "entries", h.service.ListEntries)
}

type proposeEntryRequest struct {
	SwimmerID           string `json:"swimmer_id" validate:"required"`
	MeetEventID         string `json:"meet_event_id" validate:"required"`
	CommitmentID        string `json:"commitment_id"`
	SeedTimeMs          *int   `json:"seed_time_ms"`
	IsRelay             bool   `json:"is_relay"`
	OverrideEligibility bool   `json:"override_eligibility"`
	OverrideReason      string `json:"override_reason"`
}

func (h *MeetsHandlers) proposeEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req proposeEntryRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, ok := meetIDParam(w, r)
	if !ok {
		return
	}
	entry, auditEvent, err := h.service.ProposeEntry(principal, orgID, meetID, meets.ProposeEntryInput{
		SwimmerID:           req.SwimmerID,
		MeetEventID:         req.MeetEventID,
		CommitmentID:        req.CommitmentID,
		SeedTimeMs:          req.SeedTimeMs,
		IsRelay:             req.IsRelay,
		OverrideEligibility: req.OverrideEligibility,
		OverrideReason:      req.OverrideReason,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to propose entries.", nil)
		case errors.Is(err, meets.ErrDuplicateEntry):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_entry", err.Error(), nil)
		case errors.Is(err, meets.ErrIneligibleEntry):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "ineligible_entry", err.Error(), nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_state", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "entry_proposal_failed", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "proposed",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

type overrideEntryRequest struct {
	NewMeetEventID string `json:"new_meet_event_id" validate:"required"`
	SeedTimeMs     *int   `json:"seed_time_ms"`
	Reason         string `json:"reason" validate:"required"`
}

func (h *MeetsHandlers) overrideEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req overrideEntryRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID, entryID, ok := entryIDParams(w, r)
	if !ok {
		return
	}
	entry, auditEvent, err := h.service.OverrideEntry(principal, orgID, meetID, entryID, meets.OverrideEntryInput{
		NewMeetEventID: req.NewMeetEventID,
		SeedTimeMs:     req.SeedTimeMs,
		Reason:         req.Reason,
	})
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to override entries.", nil)
		case errors.Is(err, meets.ErrDuplicateEntry):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_entry", err.Error(), nil)
		case errors.Is(err, meets.ErrIneligibleEntry):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "ineligible_entry", err.Error(), nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusConflict, "invalid_meet_state", err.Error(), nil)
		case errors.Is(err, meets.ErrEntryNotEditable):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entry_not_editable", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "override_failed", err.Error(), nil)
		}
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionMeetEntryOverride, "entry", entryID, nil, entry, map[string]string{"meet_id": meetID, "reason": req.Reason})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "overridden",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

func (h *MeetsHandlers) approveEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, entryID, ok := entryIDParams(w, r)
	if !ok {
		return
	}
	entry, auditEvent, err := h.service.ApproveEntry(principal, orgID, meetID, entryID)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to approve entries.", nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusConflict, "invalid_meet_state", err.Error(), nil)
		case errors.Is(err, meets.ErrEntryNotEditable):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entry_not_editable", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "approval_failed", err.Error(), nil)
		}
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionMeetEntryApprove, "entry", entryID, nil, entry, map[string]string{"meet_id": meetID})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "approved",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

// scratchEntry soft-scratches an entry (DELETE). The record is retained but
// marked "scratched" so it is excluded from reports/exports. An optional
// ?reason= query param is recorded on the audit event.
func (h *MeetsHandlers) scratchEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	meetID, entryID, ok := entryIDParams(w, r)
	if !ok {
		return
	}
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))

	entry, auditEvent, err := h.service.ScratchEntry(principal, orgID, meetID, entryID, reason)
	if err != nil {
		if writeMeetNotFound(w, r, err) {
			return
		}
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to scratch entries.", nil)
		case errors.Is(err, meets.ErrEntryAlreadyScratched):
			httpx.WriteError(w, r, http.StatusConflict, "already_scratched", err.Error(), nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusConflict, "invalid_meet_state", err.Error(), nil)
		case errors.Is(err, meets.ErrEntryNotEditable):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entry_not_editable", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "scratch_failed", err.Error(), nil)
		}
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionMeetEntryScratch, "entry", entryID, nil, entry, map[string]string{"meet_id": meetID, "reason": reason})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "scratched",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func (h *MeetsHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateBadRequest(w, r, dst, h.validate)
}

// validateID returns true if the given path parameter value is non-empty.
func validateID(v string) bool {
	return len(v) > 0
}
