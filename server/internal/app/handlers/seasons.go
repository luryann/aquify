package handlers

import (
	"errors"
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// SeasonsHandlers handles season CRUD endpoints.
type SeasonsHandlers struct {
	service      *seasons.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewSeasonsHandlers returns a new SeasonsHandlers.
func NewSeasonsHandlers(svc *seasons.Service, auditSvc *audit.Service) *SeasonsHandlers {
	return &SeasonsHandlers{
		service:      svc,
		auditService: auditSvc,
		validate:     validator.New(),
	}
}

// RegisterRoutes mounts all season routes on the protected router.
func (h *SeasonsHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/seasons", h.listSeasons)
	router.Post("/seasons", h.createSeason)
	router.Get("/seasons/{id}", h.getSeason)
	router.Put("/seasons/{id}", h.updateSeason)
	router.Delete("/seasons/{id}", h.deleteSeason)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// listSeasons handles GET /api/v1/seasons
func (h *SeasonsHandlers) listSeasons(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	list, err := h.service.List(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list seasons.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"seasons": list})
}

type createSeasonBody struct {
	Name      string `json:"name"       validate:"required"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"     validate:"required"`
	Goals     string `json:"goals"`
}

// createSeason handles POST /api/v1/seasons
func (h *SeasonsHandlers) createSeason(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may create seasons.", nil)
		return
	}

	var body createSeasonBody
	if !decodeAndValidateLegacy(w, r, &body, h.validate, "Validation failed.") {
		return
	}

	season, err := h.service.Create(orgID, body.Name, body.StartDate, body.EndDate, body.Status, body.Goals, principal.UserID)
	if err != nil {
		if errors.Is(err, seasons.ErrInvalidStatus) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid season status.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create season.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "season.create", "season", season.ID, nil, season, nil)

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"season": season})
}

// getSeason handles GET /api/v1/seasons/{id}
func (h *SeasonsHandlers) getSeason(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	season, err := h.service.Get(orgID, id)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Season not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get season.", err)
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"season": season})
}

type updateSeasonBody struct {
	Name      *string `json:"name"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Status    *string `json:"status"`
	Goals     *string `json:"goals"`
}

// updateSeason handles PUT /api/v1/seasons/{id}
func (h *SeasonsHandlers) updateSeason(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may update seasons.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	var body updateSeasonBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}

	updates := seasons.SeasonUpdates{
		Name:      body.Name,
		StartDate: body.StartDate,
		EndDate:   body.EndDate,
		Status:    body.Status,
		Goals:     body.Goals,
	}

	season, err := h.service.Update(orgID, id, updates, principal.UserID)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Season not found.", nil)
			return
		}
		if errors.Is(err, seasons.ErrInvalidStatus) {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid season status.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update season.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "season.update", "season", season.ID, nil, season, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"season": season})
}

// deleteSeason handles DELETE /api/v1/seasons/{id}
func (h *SeasonsHandlers) deleteSeason(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may delete seasons.", nil)
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(orgID, id); err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Season not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to delete season.", err)
		return
	}

	recordAudit(h.auditService, principal, orgID, "season.delete", "season", id, nil, nil, nil)

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}
