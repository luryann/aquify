package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// GroupsHandlers handles HTTP requests for groups, eligibility, and registrations.
type GroupsHandlers struct {
	service  *groups.Service
	validate *validator.Validate
}

// NewGroupsHandlers returns a new GroupsHandlers.
func NewGroupsHandlers(service *groups.Service) *GroupsHandlers {
	return &GroupsHandlers{
		service:  service,
		validate: validator.New(),
	}
}

// RegisterRoutes mounts all groups, eligibility, and registrations routes on the given router.
func (h *GroupsHandlers) RegisterRoutes(router chi.Router) {
	// Groups
	router.Post("/groups", h.createGroup)
	router.Patch("/groups/{groupID}", h.updateGroup)
	router.Get("/groups", h.listGroups)
	router.Get("/groups/{groupID}", h.getGroup)

	// Eligibility rules
	router.Post("/groups/{groupID}/eligibility-rules", h.attachEligibilityRule)
	router.Get("/groups/{groupID}/eligibility-rules", h.listEligibilityRules)
	router.Get("/groups/{groupID}/eligibility/{swimmerID}", h.evaluateEligibility)

	// Registrations
	router.Get("/registrations", h.listRegistrations)
	router.Post("/registrations", h.createRegistration)
	router.Post("/registrations/{registrationID}/transition", h.transitionRegistration)
	router.Post("/registrations/{registrationID}/waivers", h.acceptWaiver)
}

// ─── Groups ──────────────────────────────────────────────────────────────────

