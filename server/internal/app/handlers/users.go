package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/go-chi/chi/v5"
)

// roleTier maps each role name to its numeric tier (higher = more privileged).
// An actor may only assign/remove roles at or below their own tier.
var roleTier = map[string]int{
	"owner":           5,
	"admin":           4,
	"coach":           3,
	"billing":         2,
	"parent":          1,
	"swimmer":         1,
	"read_only_staff": 1,
}

// actorMaxTier returns the highest tier among the actor's roles.
func actorMaxTier(roles []string) int {
	max := 0
	for _, r := range roles {
		if t := roleTier[r]; t > max {
			max = t
		}
	}
	return max
}

// UserHandlers implements HTTP handlers for Keycloak-backed user management.
type UserHandlers struct {
	kc               keycloak.Client
	auditService     *audit.Service
	membershipService *members.Service
}

// NewUserHandlers returns a UserHandlers wired to the given Keycloak client and
// audit service.
func NewUserHandlers(kc keycloak.Client, auditSvc *audit.Service) *UserHandlers {
	return &UserHandlers{kc: kc, auditService: auditSvc}
}

// NewUserHandlersWithMembers returns a UserHandlers with the members service
// wired in for household linkage operations.
func NewUserHandlersWithMembers(kc keycloak.Client, auditSvc *audit.Service, membersSvc *members.Service) *UserHandlers {
	return &UserHandlers{kc: kc, auditService: auditSvc, membershipService: membersSvc}
}

// RegisterPublicRoutes mounts unauthenticated user routes (e.g. self-registration).
// Must be called on an unprotected router segment.
func (h *UserHandlers) RegisterPublicRoutes(r chi.Router) {
	r.Post("/register", h.selfRegister)
}

// RegisterRoutes mounts all user management routes under the given router.
// The router must already have auth + org-scope middleware applied.
func (h *UserHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(users chi.Router) {
		// owner/admin only
		users.With(appmiddleware.RBACStub("owner", "admin")).Get("/", h.listUsers)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/", h.createUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/invite", h.inviteUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Get("/{id}", h.getUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Put("/{id}", h.updateUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/disable", h.disableUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/enable", h.enableUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Get("/{id}/roles", h.listUserRoles)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/roles", h.assignRole)
		users.With(appmiddleware.RBACStub("owner", "admin")).Delete("/{id}/roles/{role}", h.removeRole)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/password-reset", h.passwordReset)
		// approval flow (owner/admin only)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/approve", h.approveUser)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/reject", h.rejectUser)
		// household linkage (owner/admin only)
		users.With(appmiddleware.RBACStub("owner", "admin")).Get("/{id}/household", h.getUserHousehold)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/household/link", h.linkUserHousehold)
		users.With(appmiddleware.RBACStub("owner", "admin")).Post("/{id}/household/unlink", h.unlinkUserHousehold)
		// owner only (destructive)
		users.With(appmiddleware.RBACStub("owner")).Delete("/{id}", h.deleteUser)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// userScopeFromRequest extracts the principal and orgID, writes a 401/400 if
// missing, and returns (principal, orgID, true) on success.
func userScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return nil, "", false
	}
	orgID, ok := appmiddleware.OrgIDFromContext(r.Context())
	if !ok || orgID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing org scope.", nil)
		return nil, "", false
	}
	return principal, orgID, true
}

// requireUserInOrg fetches the target user and verifies org membership.
// Writes 404 or 403 on failure. Returns (user, true) on success.
func (h *UserHandlers) requireUserInOrg(w http.ResponseWriter, r *http.Request, userID, orgID string) (*keycloak.User, bool) {
	user, err := h.kc.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to fetch user.", nil)
		}
		return nil, false
	}
	if user.OrgID() != orgID {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "User does not belong to this org.", nil)
		return nil, false
	}
	return user, true
}

// emitAudit logs a user management audit event; ignores errors (non-fatal).
func (h *UserHandlers) emitAudit(r *http.Request, principal *appmiddleware.Principal, action, resourceID string, meta map[string]string) {
	if h.auditService == nil {
		return
	}
	_, _ = h.auditService.Append(audit.AppendRequest{
		OrgID:        principal.OrgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   principal.Roles,
		Action:       action,
		ResourceType: "keycloak_user",
		ResourceID:   resourceID,
		Metadata:     meta,
	})
}

// ── GET /users ─────────────────────────────────────────────────────────────────

