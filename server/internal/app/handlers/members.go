package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type MembershipHandlers struct {
	service      *members.Service
	auditService *audit.Service
	validate     *validator.Validate
}

func NewMembershipHandlers(service *members.Service) *MembershipHandlers {
	return &MembershipHandlers{
		service:  service,
		validate: validator.New(),
	}
}

// NewMembershipHandlersWithAudit returns a MembershipHandlers with audit logging enabled.
func NewMembershipHandlersWithAudit(service *members.Service, auditSvc *audit.Service) *MembershipHandlers {
	return &MembershipHandlers{
		service:      service,
		auditService: auditSvc,
		validate:     validator.New(),
	}
}

func (h *MembershipHandlers) RegisterProtectedRoutes(router chi.Router) {
	router.Post("/households", h.createHousehold)
	router.Get("/households", h.listHouseholds)
	router.Get("/households/{householdID}", h.getHousehold)
	router.Patch("/households/{householdID}", h.updateHousehold)
	router.Delete("/households/{householdID}", h.deleteHousehold)
	router.Post("/households/{householdID}/contacts", h.addContact)
	router.Get("/households/{householdID}/contacts", h.listContacts)
	router.Post("/swimmers", h.createSwimmer)
	router.Get("/swimmers/{swimmerID}", h.getSwimmer)
	router.Patch("/swimmers/{swimmerID}", h.updateSwimmer)
	router.Delete("/swimmers/{swimmerID}", h.deleteSwimmer)
	router.Post("/swimmers/{swimmerID}/groups", h.assignSwimmerGroup)
	router.Get("/swimmer-groups", h.listSwimmerGroupLinks)
	router.Get("/swimmers", h.listSwimmers)
}

type createHouseholdRequest struct {
	OrgID            string `json:"org_id" validate:"required"`
	HouseholdName    string `json:"household_name" validate:"required"`
	PrimaryEmail     string `json:"primary_email"`
	PrimaryPhoneE164 string `json:"primary_phone_e164"`
	Status           string `json:"status" validate:"omitempty,oneof=active inactive archived"`
	Notes            string `json:"notes"`
}

func (h *MembershipHandlers) createHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createHouseholdRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}
	if !validateBodyOrgScope(w, r, orgID, req.OrgID) {
		return
	}

	household, err := h.service.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:            req.OrgID,
		HouseholdName:    req.HouseholdName,
		PrimaryEmail:     req.PrimaryEmail,
		PrimaryPhoneE164: req.PrimaryPhoneE164,
		Status:           req.Status,
		Notes:            req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to create households.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_household_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":    "created",
		"household": household,
	})
}

func (h *MembershipHandlers) listHouseholds(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	households, err := h.service.ListHouseholds(principal, orgID)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list households.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	households, total := paginateSlice(households, pagination)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"count":      len(households),
		"total":      total,
		"limit":      pagination.Limit,
		"offset":     pagination.Offset,
		"households": households,
	})
}

func (h *MembershipHandlers) getHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	householdID := strings.TrimSpace(chi.URLParam(r, "householdID"))
	household, err := h.service.GetHousehold(principal, orgID, householdID)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only view own household.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view households.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"household": household,
	})
}

type updateHouseholdRequest struct {
	HouseholdName    *string `json:"household_name"`
	PrimaryEmail     *string `json:"primary_email"`
	PrimaryPhoneE164 *string `json:"primary_phone_e164"`
	Status           *string `json:"status" validate:"omitempty,oneof=active inactive archived"`
	Notes            *string `json:"notes"`
}

func (h *MembershipHandlers) updateHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req updateHouseholdRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	householdID := strings.TrimSpace(chi.URLParam(r, "householdID"))
	household, err := h.service.UpdateHousehold(principal, orgID, householdID, members.UpdateHouseholdInput{
		HouseholdName:    req.HouseholdName,
		PrimaryEmail:     req.PrimaryEmail,
		PrimaryPhoneE164: req.PrimaryPhoneE164,
		Status:           req.Status,
		Notes:            req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only modify own household.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to update households.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_household_update", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "updated",
		"household": household,
	})
}

type addContactRequest struct {
	Relationship       string `json:"relationship" validate:"required"`
	FirstName          string `json:"first_name" validate:"required"`
	LastName           string `json:"last_name" validate:"required"`
	Email              string `json:"email"`
	PhoneE164          string `json:"phone_e164"`
	IsPrimary          bool   `json:"is_primary"`
	IsEmergencyContact bool   `json:"is_emergency_contact"`
	CanPickUp          bool   `json:"can_pick_up"`
	EmergencyPriority  *int   `json:"emergency_priority"`
	Notes              string `json:"notes"`
}

func (h *MembershipHandlers) deleteHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	householdID := strings.TrimSpace(chi.URLParam(r, "householdID"))
	household, err := h.service.DeleteHousehold(principal, orgID, householdID)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrHouseholdHasSwimmers):
			httpx.WriteError(w, r, http.StatusConflict, "household_has_swimmers", "Household cannot be deleted while swimmers are still assigned to it.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only owner or admin can delete households.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMembersHouseholdDelete,
			ResourceType: "household",
			ResourceID:   householdID,
			Before:       map[string]any{"household": household},
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "deleted",
		"id":     householdID,
	})
}

