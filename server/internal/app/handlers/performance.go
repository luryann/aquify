package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// PerformanceHandlers handles HTTP requests for the performance/best-times domain.
type PerformanceHandlers struct {
	service      *performance.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewPerformanceHandlers returns a new PerformanceHandlers.
func NewPerformanceHandlers(svc *performance.Service) *PerformanceHandlers {
	return &PerformanceHandlers{
		service:  svc,
		validate: validator.New(),
	}
}

// NewPerformanceHandlersWithAudit returns a PerformanceHandlers with audit logging enabled.
func NewPerformanceHandlersWithAudit(svc *performance.Service, auditSvc *audit.Service) *PerformanceHandlers {
	return &PerformanceHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     validator.New(),
	}
}

// RegisterRoutes mounts all performance routes on the given router.
func (h *PerformanceHandlers) RegisterRoutes(router chi.Router) {
	// Results
	router.Post("/results", h.upsertResult)
	router.Get("/performance/best-times", h.listOrgBestTimes)
	router.Get("/performance/progression", h.listOrgProgression)

	// Per-swimmer performance
	router.Get("/swimmers/{swimmerID}/best-times", h.listBestTimes)
	router.Post("/swimmers/{swimmerID}/best-times/override", h.overrideBestTime)
	router.Get("/swimmers/{swimmerID}/progression", h.listProgression)
	router.Get("/swimmers/{swimmerID}/cut-qualification", h.evaluateCutQualification)

	// Time standards management
	router.Get("/time-standards", h.listTimeStandards)
	router.Post("/time-standards", h.createTimeStandard)
	router.Get("/time-standards/{standardID}", h.getTimeStandard)
	router.Put("/time-standards/{standardID}", h.updateTimeStandard)
	router.Delete("/time-standards/{standardID}", h.deleteTimeStandard)
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func perfScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
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
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Principal org_id must match X-Org-Id scope.", nil)
		return nil, "", false
	}
	return principal, orgID, true
}

func (h *PerformanceHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return false
	}
	if err := h.validate.Struct(dst); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return false
	}
	return true
}

// ─── Request body types ───────────────────────────────────────────────────────

type splitInputBody struct {
	SplitNumber      int `json:"split_number"       validate:"required,gt=0"`
	SplitDistance    int `json:"split_distance"     validate:"required,gt=0"`
	SplitTimeMs      int `json:"split_time_ms"      validate:"gte=0"`
	CumulativeTimeMs int `json:"cumulative_time_ms" validate:"gte=0"`
}

type upsertResultBody struct {
	SwimmerID   string           `json:"swimmer_id"   validate:"required"`
	MeetID      string           `json:"meet_id"`
	MeetEventID string           `json:"meet_event_id"`
	ImportID    string           `json:"import_id"`
	EventNumber string           `json:"event_number"`
	Distance    int              `json:"distance"     validate:"required,gt=0"`
	Stroke      string           `json:"stroke"       validate:"required,oneof=free back breast fly im free_relay medley_relay"`
	Course      string           `json:"course"       validate:"required,oneof=SCY SCM LCM"`
	Gender      string           `json:"gender"       validate:"required,oneof=M F X"`
	Heat        *int             `json:"heat"`
	Lane        *int             `json:"lane"`
	Place       *int             `json:"place"`
	SeedTimeMs  *int             `json:"seed_time_ms"`
	FinTimeMs   *int             `json:"fin_time_ms"`
	DQ          bool             `json:"dq"`
	DQCode      string           `json:"dq_code"`
	DQNote      string           `json:"dq_note"`
	IsOfficial  bool             `json:"is_official"`
	Splits      []splitInputBody `json:"splits"`
}