func (h *UserHandlers) listUsers(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	limit := 50
	offset := 0
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if oStr := q.Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	users, err := h.kc.ListUsers(r.Context(), orgID, search, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to list users.", nil)
		return
	}

	// Optional role filter (post-list, applied in-process since Keycloak
	// does not support server-side role filtering on list-users).
	if roleFilter := strings.TrimSpace(q.Get("role")); roleFilter != "" {
		var filtered []keycloak.User
		for _, u := range users {
			roles, err := h.kc.GetUserRealmRoles(r.Context(), u.ID)
			if err != nil {
				continue
			}
			for _, rl := range roles {
				if strings.EqualFold(rl.Name, roleFilter) {
					filtered = append(filtered, u)
					break
				}
			}
		}
		users = filtered
	}

	// Optional status filter.
	if statusFilter := strings.TrimSpace(q.Get("status")); statusFilter != "" {
		var filtered []keycloak.User
		for _, u := range users {
			switch strings.ToLower(statusFilter) {
			case "active":
				if u.Enabled {
					filtered = append(filtered, u)
				}
			case "disabled":
				if !u.Enabled {
					filtered = append(filtered, u)
				}
			case "pending_approval":
				regStatus := ""
				if u.Attributes != nil {
					if vals := u.Attributes["registration_status"]; len(vals) > 0 {
						regStatus = vals[0]
					}
				}
				if regStatus == "pending_approval" {
					filtered = append(filtered, u)
				}
			}
		}
		users = filtered
	}

	if users == nil {
		users = []keycloak.User{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(users),
		"users":  users,
	})
}

// ── GET /users/{id} ────────────────────────────────────────────────────────────

func (h *UserHandlers) getUser(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	user, ok := h.requireUserInOrg(w, r, userID, orgID)
	if !ok {
		return
	}

	roles, err := h.kc.GetUserRealmRoles(r.Context(), userID)
	if err != nil {
		roles = []keycloak.Role{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"user":   user,
		"roles":  roles,
	})
}

// ── POST /register (public) ────────────────────────────────────────────────────

type selfRegisterRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	OrgID     string `json:"org_id"`
}

func (h *UserHandlers) selfRegister(w http.ResponseWriter, r *http.Request) {
	var req selfRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.OrgID = strings.TrimSpace(req.OrgID)

	if req.Email == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "email is required.", nil)
		return
	}
	if req.OrgID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "org_id is required.", nil)
		return
	}

	newUserID, err := h.kc.CreateUser(r.Context(), req.Email, req.FirstName, req.LastName, req.OrgID)
	if err != nil {
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "user_already_exists", "A user with that email already exists.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to create user.", nil)
		}
		return
	}

	// Set registration_status=pending_approval so the admin approvals queue can filter by it.
	// Preserve org_id alongside the new attribute.
	_ = h.kc.UpdateUser(r.Context(), newUserID, keycloak.UpdateUserFields{
		Attributes: map[string][]string{
			"org_id":              {req.OrgID},
			"registration_status": {"pending_approval"},
		},
	})

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        req.OrgID,
			ActorUserID:  newUserID,
			ActorRoles:   []string{},
			Action:       audit.ActionAuthUserSelfRegister,
			ResourceType: "keycloak_user",
			ResourceID:   newUserID,
			Metadata:     map[string]string{"email": req.Email},
		})
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":  "pending_approval",
		"user_id": newUserID,
	})
}

// ── POST /users ────────────────────────────────────────────────────────────────
// Creates a user with a directly-set password (admin-created account, no invite email).

type createUserRequest struct {
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	TemporaryPwd    bool   `json:"temporary_password"`
	CreateHousehold bool   `json:"create_household"`
}