func (h *MembershipHandlers) addContact(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req addContactRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	householdID := strings.TrimSpace(chi.URLParam(r, "householdID"))
	contact, err := h.service.AddContact(principal, orgID, householdID, members.AddContactInput{
		Relationship:       req.Relationship,
		FirstName:          req.FirstName,
		LastName:           req.LastName,
		Email:              req.Email,
		PhoneE164:          req.PhoneE164,
		IsPrimary:          req.IsPrimary,
		IsEmergencyContact: req.IsEmergencyContact,
		CanPickUp:          req.CanPickUp,
		EmergencyPriority:  req.EmergencyPriority,
		Notes:              req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only modify own household.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to add contacts.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_contact_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":  "created",
		"contact": contact,
	})
}

func (h *MembershipHandlers) listContacts(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	householdID := strings.TrimSpace(chi.URLParam(r, "householdID"))
	contacts, err := h.service.ListContacts(principal, orgID, householdID)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only view contacts for own household.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list contacts.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"count":        len(contacts),
		"total":        len(contacts),
		"limit":        len(contacts),
		"offset":       0,
		"household_id": householdID,
		"contacts":     contacts,
	})
}

type createSwimmerRequest struct {
	OrgID              string   `json:"org_id" validate:"required"`
	HouseholdID        string   `json:"household_id" validate:"required"`
	FirstName          string   `json:"first_name" validate:"required"`
	LastName           string   `json:"last_name" validate:"required"`
	PreferredName      string   `json:"preferred_name"`
	DateOfBirth        string   `json:"date_of_birth" validate:"required"`
	Gender             string   `json:"gender" validate:"required,oneof=female male non_binary undisclosed"`
	SwimmerExternalID  string   `json:"swimmer_external_id"`
	USASwimmingID      string   `json:"usa_swimming_id"`
	RegistrationStatus string   `json:"registration_status" validate:"omitempty,oneof=pending active inactive archived"`
	MedicalFlags       []string `json:"medical_flags"`
	Notes              string   `json:"notes"`
}

func (h *MembershipHandlers) createSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createSwimmerRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}
	if !validateBodyOrgScope(w, r, orgID, req.OrgID) {
		return
	}

	swimmer, auditEvent, err := h.service.CreateSwimmer(principal, orgID, members.CreateSwimmerInput{
		HouseholdID:        req.HouseholdID,
		FirstName:          req.FirstName,
		LastName:           req.LastName,
		PreferredName:      req.PreferredName,
		DateOfBirth:        req.DateOfBirth,
		Gender:             req.Gender,
		SwimmerExternalID:  req.SwimmerExternalID,
		USASwimmingID:      req.USASwimmingID,
		RegistrationStatus: req.RegistrationStatus,
		MedicalFlags:       req.MedicalFlags,
		Notes:              req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only create swimmers in own household.", nil)
		case errors.Is(err, members.ErrMedicalFlagsRestricted):
			httpx.WriteError(w, r, http.StatusForbidden, "medical_flags_restricted", err.Error(), nil)
		case errors.Is(err, members.ErrInvalidDateOfBirth):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_date_of_birth", err.Error(), nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to create swimmers.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_swimmer_request", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"swimmer":     swimmer,
		"audit_event": auditEvent,
	})
}

func (h *MembershipHandlers) getSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))
	includeMedical := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_medical")), "true")
	swimmer, err := h.service.GetSwimmer(principal, orgID, swimmerID, includeMedical)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only view own household swimmers.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view swimmers.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"swimmer": swimmer,
	})
}

type updateSwimmerRequest struct {
	FirstName                    *string   `json:"first_name"`
	LastName                     *string   `json:"last_name"`
	PreferredName                *string   `json:"preferred_name"`
	DateOfBirth                  *string   `json:"date_of_birth"`
	Gender                       *string   `json:"gender" validate:"omitempty,oneof=female male non_binary undisclosed"`
	SwimmerExternalID            *string   `json:"swimmer_external_id"`
	USASwimmingID                *string   `json:"usa_swimming_id"`
	RegistrationStatus           *string   `json:"registration_status" validate:"omitempty,oneof=pending active inactive archived"`
	MedicalFlags                 *[]string `json:"medical_flags"`
	Notes                        *string   `json:"notes"`
	EmergencyContactName         *string   `json:"emergency_contact_name"`
	EmergencyContactPhone        *string   `json:"emergency_contact_phone"`
	EmergencyContactRelationship *string   `json:"emergency_contact_relationship"`
	Allergies                    *string   `json:"allergies"`
}

