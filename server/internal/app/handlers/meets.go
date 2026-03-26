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
		validate: validator.New(),
	}
}

// NewMeetsHandlersWithAudit returns a MeetsHandlers with audit logging enabled.
func NewMeetsHandlersWithAudit(service *meets.Service, auditSvc *audit.Service) *MeetsHandlers {
	return &MeetsHandlers{
		service:      service,
		auditService: auditSvc,
		validate:     validator.New(),
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
}

// ─── Meets ────────────────────────────────────────────────────────────────────

type createMeetRequest struct {
	Name               string `json:"name" validate:"required"`
	MeetType           string `json:"meet_type" validate:"omitempty,oneof=invitational dual_meet time_trial championship other"`
	Course             string `json:"course" validate:"omitempty,oneof=SCY SCM LCM"`
	StartDate          string `json:"start_date"`
	EndDate            string `json:"end_date"`
	EntryDeadline      string `json:"entry_deadline"`
	CommitmentDeadline string `json:"commitment_deadline"`
	HostOrg            string `json:"host_org"`
	Location           string `json:"location"`
}

func (h *MeetsHandlers) parseEntryFile(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to parse meet files.", nil)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_file", "Multipart field 'file' is required.", nil)
		return
	}
	defer file.Close()

	const maxBytes = 25 << 20
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
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to parse meet files.", nil)
		case errors.Is(err, meets.ErrInvalidMeetFile):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_meet_file", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_meet_file", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"parsed": parsed,
	})
}

func (h *MeetsHandlers) createMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	result, err := h.service.ListMeets(principal, orgID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list meets.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(result),
		"meets":  result,
	})
}

func (h *MeetsHandlers) getMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	meet, err := h.service.GetMeet(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view this meet.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"meet":   meet,
	})
}

type transitionMeetRequest struct {
	Status string `json:"status" validate:"required"`
}

func (h *MeetsHandlers) transitionMeet(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req transitionMeetRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	meet, auditEvent, err := h.service.TransitionMeetStatus(principal, orgID, meetID, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createMeetSessionRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
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
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
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

func (h *MeetsHandlers) listMeetSessions(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}

	result, err := h.service.ListMeetSessions(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list sessions.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"count":    len(result),
		"sessions": result,
	})
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createMeetEventRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
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
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrSessionNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "session_not_found", "Session was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	result, err := h.service.ListMeetEvents(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list events.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(result),
		"events": result,
	})
}

// ─── Commitment request ───────────────────────────────────────────────────────

func (h *MeetsHandlers) publishCommitmentRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	meet, commitments, auditEvent, err := h.service.PublishCommitmentRequest(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	result, err := h.service.ListCommitments(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list commitments.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"count":       len(result),
		"commitments": result,
	})
}

type respondToCommitmentRequest struct {
	Response string `json:"response" validate:"required,oneof=committed_yes committed_no"`
	Notes    string `json:"notes"`
}

func (h *MeetsHandlers) respondToCommitment(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req respondToCommitmentRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	commitmentID := strings.TrimSpace(chi.URLParam(r, "commitmentID"))
	if !validateID(meetID) || !validateID(commitmentID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and commitment ID are required.", nil)
		return
	}
	commitment, auditEvent, err := h.service.RespondToCommitment(principal, orgID, meetID, commitmentID, meets.CommitmentRespondInput{
		Response: req.Response,
		Notes:    req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrCommitmentNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req selectCommitmentEventRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	commitmentID := strings.TrimSpace(chi.URLParam(r, "commitmentID"))
	if !validateID(meetID) || !validateID(commitmentID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and commitment ID are required.", nil)
		return
	}
	selection, err := h.service.SelectCommitmentEvent(principal, orgID, meetID, commitmentID, meets.SelectEventInput{
		MeetEventID: req.MeetEventID,
		SeedTimeMs:  req.SeedTimeMs,
	})
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrCommitmentNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
		case errors.Is(err, meets.ErrEventNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "event_not_found", "Meet event was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	commitmentID := strings.TrimSpace(chi.URLParam(r, "commitmentID"))
	if !validateID(meetID) || !validateID(commitmentID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and commitment ID are required.", nil)
		return
	}
	result, err := h.service.ListCommitmentEventSelections(principal, orgID, meetID, commitmentID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrCommitmentNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"count":      len(result),
		"selections": result,
	})
}

func (h *MeetsHandlers) removeCommitmentEventSelection(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
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
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrCommitmentNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
		case errors.Is(err, meets.ErrSelectionNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "selection_not_found", "Event selection was not found.", nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	meet, auditEvent, err := h.service.LockCommitments(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to lock commitments.", nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "lock_failed", err.Error(), nil)
		}
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMeetCommitmentsLock,
			ResourceType: "meet",
			ResourceID:   meetID,
			After:        meet,
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "commitments_locked",
		"meet":        meet,
		"audit_event": auditEvent,
	})
}

// ─── Entries ──────────────────────────────────────────────────────────────────

func (h *MeetsHandlers) listEntries(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	result, err := h.service.ListEntries(principal, orgID, meetID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list entries.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"count":   len(result),
		"entries": result,
	})
}