func (h *UserHandlers) createUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req createUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Role = strings.TrimSpace(strings.ToLower(req.Role))
	req.Password = strings.TrimSpace(req.Password)

	if req.Email == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "email is required.", nil)
		return
	}
	if req.Password == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "password is required.", nil)
		return
	}
	if len(req.Password) < 8 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters.", nil)
		return
	}

	// Tier guard.
	if req.Role != "" {
		targetTier := roleTier[req.Role]
		if targetTier == 0 {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "Unknown role: "+req.Role, nil)
			return
		}
		if actorMaxTier(principal.Roles) < targetTier {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You cannot assign a role above your own privilege tier.", nil)
			return
		}
	}

	newUserID, err := h.kc.CreateUser(r.Context(), req.Email, req.FirstName, req.LastName, orgID)
	if err != nil {
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "user_already_exists", "A user with that email already exists.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to create user.", nil)
		}
		return
	}

	// Set the password directly.
	if pwdErr := h.kc.SetUserPassword(r.Context(), newUserID, req.Password, req.TemporaryPwd); pwdErr != nil {
		// Best-effort cleanup: delete the orphaned user.
		_ = h.kc.DeleteUser(r.Context(), newUserID)
		httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to set user password.", nil)
		return
	}

	// Assign the requested role if provided.
	if req.Role != "" {
		if assignErr := h.kc.AssignRealmRole(r.Context(), newUserID, req.Role); assignErr != nil {
			h.emitAudit(r, principal, audit.ActionAuthRoleAssign, newUserID, map[string]string{
				"role":  req.Role,
				"error": assignErr.Error(),
			})
		}
	}

	// Auto-create a linked household if requested.
	var householdID string
	if req.CreateHousehold && h.membershipService != nil {
		householdName := strings.TrimSpace(req.FirstName + " " + req.LastName)
		if householdName == "" {
			householdName = req.Email
		}
		syntheticPrincipal := &appmiddleware.Principal{
			UserID: principal.UserID,
			OrgID:  orgID,
			Roles:  principal.Roles,
		}
		hh, hhErr := h.membershipService.CreateHousehold(syntheticPrincipal, members.CreateHouseholdInput{
			OrgID:          orgID,
			HouseholdName:  householdName,
			PrimaryEmail:   req.Email,
			KeycloakUserID: newUserID,
		})
		if hhErr == nil {
			householdID = hh.ID
		}
	}

	h.emitAudit(r, principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
		"email":        req.Email,
		"role":         req.Role,
		"method":       "direct_create",
		"household_id": householdID,
	})

	resp := map[string]any{
		"status":  "created",
		"user_id": newUserID,
	}
	if householdID != "" {
		resp["household_id"] = householdID
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, resp)
}

// ── POST /users/invite ─────────────────────────────────────────────────────────

type inviteUserRequest struct {
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Role            string `json:"role"`
	CreateHousehold bool   `json:"create_household"`
}

func (h *UserHandlers) inviteUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	var req inviteUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Role = strings.TrimSpace(strings.ToLower(req.Role))

	if req.Email == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "email is required.", nil)
		return
	}

	// Tier guard: actor may only invite users with roles at or below their tier.
	if req.Role != "" {
		targetTier := roleTier[req.Role]
		if targetTier == 0 {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "Unknown role: "+req.Role, nil)
			return
		}
		if actorMaxTier(principal.Roles) < targetTier {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You cannot assign a role above your own privilege tier.", nil)
			return
		}
	}

	newUserID, err := h.kc.CreateUser(r.Context(), req.Email, req.FirstName, req.LastName, orgID)
	if err != nil {
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "user_already_exists", "A user with that email already exists.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to create user.", nil)
		}
		return
	}

	// Assign the requested role if provided.
	if req.Role != "" {
		if assignErr := h.kc.AssignRealmRole(r.Context(), newUserID, req.Role); assignErr != nil {
			// Non-fatal: user is created; role assignment failure is logged.
			h.emitAudit(r, principal, audit.ActionAuthRoleAssign, newUserID, map[string]string{
				"role":  req.Role,
				"error": assignErr.Error(),
			})
		}
	}

	// If requested and a members service is available, auto-create a linked household.
	var householdID string
	if req.CreateHousehold && h.membershipService != nil {
		householdName := strings.TrimSpace(req.FirstName + " " + req.LastName)
		if householdName == "" {
			householdName = req.Email
		}
		syntheticPrincipal := &appmiddleware.Principal{
			UserID: principal.UserID,
			OrgID:  orgID,
			Roles:  principal.Roles,
		}
		hh, hhErr := h.membershipService.CreateHousehold(syntheticPrincipal, members.CreateHouseholdInput{
			OrgID:          orgID,
			HouseholdName:  householdName,
			PrimaryEmail:   req.Email,
			KeycloakUserID: newUserID,
		})
		if hhErr != nil {
			// Non-fatal: user is already created; log the failure and continue.
			h.emitAudit(r, principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
				"email":              req.Email,
				"household_error":    hhErr.Error(),
			})
		} else {
			householdID = hh.ID
		}
	}

	// Send welcome / verification email.
	_ = h.kc.SendVerificationEmail(r.Context(), newUserID)

	h.emitAudit(r, principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
		"email":        req.Email,
		"role":         req.Role,
		"household_id": householdID,
	})

	resp := map[string]any{
		"status":  "created",
		"user_id": newUserID,
	}
	if householdID != "" {
		resp["household_id"] = householdID
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, resp)
}

