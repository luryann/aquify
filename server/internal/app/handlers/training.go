package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// TrainingHandlers handles practice/schedule CRUD endpoints.
type TrainingHandlers struct {
	service        *training.Service
	membersService *members.Service
	groupsService  *groups.Service
	seasonsService *seasons.Service
	auditService   *audit.Service
	validate       *validator.Validate
}

// NewTrainingHandlers returns a new TrainingHandlers.
func NewTrainingHandlers(svc *training.Service, auditSvc *audit.Service) *TrainingHandlers {
	return &TrainingHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     httpx.NewValidator(),
	}
}

// NewTrainingHandlersWithMembers returns a new TrainingHandlers with the
// members and groups services wired in for per-swimmer attendance roster
// derivation (a practice's group_id must be resolved to the group's code
// before it can be matched against swimmer_groups links).
func NewTrainingHandlersWithMembers(svc *training.Service, membersService *members.Service, groupsService *groups.Service, auditSvc *audit.Service) *TrainingHandlers {
	return &TrainingHandlers{
		service:        svc,
		membersService: membersService,
		groupsService:  groupsService,
		auditService:   auditSvc,
		validate:       httpx.NewValidator(),
	}
}

// RegisterRoutes mounts all practice routes on the protected router.
func (h *TrainingHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/practices", h.listPractices)
	router.Post("/practices", h.createPractice)
	router.Get("/practices/{id}", h.getPractice)
	router.Put("/practices/{id}", h.updatePractice)
	router.Delete("/practices/{id}", h.deletePractice)
	router.Get("/practices/{id}/attendance", h.getAttendance)
	router.Put("/practices/{id}/attendance/{swimmerID}", h.setAttendance)
	h.registerWorkoutRoutes(router)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// listPractices handles GET /api/v1/practices
func (h *TrainingHandlers) listPractices(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	practices, err := h.service.List(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list practices.", err)
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		// Parents/swimmers only see team-wide practices and their household
		// swimmers' groups.
		scope, err := h.practiceScopeFor(principal, orgID)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve practice scope.", err)
			return
		}
		visible := make([]*training.Practice, 0, len(practices))
		for _, p := range practices {
			if scope.allows(p) {
				p.Notes = ""
				visible = append(visible, p)
			}
		}
		practices = visible
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"practices": practices})
}

type practiceStrokesBody struct {
	Free   int `json:"free"`
	Back   int `json:"back"`
	Breast int `json:"breast"`
	Fly    int `json:"fly"`
	IM     int `json:"im"`
}

func (b practiceStrokesBody) toStrokes() training.PracticeStrokes {
	return training.PracticeStrokes{Free: b.Free, Back: b.Back, Breast: b.Breast, Fly: b.Fly, IM: b.IM}
}

type createPracticeBody struct {
	Date          string              `json:"date"           validate:"required"`
	GroupID       string              `json:"group_id"`
	Type          string              `json:"type"           validate:"required"`
	Description   string              `json:"description"`
	YardageTarget int                 `json:"yardage_target"`
	Attendance    *int                `json:"attendance"`
	Strokes       practiceStrokesBody `json:"strokes"`
	Notes         string              `json:"notes"`
}

// createPractice handles POST /api/v1/practices
func (h *TrainingHandlers) createPractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may create practices.", nil)
		return
	}

	var body createPracticeBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}

	strokes := body.Strokes.toStrokes()

	p, err := h.service.Create(
		orgID, body.Date, body.GroupID, body.Type, body.Description,
		body.YardageTarget, body.Attendance, strokes, body.Notes, principal.UserID,
	)
	if err != nil {
		if errors.Is(err, training.ErrInvalidType) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid practice type.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create practice.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "practice.create", "practice", p.ID, nil, p, nil)

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"practice": p})
}

// getPractice handles GET /api/v1/practices/{id}
func (h *TrainingHandlers) getPractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	p, ok := h.visiblePractice(w, r, principal, orgID, id)
	if !ok {
		return
	}
	redactPracticeForViewer(principal, p)
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"practice": p})
}

type updatePracticeBody struct {
	Date          *string              `json:"date"`
	GroupID       *string              `json:"group_id"`
	Type          *string              `json:"type"`
	Description   *string              `json:"description"`
	YardageTarget *int                 `json:"yardage_target"`
	Attendance    *int                 `json:"attendance"`
	Strokes       *practiceStrokesBody `json:"strokes"`
	Notes         *string              `json:"notes"`
}