type overrideBestTimeBody struct {
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke"         validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course"         validate:"required,oneof=SCY SCM LCM"`
	BestTimeMs    *int   `json:"best_time_ms"`
	Reason        string `json:"reason"         validate:"required"`
	RecordedAt    string `json:"recorded_at"`
}

type createTimeStandardBody struct {
	Name          string `json:"name"           validate:"required"`
	SeasonYear    int    `json:"season_year"    validate:"required,gt=2000"`
	StandardCode  string `json:"standard_code"  validate:"required"`
	Description   string `json:"description"`
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke"         validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course"         validate:"required,oneof=SCY SCM LCM"`
	Gender        string `json:"gender"         validate:"required,oneof=M F X"`
	AgeMin        *int   `json:"age_min"`
	AgeMax        *int   `json:"age_max"`
	TimeMs        int    `json:"time_ms"        validate:"required,gt=0"`
	IsActive      bool   `json:"is_active"`
}

type updateTimeStandardBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	TimeMs      *int    `json:"time_ms"`
	IsActive    *bool   `json:"is_active"`
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// upsertResult POST /results
func (h *PerformanceHandlers) upsertResult(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body upsertResultBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	splits := make([]performance.SplitInput, 0, len(body.Splits))
	for _, sp := range body.Splits {
		splits = append(splits, performance.SplitInput{
			SplitNumber:      sp.SplitNumber,
			SplitDistance:    sp.SplitDistance,
			SplitTimeMs:      sp.SplitTimeMs,
			CumulativeTimeMs: sp.CumulativeTimeMs,
		})
	}

	result, err := h.service.UpsertResult(orgID, principal.UserID, principal, performance.UpsertResultInput{
		SwimmerID:   body.SwimmerID,
		MeetID:      body.MeetID,
		MeetEventID: body.MeetEventID,
		ImportID:    body.ImportID,
		EventNumber: body.EventNumber,
		Distance:    body.Distance,
		Stroke:      body.Stroke,
		Course:      body.Course,
		Gender:      body.Gender,
		Heat:        body.Heat,
		Lane:        body.Lane,
		Place:       body.Place,
		SeedTimeMs:  body.SeedTimeMs,
		FinTimeMs:   body.FinTimeMs,
		DQ:          body.DQ,
		DQCode:      body.DQCode,
		DQNote:      body.DQNote,
		IsOfficial:  body.IsOfficial,
		Splits:      splits,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to upsert results.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"result": result})
}

// listBestTimes GET /swimmers/{swimmerID}/best-times
// Allowed roles: owner, admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) listBestTimes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	filters := performance.ListBestTimesFilters{
		Course: r.URL.Query().Get("course"),
		Stroke: r.URL.Query().Get("stroke"),
	}

	bestTimes, err := h.service.ListBestTimes(orgID, principal, swimmerID, filters)
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read best times.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer_id": swimmerID,
		"count":      len(bestTimes),
		"best_times": bestTimes,
	})
}

// listOrgBestTimes GET /performance/best-times
func (h *PerformanceHandlers) listOrgBestTimes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	bestTimes, err := h.service.ListBestTimesForOrg(orgID, principal, performance.ListBestTimesFilters{
		SwimmerID: r.URL.Query().Get("swimmer_id"),
		Course:    r.URL.Query().Get("course"),
		Stroke:    r.URL.Query().Get("stroke"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read best times.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":      len(bestTimes),
		"best_times": bestTimes,
	})
}

// overrideBestTime POST /swimmers/{swimmerID}/best-times/override
func (h *PerformanceHandlers) overrideBestTime(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	var body overrideBestTimeBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	bt, internalAudit, err := h.service.OverrideBestTime(orgID, principal.UserID, principal, performance.OverrideBestTimeInput{
		SwimmerID:     swimmerID,
		EventDistance: body.EventDistance,
		Stroke:        body.Stroke,
		Course:        body.Course,
		BestTimeMs:    body.BestTimeMs,
		Reason:        body.Reason,
		RecordedAt:    body.RecordedAt,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may override best times.", nil)
			return
		}
		if errors.Is(err, performance.ErrOverrideRequiresReason) {
			// 422 Unprocessable Entity: request is well-formed but fails semantic validation.
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "override_requires_reason", err.Error(), nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	if h.auditService != nil {
		meta := map[string]string{"swimmer_id": swimmerID}
		if body.Reason != "" {
			meta["reason"] = body.Reason
		}
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionPerformanceBestTimeOverride,
			ResourceType: "best_time",
			ResourceID:   bt.ID,
			After:        bt,
			Metadata:     meta,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"best_time":   bt,
		"audit_event": internalAudit,
	})
}

// listProgression GET /swimmers/{swimmerID}/progression
// Allowed roles: owner, admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) listProgression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	q := r.URL.Query()
	distanceStr := q.Get("distance")
	var distance int
	if distanceStr != "" {
		d, err := strconv.Atoi(distanceStr)
		if err == nil {
			distance = d
		}
	}

	results, err := h.service.ListProgression(orgID, principal, swimmerID, performance.ListResultsFilters{
		EventDistance: distance,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read progression data.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer_id": swimmerID,
		"count":      len(results),
		"results":    results,
	})
}