// ── PUT /users/{id} ────────────────────────────────────────────────────────────

type updateUserRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

func (h *UserHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	var req updateUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}

	fields := keycloak.UpdateUserFields{
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		Email:     strings.TrimSpace(strings.ToLower(req.Email)),
	}

	if err := h.kc.UpdateUser(r.Context(), userID, fields); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to update user.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthUserUpdate, userID, map[string]string{
		"email":      req.Email,
		"first_name": req.FirstName,
		"last_name":  req.LastName,
	})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

// ── POST /users/{id}/disable ───────────────────────────────────────────────────

func (h *UserHandlers) disableUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.DisableUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to disable user.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthUserDisable, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
}

// ── POST /users/{id}/enable ────────────────────────────────────────────────────

func (h *UserHandlers) enableUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.EnableUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to enable user.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthUserEnable, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "enabled"})
}

// ── POST /users/{id}/approve ───────────────────────────────────────────────────

func (h *UserHandlers) approveUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	user, ok := h.requireUserInOrg(w, r, userID, orgID)
	if !ok {
		return
	}

	if err := h.kc.EnableUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to enable user.", nil)
		}
		return
	}

	// Update registration_status to "active", preserving org_id.
	_ = h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{
		Attributes: map[string][]string{
			"org_id":              {user.OrgID()},
			"registration_status": {"active"},
		},
	})

	// Assign the parent role if the user has no realm roles yet.
	existingRoles, _ := h.kc.GetUserRealmRoles(r.Context(), userID)
	if len(existingRoles) == 0 {
		_ = h.kc.AssignRealmRole(r.Context(), userID, "parent")
	}

	// Trigger welcome / verification email.
	_ = h.kc.SendVerificationEmail(r.Context(), userID)

	h.emitAudit(r, principal, audit.ActionAuthUserApprove, userID, map[string]string{
		"email": user.Email,
	})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "approved"})
}

// ── POST /users/{id}/reject ────────────────────────────────────────────────────

type rejectUserRequest struct {
	Reason string `json:"reason"`
}

func (h *UserHandlers) rejectUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	user, ok := h.requireUserInOrg(w, r, userID, orgID)
	if !ok {
		return
	}

	var req rejectUserRequest
	// Ignore decode error — reason is optional and body may be empty.
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.Reason = strings.TrimSpace(req.Reason)

	if err := h.kc.DisableUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to disable user.", nil)
		}
		return
	}

	// Update registration_status to "rejected", preserving org_id.
	_ = h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{
		Attributes: map[string][]string{
			"org_id":              {user.OrgID()},
			"registration_status": {"rejected"},
		},
	})

	meta := map[string]string{"email": user.Email}
	if req.Reason != "" {
		meta["reason"] = req.Reason
	}
	h.emitAudit(r, principal, audit.ActionAuthUserReject, userID, meta)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

// ── DELETE /users/{id} ─────────────────────────────────────────────────────────