type proposeEntryRequest struct {
	SwimmerID    string `json:"swimmer_id" validate:"required"`
	MeetEventID  string `json:"meet_event_id" validate:"required"`
	CommitmentID string `json:"commitment_id"`
	SeedTimeMs   *int   `json:"seed_time_ms"`
	IsRelay      bool   `json:"is_relay"`
}

func (h *MeetsHandlers) proposeEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req proposeEntryRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	if !validateID(meetID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID is required.", nil)
		return
	}
	entry, auditEvent, err := h.service.ProposeEntry(principal, orgID, meetID, meets.ProposeEntryInput{
		SwimmerID:    req.SwimmerID,
		MeetEventID:  req.MeetEventID,
		CommitmentID: req.CommitmentID,
		SeedTimeMs:   req.SeedTimeMs,
		IsRelay:      req.IsRelay,
	})
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrEventNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "event_not_found", "Meet event was not found.", nil)
		case errors.Is(err, meets.ErrCommitmentNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "commitment_not_found", "Commitment was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to propose entries.", nil)
		case errors.Is(err, meets.ErrDuplicateEntry):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_entry", err.Error(), nil)
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
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req overrideEntryRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	entryID := strings.TrimSpace(chi.URLParam(r, "entryID"))
	if !validateID(meetID) || !validateID(entryID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and entry ID are required.", nil)
		return
	}
	entry, auditEvent, err := h.service.OverrideEntry(principal, orgID, meetID, entryID, meets.OverrideEntryInput{
		NewMeetEventID: req.NewMeetEventID,
		SeedTimeMs:     req.SeedTimeMs,
		Reason:         req.Reason,
	})
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrEntryNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "entry_not_found", "Entry was not found.", nil)
		case errors.Is(err, meets.ErrEventNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "event_not_found", "Meet event was not found.", nil)
		case errors.Is(err, meets.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to override entries.", nil)
		case errors.Is(err, meets.ErrDuplicateEntry):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_entry", err.Error(), nil)
		case errors.Is(err, meets.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusConflict, "invalid_meet_state", err.Error(), nil)
		case errors.Is(err, meets.ErrEntryNotEditable):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entry_not_editable", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "override_failed", err.Error(), nil)
		}
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMeetEntryOverride,
			ResourceType: "entry",
			ResourceID:   entryID,
			After:        entry,
			Metadata:     map[string]string{"meet_id": meetID, "reason": req.Reason},
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "overridden",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

func (h *MeetsHandlers) approveEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := meetScopeFromRequest(w, r)
	if !ok {
		return
	}

	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	entryID := strings.TrimSpace(chi.URLParam(r, "entryID"))
	if !validateID(meetID) || !validateID(entryID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "Meet ID and entry ID are required.", nil)
		return
	}
	entry, auditEvent, err := h.service.ApproveEntry(principal, orgID, meetID, entryID)
	if err != nil {
		switch {
		case errors.Is(err, meets.ErrMeetNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		case errors.Is(err, meets.ErrEntryNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "entry_not_found", "Entry was not found.", nil)
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

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMeetEntryApprove,
			ResourceType: "entry",
			ResourceID:   entryID,
			After:        entry,
			Metadata:     map[string]string{"meet_id": meetID},
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "approved",
		"entry":       entry,
		"audit_event": auditEvent,
	})
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func (h *MeetsHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
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

// validateID returns true if the given path parameter value is non-empty.
func validateID(v string) bool {
	return len(v) > 0
}

func meetScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return nil, "", false
	}

	orgID, ok := appmiddleware.OrgIDFromContext(r.Context())
	if !ok || strings.TrimSpace(orgID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing X-Org-Id header.", nil)
		return nil, "", false
	}
	if strings.TrimSpace(principal.OrgID) != strings.TrimSpace(orgID) {
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Principal org_id must match X-Org-Id scope.", map[string]any{
			"header_org_id":    orgID,
			"principal_org_id": principal.OrgID,
		})
		return nil, "", false
	}
	return principal, orgID, true
}