// updatePractice handles PUT /api/v1/practices/{id}
func (h *TrainingHandlers) updatePractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may update practices.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	var body updatePracticeBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}

	updates := training.PracticeUpdates{
		Date:          body.Date,
		GroupID:       body.GroupID,
		Type:          body.Type,
		Description:   body.Description,
		YardageTarget: body.YardageTarget,
		Attendance:    body.Attendance,
		Notes:         body.Notes,
	}
	if body.Strokes != nil {
		s := body.Strokes.toStrokes()
		updates.Strokes = &s
	}

	p, err := h.service.Update(orgID, id, updates, principal.UserID)
	if err != nil {
		if errors.Is(err, training.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
			return
		}
		if errors.Is(err, training.ErrInvalidType) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid practice type.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update practice.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "practice.update", "practice", p.ID, nil, p, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"practice": p})
}

// deletePractice handles DELETE /api/v1/practices/{id}
func (h *TrainingHandlers) deletePractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may delete practices.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(orgID, id); err != nil {
		if errors.Is(err, training.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to delete practice.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "practice.delete", "practice", id, nil, nil, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// ─── Attendance ───────────────────────────────────────────────────────────────

type attendanceRosterEntry struct {
	SwimmerID   string `json:"swimmer_id"`
	SwimmerName string `json:"swimmer_name"`
	Status      string `json:"status,omitempty"`
	MarkedAt    string `json:"marked_at,omitempty"`
	MarkedBy    string `json:"marked_by,omitempty"`
}

// getAttendance handles GET /api/v1/practices/{id}/attendance
func (h *TrainingHandlers) getAttendance(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may view attendance.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	practice, err := h.service.Get(orgID, id)
	if err != nil {
		if errors.Is(err, training.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get practice.", err)
		}
		return
	}

	groupCode := strings.TrimSpace(practice.GroupID)
	if h.groupsService != nil && groupCode != "" {
		if grp, gerr := h.groupsService.GetGroup(principal, orgID, groupCode); gerr == nil {
			groupCode = grp.Code
		}
	}

	roster := []attendanceRosterEntry{}
	if h.membersService != nil && groupCode != "" {
		links, err := h.membersService.ListSwimmerGroupLinks(principal, orgID)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load group roster.", err)
			return
		}
		swimmers, err := h.membersService.ListSwimmers(principal, orgID, members.ListSwimmersInput{})
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load swimmers.", err)
			return
		}
		swimmersByID := make(map[string]members.SwimmerView, len(swimmers))
		for _, sw := range swimmers {
			swimmersByID[sw.ID] = sw
		}
		for _, link := range links {
			if link.GroupCode != groupCode {
				continue
			}
			sw, ok := swimmersByID[link.SwimmerID]
			if !ok {
				continue
			}
			name := strings.TrimSpace(sw.PreferredName)
			if name == "" {
				name = strings.TrimSpace(sw.FirstName + " " + sw.LastName)
			}
			roster = append(roster, attendanceRosterEntry{SwimmerID: sw.ID, SwimmerName: name})
		}
	}

	records, err := h.service.ListAttendance(orgID, id)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load attendance.", err)
		return
	}
	recordsBySwimmer := make(map[string]*training.PracticeAttendance, len(records))
	for _, rec := range records {
		recordsBySwimmer[rec.SwimmerID] = rec
	}
	for i := range roster {
		if rec, ok := recordsBySwimmer[roster[i].SwimmerID]; ok {
			roster[i].Status = rec.Status
			roster[i].MarkedAt = rec.MarkedAt.Format("2006-01-02T15:04:05Z07:00")
			roster[i].MarkedBy = rec.MarkedBy
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"roster": roster})
}

type setAttendanceBody struct {
	Status string `json:"status" validate:"required,oneof=present absent excused"`
}

// setAttendance handles PUT /api/v1/practices/{id}/attendance/{swimmerID}
func (h *TrainingHandlers) setAttendance(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may mark attendance.", nil)
		return
	}
	practiceID := chi.URLParam(r, "id")
	swimmerID := chi.URLParam(r, "swimmerID")

	var body setAttendanceBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}

	// Attendance may only be recorded for a swimmer of this org.
	if h.membersService != nil {
		if _, known := h.membersService.SwimmerHouseholdID(orgID, swimmerID); !known {
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
			return
		}
	}

	rec, err := h.service.SetAttendance(orgID, practiceID, swimmerID, body.Status, principal.UserID)
	if err != nil {
		if errors.Is(err, training.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
			return
		}
		if errors.Is(err, training.ErrInvalidAttendance) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid attendance status.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to set attendance.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "practice.attendance.update", "practice_attendance", rec.ID, nil, rec, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"attendance": rec})
}