func (h *UserHandlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	// Delete requires MFA.
	if !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA authentication is required to delete users.", nil)
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to delete user.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthUserDelete, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

// ── GET /users/{id}/roles ──────────────────────────────────────────────────────

func (h *UserHandlers) listUserRoles(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	roles, err := h.kc.GetUserRealmRoles(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to list roles.", nil)
		return
	}
	if roles == nil {
		roles = []keycloak.Role{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"roles":  roles,
	})
}

// ── POST /users/{id}/roles ─────────────────────────────────────────────────────

type userAssignRoleRequest struct {
	Role string `json:"role"`
}

func (h *UserHandlers) assignRole(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	var req userAssignRoleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Role = strings.TrimSpace(strings.ToLower(req.Role))

	if req.Role == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "role is required.", nil)
		return
	}

	targetTier := roleTier[req.Role]
	if targetTier == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "Unknown role: "+req.Role, nil)
		return
	}
	if actorMaxTier(principal.Roles) < targetTier {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You cannot assign a role above your own privilege tier.", nil)
		return
	}

	if err := h.kc.AssignRealmRole(r.Context(), userID, req.Role); err != nil {
		if errors.Is(err, keycloak.ErrRoleNotFound) {
			httpx.WriteError(w, r, http.StatusBadRequest, "role_not_found", "Role does not exist in the realm.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to assign role.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthRoleAssign, userID, map[string]string{"role": req.Role})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "assigned"})
}

// ── DELETE /users/{id}/roles/{role} ────────────────────────────────────────────

func (h *UserHandlers) removeRole(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	roleName := strings.TrimSpace(strings.ToLower(chi.URLParam(r, "role")))

	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	targetTier := roleTier[roleName]
	if targetTier == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "Unknown role: "+roleName, nil)
		return
	}
	if actorMaxTier(principal.Roles) < targetTier {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You cannot remove a role above your own privilege tier.", nil)
		return
	}

	// Last-owner guard: prevent removal of the final owner role in the org.
	if roleName == "owner" {
		owners, err := h.kc.ListUsers(r.Context(), orgID, "", 500, 0)
		if err == nil {
			ownerCount := 0
			for _, u := range owners {
				roles, rerr := h.kc.GetUserRealmRoles(r.Context(), u.ID)
				if rerr != nil {
					continue
				}
				if slices.ContainsFunc(roles, func(rl keycloak.Role) bool {
					return strings.EqualFold(rl.Name, "owner")
				}) {
					ownerCount++
				}
			}
			if ownerCount <= 1 {
				httpx.WriteError(w, r, http.StatusConflict, "last_owner", "Cannot remove the last owner role assignment.", nil)
				return
			}
		}
	}

	if err := h.kc.RemoveRealmRole(r.Context(), userID, roleName); err != nil {
		if errors.Is(err, keycloak.ErrRoleNotFound) {
			httpx.WriteError(w, r, http.StatusBadRequest, "role_not_found", "Role does not exist.", nil)
		} else if errors.Is(err, keycloak.ErrLastOwner) {
			httpx.WriteError(w, r, http.StatusConflict, "last_owner", "Cannot remove the last owner role assignment.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to remove role.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthRoleRemove, userID, map[string]string{"role": roleName})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "removed"})
}

// ── POST /users/{id}/password-reset ────────────────────────────────────────────

func (h *UserHandlers) passwordReset(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.SendPasswordResetEmail(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", "Failed to send password reset email.", nil)
		}
		return
	}

	h.emitAudit(r, principal, audit.ActionAuthUserPasswordReset, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "password_reset_sent"})
}

// ── GET /users/{id}/household ──────────────────────────────────────────────────

func (h *UserHandlers) getUserHousehold(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	household, found := h.membershipService.GetHouseholdByKeycloakUserID(userID)
	if !found {
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"household": nil,
		})
		return
	}

	// Ensure the household belongs to the same org.
	if household.OrgID != orgID {
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"household": nil,
		})
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"household": household,
	})
}

// ── POST /users/{id}/household/link ───────────────────────────────────────────

type linkHouseholdRequest struct {
	HouseholdID string `json:"household_id"`
}

func (h *UserHandlers) linkUserHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	var req linkHouseholdRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.HouseholdID = strings.TrimSpace(req.HouseholdID)
	if req.HouseholdID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "household_id is required.", nil)
		return
	}

	if err := h.membershipService.LinkHouseholdToUser(orgID, req.HouseholdID, userID); err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusConflict, "link_conflict", err.Error(), nil)
		}
		return
	}

	h.emitAudit(r, principal, "user.household.link", userID, map[string]string{
		"household_id": req.HouseholdID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "linked",
		"household_id": req.HouseholdID,
	})
}

// ── POST /users/{id}/household/unlink ─────────────────────────────────────────

type unlinkHouseholdRequest struct {
	HouseholdID string `json:"household_id"`
}

func (h *UserHandlers) unlinkUserHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	var req unlinkHouseholdRequest
	if err := decodeJSONBody(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.HouseholdID = strings.TrimSpace(req.HouseholdID)
	if req.HouseholdID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "household_id is required.", nil)
		return
	}

	if err := h.membershipService.UnlinkHouseholdFromUser(orgID, req.HouseholdID); err != nil {
		if errors.Is(err, members.ErrHouseholdNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "service_error", "Failed to unlink household.", nil)
		}
		return
	}

	h.emitAudit(r, principal, "user.household.unlink", userID, map[string]string{
		"household_id": req.HouseholdID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "unlinked",
		"household_id": req.HouseholdID,
	})
}
