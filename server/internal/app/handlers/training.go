package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// TrainingHandlers handles practice/schedule CRUD endpoints.
type TrainingHandlers struct {
	service        *training.Service
	membersService *members.Service
	groupsService  *groups.Service
	auditService   *audit.Service
	validate       *validator.Validate
}

// NewTrainingHandlers returns a new TrainingHandlers.
func NewTrainingHandlers(svc *training.Service, auditSvc *audit.Service) *TrainingHandlers {
	return &TrainingHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     validator.New(),
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
		validate:       validator.New(),
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
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func trainingScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// listPractices handles GET /api/v1/practices
func (h *TrainingHandlers) listPractices(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := trainingScopeFromRequest(w, r)
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
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"practices": practices})
}

type practiceStrokesBody struct {
	Free   int `json:"free"`
	Back   int `json:"back"`
	Breast int `json:"breast"`
	Fly    int `json:"fly"`
	IM     int `json:"im"`
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
	principal, orgID, ok := trainingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may create practices.", nil)
		return
	}

	var body createPracticeBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	strokes := training.PracticeStrokes{
		Free:   body.Strokes.Free,
		Back:   body.Strokes.Back,
		Breast: body.Strokes.Breast,
		Fly:    body.Strokes.Fly,
		IM:     body.Strokes.IM,
	}

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

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "practice.create",
			ResourceType: "practice",
			ResourceID:   p.ID,
			After:        p,
		})
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"practice": p})
}

// getPractice handles GET /api/v1/practices/{id}
func (h *TrainingHandlers) getPractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := trainingScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	p, err := h.service.Get(orgID, id)
	if err != nil {
		if errors.Is(err, training.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get practice.", nil)
		}
		return
	}
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
	principal, orgID, ok := trainingScopeFromRequest(w, r)
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
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
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
		s := training.PracticeStrokes{
			Free:   body.Strokes.Free,
			Back:   body.Strokes.Back,
			Breast: body.Strokes.Breast,
			Fly:    body.Strokes.Fly,
			IM:     body.Strokes.IM,
		}
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

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "practice.update",
			ResourceType: "practice",
			ResourceID:   p.ID,
			After:        p,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"practice": p})
}

// deletePractice handles DELETE /api/v1/practices/{id}
func (h *TrainingHandlers) deletePractice(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := trainingScopeFromRequest(w, r)
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

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "practice.delete",
			ResourceType: "practice",
			ResourceID:   id,
		})
	}

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
	principal, orgID, ok := trainingScopeFromRequest(w, r)
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get practice.", nil)
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
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load group roster.", nil)
			return
		}
		swimmers, err := h.membersService.ListSwimmers(principal, orgID, members.ListSwimmersInput{})
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load swimmers.", nil)
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
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load attendance.", nil)
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
	principal, orgID, ok := trainingScopeFromRequest(w, r)
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
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.", httpx.FormatValidationErrors(err))
		return
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

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "practice.attendance.update",
			ResourceType: "practice_attendance",
			ResourceID:   rec.ID,
			After:        rec,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"attendance": rec})
}
