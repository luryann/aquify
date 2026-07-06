package handlers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/go-chi/chi/v5"
)

// roleTier maps each role name to its numeric tier (higher = more privileged).
// An actor may only assign/remove roles at or below their own tier.
var roleTier = map[string]int{
	"admin":   3,
	"coach":   2,
	"parent":  1,
	"swimmer": 1,
}

// actorMaxTier returns the highest tier among the actor's roles.
func actorMaxTier(roles []string) int {
	max := 0
	for _, r := range roles {
		if t := roleTier[normalizeUserRole(r)]; t > max {
			max = t
		}
	}
	return max
}

// countOrgAdmins returns how many users in orgID currently hold the admin
// realm role. Errors listing users/roles are treated as zero found for that
// user, matching the fail-open behavior already used by the role-removal
// guard (a listing failure should not itself block a legitimate action).
func (h *UserHandlers) countOrgAdmins(ctx context.Context, orgID string) (int, error) {
	users, err := h.kc.ListUsers(ctx, orgID, "", 500, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, u := range users {
		roles, rerr := h.kc.GetUserRealmRoles(ctx, u.ID)
		if rerr != nil {
			continue
		}
		if slices.ContainsFunc(roles, func(rl keycloak.Role) bool {
			return strings.EqualFold(rl.Name, "admin")
		}) {
			count++
		}
	}
	return count, nil
}

// orgHasOnlyOneAdmin reports whether orgID currently has at most one admin.
// A listing failure fails open (returns false) so it doesn't itself block a
// legitimate role-removal or delete request.
func (h *UserHandlers) orgHasOnlyOneAdmin(ctx context.Context, orgID string) bool {
	count, err := h.countOrgAdmins(ctx, orgID)
	return err == nil && count <= 1
}

// userIsOrgAdmin reports whether userID currently holds the admin realm role.
func (h *UserHandlers) userIsOrgAdmin(ctx context.Context, userID string) bool {
	roles, err := h.kc.GetUserRealmRoles(ctx, userID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(roles, func(rl keycloak.Role) bool {
		return strings.EqualFold(rl.Name, "admin")
	})
}

func normalizeUserRole(role string) string {
	return strings.TrimSpace(strings.ToLower(role))
}

func normalizeRealmRoles(roles []keycloak.Role) []string {
	out := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		name := normalizeUserRole(role.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func normalizeRealmRoleObjects(roles []keycloak.Role) []keycloak.Role {
	out := make([]keycloak.Role, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role.Name = normalizeUserRole(role.Name)
		if role.Name == "" {
			continue
		}
		if _, ok := seen[role.Name]; ok {
			continue
		}
		seen[role.Name] = struct{}{}
		out = append(out, role)
	}
	slices.SortFunc(out, func(a, b keycloak.Role) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})
	return out
}

type userListItem struct {
	ID            string              `json:"id"`
	Username      string              `json:"username"`
	Email         string              `json:"email"`
	FirstName     string              `json:"firstName"`
	LastName      string              `json:"lastName"`
	Enabled       bool                `json:"enabled"`
	EmailVerified bool                `json:"emailVerified"`
	Attributes    map[string][]string `json:"attributes,omitempty"`
	Roles         []string            `json:"roles,omitempty"`
}

func buildUserListItem(user keycloak.User, roles []string) userListItem {
	return userListItem{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		Enabled:       user.Enabled,
		EmailVerified: user.EmailVerified,
		Attributes:    user.Attributes,
		Roles:         roles,
	}
}

func keycloakSource(client keycloak.Client) string {
	if _, ok := client.(*keycloak.StubClient); ok {
		return "stub"
	}
	return "keycloak"
}

// UserHandlers implements HTTP handlers for Keycloak-backed user management.
type UserHandlers struct {
	kc                keycloak.Client
	auditService      *audit.Service
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
		// admin only
		users.With(appmiddleware.RBACStub("admin")).Get("/", h.listUsers)
		users.With(appmiddleware.RBACStub("admin")).Post("/", h.createUser)
		users.With(appmiddleware.RBACStub("admin")).Post("/invite", h.inviteUser)
		users.With(appmiddleware.RBACStub("admin")).Get("/{id}", h.getUser)
		users.With(appmiddleware.RBACStub("admin")).Put("/{id}", h.updateUser)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/disable", h.disableUser)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/enable", h.enableUser)
		users.With(appmiddleware.RBACStub("admin")).Get("/{id}/roles", h.listUserRoles)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/roles", h.assignRole)
		users.With(appmiddleware.RBACStub("admin")).Delete("/{id}/roles/{role}", h.removeRole)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/password-reset", h.passwordReset)
		// approval flow (admin only)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/approve", h.approveUser)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/reject", h.rejectUser)
		// household linkage (admin only)
		users.With(appmiddleware.RBACStub("admin")).Get("/{id}/household", h.getUserHousehold)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/household/link", h.linkUserHousehold)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/household/unlink", h.unlinkUserHousehold)
		// swimmer linkage (admin only) — links a swimmer-role user's Keycloak
		// account to a specific swimmer record, so their portal session resolves
		// deterministically instead of guessing from the household roster.
		users.With(appmiddleware.RBACStub("admin")).Get("/{id}/swimmer", h.getUserSwimmer)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/swimmer/link", h.linkUserSwimmer)
		users.With(appmiddleware.RBACStub("admin")).Post("/{id}/swimmer/unlink", h.unlinkUserSwimmer)
		// self-service swimmer-link recovery: any authenticated user may submit a
		// request for their own account; only admin may view/resolve the queue.
		users.Post("/me/swimmer-link-requests", h.createSwimmerLinkRequest)
		users.With(appmiddleware.RBACStub("admin")).Get("/swimmer-link-requests", h.listSwimmerLinkRequests)
		users.With(appmiddleware.RBACStub("admin")).Post("/swimmer-link-requests/{requestID}/resolve", h.resolveSwimmerLinkRequest)
		// admin only (destructive)
		users.With(appmiddleware.RBACStub("admin")).Delete("/{id}", h.deleteUser)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// userScopeFromRequest extracts the principal and orgID, writes a 401/400 if
// missing, and returns (principal, orgID, true) on success.
func userScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

func writeKeycloakAdminError(w http.ResponseWriter, r *http.Request, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, keycloak.ErrInvalidConfig):
		httpx.WriteError(
			w,
			r,
			http.StatusServiceUnavailable,
			"keycloak_unavailable",
			"Keycloak admin client is not configured. Set KEYCLOAK_ADMIN_URL, KEYCLOAK_REALM, KEYCLOAK_ADMIN_CLIENT_ID, and KEYCLOAK_ADMIN_CLIENT_SECRET.",
			nil,
		)
	case errors.Is(err, keycloak.ErrUnauthorized):
		httpx.WriteError(
			w,
			r,
			http.StatusServiceUnavailable,
			"keycloak_unavailable",
			"Keycloak admin client could not authenticate. Verify KEYCLOAK_ADMIN_CLIENT_ID, KEYCLOAK_ADMIN_CLIENT_SECRET, and the service account's realm-management roles.",
			nil,
		)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", fallbackMessage, nil)
	}
}