type createGroupRequest struct {
	Code        string `json:"code" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	Type        string `json:"type" validate:"omitempty,oneof=competitive recreational developmental masters other"`
	MinAge      *int   `json:"min_age"`
	MaxAge      *int   `json:"max_age"`
	CoachUserID string `json:"coach_user_id"`
}

func (h *GroupsHandlers) createGroup(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createGroupRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	group, auditEvent, err := h.service.CreateGroup(principal, orgID, groups.CreateGroupInput{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		MinAge:      req.MinAge,
		MaxAge:      req.MaxAge,
		CoachUserID: req.CoachUserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to create groups.", nil)
		case errors.Is(err, groups.ErrGroupCodeConflict):
			httpx.WriteError(w, r, http.StatusConflict, "group_code_conflict", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_group_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"group":       group,
		"audit_event": auditEvent,
	})
}

type updateGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Type        *string `json:"type" validate:"omitempty,oneof=competitive recreational developmental masters other"`
	MinAge      *int    `json:"min_age"`
	MaxAge      *int    `json:"max_age"`
	CoachUserID *string `json:"coach_user_id"`
	IsActive    *bool   `json:"is_active"`
}

func (h *GroupsHandlers) updateGroup(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req updateGroupRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	group, auditEvent, err := h.service.UpdateGroup(principal, orgID, groupID, groups.UpdateGroupInput{
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		MinAge:      req.MinAge,
		MaxAge:      req.MaxAge,
		CoachUserID: req.CoachUserID,
		IsActive:    req.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to update groups.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_group_update", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "updated",
		"group":       group,
		"audit_event": auditEvent,
	})
}

func (h *GroupsHandlers) getGroup(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	group, err := h.service.GetGroup(principal, orgID, groupID)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view groups.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"group":  group,
	})
}

func (h *GroupsHandlers) listGroups(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	activeOnly, _ := strconv.ParseBool(strings.TrimSpace(r.URL.Query().Get("active_only")))
	result, err := h.service.ListGroups(principal, orgID, activeOnly)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list groups.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	result, total := paginateSlice(result, pagination)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(result),
		"total":  total,
		"limit":  pagination.Limit,
		"offset": pagination.Offset,
		"groups": result,
	})
}

// ─── Eligibility ─────────────────────────────────────────────────────────────

type attachEligibilityRuleRequest struct {
	RuleType   string         `json:"rule_type" validate:"required,oneof=age_range gender registration_status"`
	Parameters map[string]any `json:"parameters"`
}

func (h *GroupsHandlers) attachEligibilityRule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req attachEligibilityRuleRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	rule, auditEvent, err := h.service.AttachEligibilityRule(principal, orgID, groupID, groups.AttachEligibilityRuleInput{
		RuleType:   req.RuleType,
		Parameters: req.Parameters,
	})
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin can attach eligibility rules.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_rule_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "attached",
		"rule":        rule,
		"audit_event": auditEvent,
	})
}

func (h *GroupsHandlers) listEligibilityRules(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	rules, err := h.service.ListEligibilityRules(principal, orgID, groupID)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view eligibility rules.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"count":    len(rules),
		"total":    len(rules),
		"limit":    len(rules),
		"offset":   0,
		"group_id": groupID,
		"rules":    rules,
	})
}

func (h *GroupsHandlers) evaluateEligibility(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))

	result, err := h.service.EvaluateSwimmerEligibility(principal, orgID, groupID, swimmerID)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to evaluate eligibility.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"result": result,
	})
}

func (h *GroupsHandlers) listRegistrations(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	if !hasRegistrationListAccess(principal) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list registrations.", nil)
		return
	}

	registrations, err := h.service.ListRegistrations(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list registrations.", nil)
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	registrations, total := paginateSlice(registrations, pagination)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"count":         len(registrations),
		"total":         total,
		"limit":         pagination.Limit,
		"offset":        pagination.Offset,
		"registrations": registrations,
	})
}

// ─── Registrations ───────────────────────────────────────────────────────────

type createRegistrationRequest struct {
	HouseholdID      string `json:"household_id" validate:"required"`
	SwimmerID        string `json:"swimmer_id" validate:"required"`
	GroupID          string `json:"group_id" validate:"required"`
	Season           string `json:"season" validate:"required"`
	RegistrationDate string `json:"registration_date"`
	Notes            string `json:"notes"`
}

func (h *GroupsHandlers) createRegistration(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createRegistrationRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	reg, err := h.service.CreateRegistration(principal, orgID, groups.CreateRegistrationInput{
		HouseholdID:      req.HouseholdID,
		SwimmerID:        req.SwimmerID,
		GroupID:          req.GroupID,
		Season:           req.Season,
		RegistrationDate: req.RegistrationDate,
		Notes:            req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to create registrations.", nil)
		case errors.Is(err, groups.ErrGroupNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "group_not_found", "Group was not found.", nil)
		case errors.Is(err, groups.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, groups.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, groups.ErrDuplicateRegistration):
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_registration", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_registration_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":       "created",
		"registration": reg,
	})
}

type transitionRegistrationRequest struct {
	Status string `json:"status" validate:"required,oneof=active inactive withdrawn"`
}

func (h *GroupsHandlers) transitionRegistration(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req transitionRegistrationRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	registrationID := strings.TrimSpace(chi.URLParam(r, "registrationID"))
	reg, auditEvent, err := h.service.TransitionRegistrationStatus(principal, orgID, registrationID, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrRegistrationNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "registration_not_found", "Registration was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
		case errors.Is(err, groups.ErrInvalidTransition):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_transition", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_transition_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "updated",
		"registration": reg,
		"audit_event":  auditEvent,
	})
}

type acceptWaiverRequest struct {
	WaiverType string `json:"waiver_type" validate:"required"`
	IPAddress  string `json:"ip_address"`
}

func (h *GroupsHandlers) acceptWaiver(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := groupScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req acceptWaiverRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	registrationID := strings.TrimSpace(chi.URLParam(r, "registrationID"))
	waiver, err := h.service.AcceptWaiver(principal, orgID, registrationID, groups.AcceptWaiverInput{
		WaiverType: req.WaiverType,
		IPAddress:  req.IPAddress,
	})
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrRegistrationNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "registration_not_found", "Registration was not found.", nil)
		case errors.Is(err, groups.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to accept waivers.", nil)
		case errors.Is(err, groups.ErrWaiverAlreadyAccepted):
			httpx.WriteError(w, r, http.StatusConflict, "waiver_already_accepted", err.Error(), nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_waiver_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status": "accepted",
		"waiver": waiver,
	})
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func (h *GroupsHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateJSON(w, r, dst, h.validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusBadRequest,
		ValidationCode:    "validation_failed",
		ValidationMessage: "One or more request fields are invalid.",
	})
}

func groupScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

func hasRegistrationListAccess(principal *appmiddleware.Principal) bool {
	if principal == nil {
		return false
	}
	for _, role := range principal.Roles {
		switch strings.TrimSpace(strings.ToLower(role)) {
		case "admin", "coach", "parent":
			return true
		}
	}
	return false
}