func (h *MembershipHandlers) updateSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req updateSwimmerRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))
	swimmer, auditEvent, err := h.service.UpdateSwimmer(principal, orgID, swimmerID, members.UpdateSwimmerInput{
		FirstName:                    req.FirstName,
		LastName:                     req.LastName,
		PreferredName:                req.PreferredName,
		DateOfBirth:                  req.DateOfBirth,
		Gender:                       req.Gender,
		SwimmerExternalID:            req.SwimmerExternalID,
		USASwimmingID:                req.USASwimmingID,
		RegistrationStatus:           req.RegistrationStatus,
		MedicalFlags:                 req.MedicalFlags,
		Notes:                        req.Notes,
		EmergencyContactName:         req.EmergencyContactName,
		EmergencyContactPhone:        req.EmergencyContactPhone,
		EmergencyContactRelationship: req.EmergencyContactRelationship,
		Allergies:                    req.Allergies,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Parent can only update own household swimmers.", nil)
		case errors.Is(err, members.ErrMedicalFlagsRestricted):
			httpx.WriteError(w, r, http.StatusForbidden, "medical_flags_restricted", err.Error(), nil)
		case errors.Is(err, members.ErrInvalidDateOfBirth):
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_date_of_birth", err.Error(), nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to update swimmers.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_swimmer_update", err.Error(), nil)
		}
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMembersSwimmerUpdate,
			ResourceType: "swimmer",
			ResourceID:   swimmerID,
			Before:       auditEvent.Before,
			After:        swimmer,
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "updated",
		"swimmer":     swimmer,
		"audit_event": auditEvent,
	})
}

func (h *MembershipHandlers) deleteSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))
	swimmer, dependencies, err := h.service.DeleteSwimmer(principal, orgID, swimmerID)
	if err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, members.ErrSwimmerHasDependencies):
			httpx.WriteError(w, r, http.StatusConflict, "swimmer_has_dependencies", buildSwimmerDeleteDependencyMessage(dependencies), map[string]any{
				"dependencies": dependencies,
			})
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only owner or admin can delete swimmers.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionMembersSwimmerDelete,
			ResourceType: "swimmer",
			ResourceID:   swimmerID,
			Before:       map[string]any{"swimmer": swimmer},
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "deleted",
		"id":     swimmerID,
	})
}

func buildSwimmerDeleteDependencyMessage(dependencies []members.SwimmerDeleteDependency) string {
	if len(dependencies) == 0 {
		return "Swimmer cannot be deleted while related records still exist."
	}

	parts := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		parts = append(parts, dependency.Label+" ("+strconv.Itoa(dependency.Count)+")")
	}
	return "Swimmer cannot be deleted while related records still exist: " + strings.Join(parts, ", ") + "."
}

type assignSwimmerGroupRequest struct {
	GroupCode string `json:"group_code" validate:"required"`
	IsPrimary bool   `json:"is_primary"`
}

func (h *MembershipHandlers) assignSwimmerGroup(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req assignSwimmerGroupRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))
	assignment, err := h.service.AssignSwimmerToGroup(principal, orgID, swimmerID, members.AssignSwimmerGroupInput{
		GroupCode: req.GroupCode,
		IsPrimary: req.IsPrimary,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only owner/admin/coach can assign swimmer groups.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_group_assignment", err.Error(), nil)
		}
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":     "assigned",
		"assignment": assignment,
	})
}

func (h *MembershipHandlers) listSwimmers(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	includeMedical, _ := strconv.ParseBool(strings.TrimSpace(r.URL.Query().Get("include_medical")))
	swimmers, err := h.service.ListSwimmers(principal, orgID, members.ListSwimmersInput{
		HouseholdID:        strings.TrimSpace(r.URL.Query().Get("household_id")),
		GroupCode:          strings.TrimSpace(r.URL.Query().Get("group_code")),
		RegistrationStatus: strings.TrimSpace(r.URL.Query().Get("registration_status")),
		IncludeMedical:     includeMedical,
	})
	if err != nil {
		switch {
		case errors.Is(err, members.ErrParentHouseholdScope):
			httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", err.Error(), nil)
		case errors.Is(err, members.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list swimmers.", nil)
		default:
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_swimmer_filter", err.Error(), nil)
		}
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	swimmers, total := paginateSlice(swimmers, pagination)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"count":    len(swimmers),
		"total":    total,
		"limit":    pagination.Limit,
		"offset":   pagination.Offset,
		"swimmers": swimmers,
	})
}

func (h *MembershipHandlers) listSwimmerGroupLinks(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := membershipScopeFromRequest(w, r)
	if !ok {
		return
	}

	links, err := h.service.ListSwimmerGroupLinks(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to list swimmer groups.", nil)
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	links, total := paginateSlice(links, pagination)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"count":          len(links),
		"total":          total,
		"limit":          pagination.Limit,
		"offset":         pagination.Offset,
		"swimmer_groups": links,
	})
}

func (h *MembershipHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateJSON(w, r, dst, h.validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusBadRequest,
		ValidationCode:    "validation_failed",
		ValidationMessage: "One or more request fields are invalid.",
	})
}

func membershipScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}