func cloneUserAttributes(user *keycloak.User) map[string][]string {
	attrs := make(map[string][]string)
	if user == nil || user.Attributes == nil {
		return attrs
	}
	for key, values := range user.Attributes {
		cp := make([]string, len(values))
		copy(cp, values)
		attrs[key] = cp
	}
	return attrs
}

func setUserAttribute(attrs map[string][]string, key, value string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		delete(attrs, key)
		return
	}
	attrs[key] = []string{trimmed}
}

func firstUserAttribute(user *keycloak.User, key string) string {
	if user == nil || user.Attributes == nil {
		return ""
	}
	values := user.Attributes[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

// requireUserInOrg fetches the target user and verifies org membership.
// Writes 404 or 403 on failure. Returns (user, true) on success.
func (h *UserHandlers) requireUserInOrg(w http.ResponseWriter, r *http.Request, userID, orgID string) (*keycloak.User, bool) {
	user, err := h.kc.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to fetch user.")
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
	pagination := parseListPagination(r, listPaginationOptions{DefaultLimit: 50})

	users, err := h.kc.ListUsers(r.Context(), orgID, search, pagination.Limit, pagination.Offset)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to list users.")
		return
	}

	roleFilter := normalizeUserRole(q.Get("role"))

	listItems := make([]userListItem, 0, len(users))
	for _, u := range users {
		realmRoles, err := h.kc.GetUserRealmRoles(r.Context(), u.ID)
		if err != nil {
			realmRoles = []keycloak.Role{}
		}
		normalizedRoles := normalizeRealmRoles(realmRoles)
		if roleFilter != "" && !slices.Contains(normalizedRoles, roleFilter) {
			continue
		}
		listItems = append(listItems, buildUserListItem(u, normalizedRoles))
	}

	// Optional status filter.
	if statusFilter := strings.TrimSpace(q.Get("status")); statusFilter != "" {
		var filtered []userListItem
		for _, u := range listItems {
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
		listItems = filtered
	}

	if listItems == nil {
		listItems = []userListItem{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"count":      len(listItems),
		"limit":      pagination.Limit,
		"offset":     pagination.Offset,
		"source":     keycloakSource(h.kc),
		"is_stubbed": keycloakSource(h.kc) == "stub",
		"users":      listItems,
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
		"roles":  normalizeRealmRoleObjects(roles),
	})
}

// ── POST /register (public) ────────────────────────────────────────────────────

type selfRegisterRequest struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Email         string `json:"email"`
	OrgID         string `json:"org_id"`
	RequestedRole string `json:"requested_role"`
	ClubCode      string `json:"club_code"`
	Note          string `json:"note"`
	SwimmerName   string `json:"swimmer_name"`
}

func (h *UserHandlers) selfRegister(w http.ResponseWriter, r *http.Request) {
	var req selfRegisterRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.OrgID = strings.TrimSpace(req.OrgID)
	req.RequestedRole = normalizeUserRole(req.RequestedRole)
	req.ClubCode = strings.ToUpper(strings.TrimSpace(req.ClubCode))
	req.Note = strings.TrimSpace(req.Note)
	req.SwimmerName = strings.TrimSpace(req.SwimmerName)

	if req.Email == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "email is required.", nil)
		return
	}
	if req.OrgID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "org_id is required.", nil)
		return
	}
	if req.RequestedRole != "" {
		if _, ok := roleTier[req.RequestedRole]; !ok {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "requested_role is not supported.", nil)
			return
		}
	}

	newUserID, err := h.kc.CreateUser(r.Context(), req.Email, req.FirstName, req.LastName, req.OrgID)
	if err != nil {
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "user_already_exists", "A user with that email already exists.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to create user.")
		}
		return
	}

	registrationAttributes := map[string][]string{
		"org_id":                    {req.OrgID},
		"registration_status":       {"pending_approval"},
		"registration_submitted_at": {time.Now().UTC().Format(time.RFC3339)},
	}
	if req.RequestedRole != "" {
		registrationAttributes["requested_role"] = []string{req.RequestedRole}
	}
	if req.ClubCode != "" {
		registrationAttributes["registration_club_code"] = []string{req.ClubCode}
	}
	if req.Note != "" {
		registrationAttributes["registration_note"] = []string{req.Note}
	}
	if req.SwimmerName != "" {
		registrationAttributes["registration_swimmer_name"] = []string{req.SwimmerName}
	}

	_ = h.kc.UpdateUser(r.Context(), newUserID, keycloak.UpdateUserFields{Attributes: registrationAttributes})

	if h.auditService != nil {
		metadata := map[string]string{"email": req.Email}
		if req.RequestedRole != "" {
			metadata["requested_role"] = req.RequestedRole
		}
		if req.ClubCode != "" {
			metadata["club_code"] = req.ClubCode
		}
		if req.SwimmerName != "" {
			metadata["swimmer_name"] = req.SwimmerName
		}
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        req.OrgID,
			ActorUserID:  newUserID,
			ActorRoles:   []string{},
			Action:       audit.ActionAuthUserSelfRegister,
			ResourceType: "keycloak_user",
			ResourceID:   newUserID,
			Metadata:     metadata,
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
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Role = normalizeUserRole(req.Role)
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
			writeKeycloakAdminError(w, r, err, "Failed to create user.")
		}
		return
	}

	// Set the password directly.
	if pwdErr := h.kc.SetUserPassword(r.Context(), newUserID, req.Password, req.TemporaryPwd); pwdErr != nil {
		// Best-effort cleanup: delete the orphaned user.
		_ = h.kc.DeleteUser(r.Context(), newUserID)
		writeKeycloakAdminError(w, r, pwdErr, "Failed to set user password.")
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
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Role = normalizeUserRole(req.Role)

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
			writeKeycloakAdminError(w, r, err, "Failed to create user.")
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
				"email":           req.Email,
				"household_error": hhErr.Error(),
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
	if err := decodeJSONBody(w, r, &req); err != nil {
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
			writeKeycloakAdminError(w, r, err, "Failed to update user.")
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
			writeKeycloakAdminError(w, r, err, "Failed to disable user.")
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
			writeKeycloakAdminError(w, r, err, "Failed to enable user.")
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
			writeKeycloakAdminError(w, r, err, "Failed to enable user.")
		}
		return
	}

	attrs := cloneUserAttributes(user)
	setUserAttribute(attrs, "org_id", user.OrgID())
	setUserAttribute(attrs, "registration_status", "active")
	_ = h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{Attributes: attrs})

	requestedRole := normalizeUserRole(firstUserAttribute(user, "requested_role"))
	if requestedRole == "" {
		requestedRole = "parent"
	}

	// Approval must not let an approver grant a role above their own tier —
	// the same guard applied to direct create/invite/assign-role paths.
	// Self-registration can request any role, so this is the only gate that
	// prevents a lower-tier approver from indirectly granting a role above
	// their own.
	targetTier := roleTier[requestedRole]
	if actorMaxTier(principal.Roles) < targetTier {
		httpx.WriteError(w, r, http.StatusForbidden, "insufficient_role_tier", "You cannot approve a user for a role above your own.", nil)
		return
	}

	// Assign the requested role if the user has no realm roles yet.
	existingRoles, _ := h.kc.GetUserRealmRoles(r.Context(), userID)
	if len(existingRoles) == 0 {
		_ = h.kc.AssignRealmRole(r.Context(), userID, requestedRole)
	}

	// Trigger welcome / verification email.
	_ = h.kc.SendVerificationEmail(r.Context(), userID)

	h.emitAudit(r, principal, audit.ActionAuthUserApprove, userID, map[string]string{
		"email":          user.Email,
		"requested_role": requestedRole,
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
	_ = decodeOptionalJSONBody(w, r, &req)
	req.Reason = strings.TrimSpace(req.Reason)

	if err := h.kc.DisableUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to disable user.")
		}
		return
	}

	attrs := cloneUserAttributes(user)
	setUserAttribute(attrs, "org_id", user.OrgID())
	setUserAttribute(attrs, "registration_status", "rejected")
	_ = h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{Attributes: attrs})

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

	// Last-admin guard: deleting the target must not leave the org with zero
	// admins, mirroring the guard on admin role removal.
	if h.userIsOrgAdmin(r.Context(), userID) && h.orgHasOnlyOneAdmin(r.Context(), orgID) {
		httpx.WriteError(w, r, http.StatusConflict, "last_admin", "Cannot delete the last admin in this org.", nil)
		return
	}

	if err := h.kc.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, keycloak.ErrUserNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to delete user.")
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
		writeKeycloakAdminError(w, r, err, "Failed to list roles.")
		return
	}
	if roles == nil {
		roles = []keycloak.Role{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"roles":  normalizeRealmRoleObjects(roles),
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
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.Role = normalizeUserRole(req.Role)

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
			writeKeycloakAdminError(w, r, err, "Failed to assign role.")
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
	roleName := normalizeUserRole(chi.URLParam(r, "role"))

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

	// Last-admin guard: prevent removal of the final admin role in the org.
	if roleName == "admin" && h.orgHasOnlyOneAdmin(r.Context(), orgID) {
		httpx.WriteError(w, r, http.StatusConflict, "last_admin", "Cannot remove the last admin role assignment.", nil)
		return
	}

	if err := h.kc.RemoveRealmRole(r.Context(), userID, roleName); err != nil {
		if errors.Is(err, keycloak.ErrRoleNotFound) {
			httpx.WriteError(w, r, http.StatusBadRequest, "role_not_found", "Role does not exist.", nil)
		} else if errors.Is(err, keycloak.ErrLastAdmin) {
			httpx.WriteError(w, r, http.StatusConflict, "last_admin", "Cannot remove the last admin role assignment.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to remove role.")
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
			writeKeycloakAdminError(w, r, err, "Failed to send password reset email.")
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
	if err := decodeJSONBody(w, r, &req); err != nil {
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
	if err := decodeJSONBody(w, r, &req); err != nil {
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

// ── GET /users/{id}/swimmer ────────────────────────────────────────────────

func (h *UserHandlers) getUserSwimmer(w http.ResponseWriter, r *http.Request) {
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

	swimmer, found := h.membershipService.GetSwimmerByKeycloakUserID(userID)
	if !found || swimmer.OrgID != orgID {
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"swimmer": nil,
		})
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"swimmer": swimmer,
	})
}

// ── POST /users/{id}/swimmer/link ─────────────────────────────────────────

type linkSwimmerRequest struct {
	SwimmerID string `json:"swimmer_id"`
}

func (h *UserHandlers) linkUserSwimmer(w http.ResponseWriter, r *http.Request) {
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

	var req linkSwimmerRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.SwimmerID = strings.TrimSpace(req.SwimmerID)
	if req.SwimmerID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "swimmer_id is required.", nil)
		return
	}

	if err := h.membershipService.LinkSwimmerToUser(orgID, req.SwimmerID, userID); err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusConflict, "link_conflict", err.Error(), nil)
		}
		return
	}

	h.emitAudit(r, principal, "user.swimmer.link", userID, map[string]string{
		"swimmer_id": req.SwimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "linked",
		"swimmer_id": req.SwimmerID,
	})
}