// listOrgProgression GET /performance/progression
func (h *PerformanceHandlers) listOrgProgression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	distanceStr := q.Get("distance")
	var distance int
	if distanceStr != "" {
		d, err := strconv.Atoi(distanceStr)
		if err == nil {
			distance = d
		}
	}

	results, err := h.service.ListProgressionForOrg(orgID, principal, performance.ListResultsFilters{
		SwimmerID:     q.Get("swimmer_id"),
		EventDistance: distance,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read progression data.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

// evaluateCutQualification GET /swimmers/{swimmerID}/cut-qualification?standard_id=...
// Allowed roles: owner, admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) evaluateCutQualification(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")
	standardID := r.URL.Query().Get("standard_id")
	if strings.TrimSpace(standardID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_standard_id", "Query parameter 'standard_id' is required.", nil)
		return
	}

	qual, err := h.service.EvaluateCutQualification(orgID, principal, swimmerID, standardID)
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read cut qualification.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"qualification": qual})
}

// listTimeStandards GET /time-standards
func (h *PerformanceHandlers) listTimeStandards(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	var yearFilter int
	if y := q.Get("season_year"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			yearFilter = n
		}
	}
	var distFilter int
	if d := q.Get("event_distance"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			distFilter = n
		}
	}
	activeOnly := q.Get("active_only") == "true" || q.Get("active_only") == "1"

	standards, err := h.service.ListTimeStandards(orgID, performance.ListTimeStandardsFilters{
		SeasonYear:    yearFilter,
		StandardCode:  q.Get("standard_code"),
		EventDistance: distFilter,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
		Gender:        q.Get("gender"),
		ActiveOnly:    activeOnly,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":          len(standards),
		"time_standards": standards,
	})
}

// createTimeStandard POST /time-standards
func (h *PerformanceHandlers) createTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body createTimeStandardBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	ts, err := h.service.CreateTimeStandard(orgID, principal.UserID, principal, performance.CreateTimeStandardInput{
		Name:          body.Name,
		SeasonYear:    body.SeasonYear,
		StandardCode:  body.StandardCode,
		Description:   body.Description,
		EventDistance: body.EventDistance,
		Stroke:        body.Stroke,
		Course:        body.Course,
		Gender:        body.Gender,
		AgeMin:        body.AgeMin,
		AgeMax:        body.AgeMax,
		TimeMs:        body.TimeMs,
		IsActive:      body.IsActive,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"time_standard": ts})
}

// getTimeStandard GET /time-standards/{standardID}
func (h *PerformanceHandlers) getTimeStandard(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	ts, err := h.service.GetTimeStandard(orgID, standardID)
	if err != nil {
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"time_standard": ts})
}

// updateTimeStandard PUT /time-standards/{standardID}
func (h *PerformanceHandlers) updateTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	var body updateTimeStandardBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return
	}

	ts, err := h.service.UpdateTimeStandard(orgID, principal.UserID, standardID, principal, performance.UpdateTimeStandardInput{
		Name:        body.Name,
		Description: body.Description,
		TimeMs:      body.TimeMs,
		IsActive:    body.IsActive,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"time_standard": ts})
}

// deleteTimeStandard DELETE /time-standards/{standardID}
func (h *PerformanceHandlers) deleteTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	if err := h.service.DeleteTimeStandard(orgID, principal.UserID, standardID, principal); err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": standardID})
}