// ── POST /users/{id}/swimmer/unlink ───────────────────────────────────────

type unlinkSwimmerRequest struct {
	SwimmerID string `json:"swimmer_id"`
}

func (h *UserHandlers) unlinkUserSwimmer(w http.ResponseWriter, r *http.Request) {
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

	var req unlinkSwimmerRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	req.SwimmerID = strings.TrimSpace(req.SwimmerID)
	if req.SwimmerID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "swimmer_id is required.", nil)
		return
	}

	if err := h.membershipService.UnlinkSwimmerFromUser(orgID, req.SwimmerID); err != nil {
		if errors.Is(err, members.ErrSwimmerNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "service_error", "Failed to unlink swimmer.", nil)
		}
		return
	}

	h.emitAudit(r, principal, "user.swimmer.unlink", userID, map[string]string{
		"swimmer_id": req.SwimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "unlinked",
		"swimmer_id": req.SwimmerID,
	})
}

// ── POST /users/me/swimmer-link-requests ──────────────────────────────────

type createSwimmerLinkRequestBody struct {
	RequestedSwimmerName string `json:"requested_swimmer_name"`
	Note                 string `json:"note"`
}

func (h *UserHandlers) createSwimmerLinkRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	var body createSwimmerLinkRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}

	req, err := h.membershipService.CreateLinkRequest(orgID, principal.UserID, body.RequestedSwimmerName, body.Note)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create link request.", err)
		return
	}

	h.emitAudit(r, principal, "user.swimmer_link_request.create", principal.UserID, map[string]string{
		"request_id": req.ID,
	})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"request": req})
}

// ── GET /users/swimmer-link-requests ───────────────────────────────────────

func (h *UserHandlers) listSwimmerLinkRequests(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	requests, err := h.membershipService.ListPendingLinkRequests(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list link requests.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": requests})
}

// ── POST /users/swimmer-link-requests/{requestID}/resolve ─────────────────

type resolveSwimmerLinkRequestBody struct {
	SwimmerID string `json:"swimmer_id"`
}

func (h *UserHandlers) resolveSwimmerLinkRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := userScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return
	}

	requestID := chi.URLParam(r, "requestID")
	var body resolveSwimmerLinkRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}
	body.SwimmerID = strings.TrimSpace(body.SwimmerID)
	if body.SwimmerID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "swimmer_id is required.", nil)
		return
	}

	requests, err := h.membershipService.ListPendingLinkRequests(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load link request.", err)
		return
	}
	var target *members.SwimmerLinkRequest
	for i := range requests {
		if requests[i].ID == requestID {
			target = &requests[i]
			break
		}
	}
	if target == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "request_not_found", "Link request not found.", nil)
		return
	}

	if err := h.membershipService.LinkSwimmerToUser(orgID, body.SwimmerID, target.UserID); err != nil {
		if errors.Is(err, members.ErrSwimmerNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusConflict, "link_conflict", err.Error(), nil)
		}
		return
	}
	if err := h.membershipService.ResolveLinkRequest(orgID, requestID, principal.UserID, body.SwimmerID); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve link request.", err)
		return
	}

	h.emitAudit(r, principal, "user.swimmer_link_request.resolve", target.UserID, map[string]string{
		"request_id": requestID,
		"swimmer_id": body.SwimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "resolved",
		"swimmer_id": body.SwimmerID,
	})
}
