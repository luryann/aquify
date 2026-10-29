package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// requireAssignableRole writes a 400/403 unless role exists and sits at or below the actor's tier.
func requireAssignableRole(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, role, verb string) bool {
	targetTier := roleTier[role]
	if targetTier == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", "Unknown role: "+role, nil)
		return false
	}
	if actorMaxTier(principal.Roles) < targetTier {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You cannot "+verb+" a role above your own privilege tier.", nil)
		return false
	}
	return true
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
// realm role, using a single "members of the admin role" query instead of one
// role lookup per user. Any lookup failure is returned: a partial count would
// undercount admins and let the last admin be removed.
func (h *UserHandlers) countOrgAdmins(ctx context.Context, orgID string) (int, error) {
	admins, err := h.kc.ListUsersWithRealmRole(ctx, orgID, "admin")
	if err != nil {
		return 0, fmt.Errorf("list admin role members: %w", err)
	}
	return len(admins), nil
}

// orgHasOnlyOneAdmin reports whether orgID currently has at most one admin.
// A lookup failure is returned (never folded into the answer) so callers can
// refuse the destructive action with a clear "could not verify" response.
func (h *UserHandlers) orgHasOnlyOneAdmin(ctx context.Context, orgID string) (bool, error) {
	count, err := h.countOrgAdmins(ctx, orgID)
	if err != nil {
		return false, err
	}
	return count <= 1, nil
}

// userIsOrgAdmin reports whether userID currently holds the admin realm role.
func (h *UserHandlers) userIsOrgAdmin(ctx context.Context, userID string) (bool, error) {
	roles, err := h.kc.GetUserRealmRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	return hasAdminRealmRole(roles), nil
}

func hasAdminRealmRole(roles []keycloak.Role) bool {
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
	slices.SortFunc(out, func(a, b keycloak.Role) int { return strings.Compare(a.Name, b.Name) })
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

// userMatchesStatus applies the /users ?status= filter; unknown values match nothing.
func userMatchesStatus(u userListItem, status string) bool {
	switch status {
	case "active":
		return u.Enabled
	case "disabled":
		return !u.Enabled
	case "pending_approval":
		vals := u.Attributes["registration_status"]
		return len(vals) > 0 && vals[0] == "pending_approval"
	}
	return false
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
		admin := users.With(appmiddleware.RBACStub("admin"))
		// admin only
		admin.Get("/", h.listUsers)
		// staff directory (admin + coach): names only, for coach pickers and
		// resolving coach_user_id labels without exposing the full user list.
		users.With(appmiddleware.RBACStub("admin", "coach")).Get("/staff", h.listStaff)
		admin.Post("/", h.createUser)
		admin.Post("/invite", h.inviteUser)
		admin.Get("/{id}", h.getUser)
		admin.Put("/{id}", h.updateUser)
		admin.Post("/{id}/disable", h.disableUser)
		admin.Post("/{id}/enable", h.enableUser)
		admin.Get("/{id}/roles", h.listUserRoles)
		admin.Post("/{id}/roles", h.assignRole)
		admin.Delete("/{id}/roles/{role}", h.removeRole)
		admin.Post("/{id}/password-reset", h.passwordReset)
		// approval flow (admin only)
		admin.Post("/{id}/approve", h.approveUser)
		admin.Post("/{id}/reject", h.rejectUser)
		// household linkage (admin only)
		admin.Get("/{id}/household", h.getUserHousehold)
		admin.Post("/{id}/household/link", h.linkUserHousehold)
		admin.Post("/{id}/household/unlink", h.unlinkUserHousehold)
		// swimmer linkage (admin only) — links a swimmer-role user's Keycloak
		// account to a specific swimmer record, so their portal session resolves
		// deterministically instead of guessing from the household roster.
		admin.Get("/{id}/swimmer", h.getUserSwimmer)
		admin.Post("/{id}/swimmer/link", h.linkUserSwimmer)
		admin.Post("/{id}/swimmer/unlink", h.unlinkUserSwimmer)
		// self-service swimmer-link recovery: any authenticated user may submit a
		// request for their own account; only admin may view/resolve the queue.
		users.Post("/me/swimmer-link-requests", h.createSwimmerLinkRequest)
		admin.Get("/swimmer-link-requests", h.listSwimmerLinkRequests)
		admin.Post("/swimmer-link-requests/{requestID}/resolve", h.resolveSwimmerLinkRequest)
		// admin only (destructive)
		admin.Delete("/{id}", h.deleteUser)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
		httpx.WriteError(w, r, http.StatusInternalServerError, "keycloak_error", fallbackMessage, err)
	}
}

// writeKeycloakUserError answers 404 for a missing user, else the admin-client error.
func writeKeycloakUserError(w http.ResponseWriter, r *http.Request, err error, fallbackMessage string) {
	if errors.Is(err, keycloak.ErrUserNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		return
	}
	writeKeycloakAdminError(w, r, err, fallbackMessage)
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

// createKeycloakUser creates the account, writing 409/5xx responses itself on failure.
func (h *UserHandlers) createKeycloakUser(w http.ResponseWriter, r *http.Request, email, firstName, lastName, orgID string) (string, bool) {
	userID, err := h.kc.CreateUser(r.Context(), email, firstName, lastName, orgID)
	if err != nil {
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "user_already_exists", "A user with that email already exists.", nil)
		} else {
			writeKeycloakAdminError(w, r, err, "Failed to create user.")
		}
		return "", false
	}
	return userID, true
}

// createLinkedHousehold creates a household named after the user and linked to their Keycloak account.
func (h *UserHandlers) createLinkedHousehold(principal *appmiddleware.Principal, orgID, firstName, lastName, email, keycloakUserID string) (members.Household, error) {
	householdName := strings.TrimSpace(firstName + " " + lastName)
	if householdName == "" {
		householdName = email
	}
	actor := &appmiddleware.Principal{UserID: principal.UserID, OrgID: orgID, Roles: principal.Roles}
	return h.membershipService.CreateHousehold(actor, members.CreateHouseholdInput{
		OrgID:          orgID,
		HouseholdName:  householdName,
		PrimaryEmail:   email,
		KeycloakUserID: keycloakUserID,
	})
}

// assignInitialRole grants role to a just-created user. Failure is non-fatal
// (the account exists) and is recorded as an audit event instead.
func (h *UserHandlers) assignInitialRole(ctx context.Context, principal *appmiddleware.Principal, userID, role string) {
	if role == "" {
		return
	}
	if err := h.kc.AssignRealmRole(ctx, userID, role); err != nil {
		h.emitAudit(principal, audit.ActionAuthRoleAssign, userID, map[string]string{
			"role":  role,
			"error": err.Error(),
		})
	}
}

func writeUserCreated(w http.ResponseWriter, userID, householdID string) {
	resp := map[string]any{"status": "created", "user_id": userID}
	if householdID != "" {
		resp["household_id"] = householdID
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, resp)
}

// requireUserInOrg fetches the target user and verifies org membership.
// Writes 404 or 403 on failure. Returns (user, true) on success.
func (h *UserHandlers) requireUserInOrg(w http.ResponseWriter, r *http.Request, userID, orgID string) (*keycloak.User, bool) {
	user, err := h.kc.GetUser(r.Context(), userID)
	if err != nil {
		writeKeycloakUserError(w, r, err, "Failed to fetch user.")
		return nil, false
	}
	if user.OrgID() != orgID {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "User does not belong to this org.", nil)
		return nil, false
	}
	return user, true
}

// emitAudit logs a user management audit event; ignores errors (non-fatal).
func (h *UserHandlers) emitAudit(principal *appmiddleware.Principal, action, resourceID string, meta map[string]string) {
	recordAudit(h.auditService, principal, principal.OrgID, action, "keycloak_user", resourceID, nil, nil, meta)
}

// ── GET /users ─────────────────────────────────────────────────────────────────

func (h *UserHandlers) listUsers(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
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

	source := keycloakSource(h.kc)
	listItems := make([]userListItem, 0, len(users))
	for _, u := range users {
		realmRoles, err := h.kc.GetUserRealmRoles(r.Context(), u.ID)
		if err != nil {
			// Do not render a lookup failure as "no roles": the role filter
			// would silently drop the user from results.
			writeKeycloakAdminError(w, r, err, "Failed to load user roles.")
			return
		}
		normalizedRoles := normalizeRealmRoles(realmRoles)
		if roleFilter != "" && !slices.Contains(normalizedRoles, roleFilter) {
			continue
		}
		listItems = append(listItems, buildUserListItem(u, normalizedRoles))
	}

	if statusFilter := strings.ToLower(strings.TrimSpace(q.Get("status"))); statusFilter != "" {
		listItems = slices.DeleteFunc(listItems, func(u userListItem) bool { return !userMatchesStatus(u, statusFilter) })
	}

	if listItems == nil {
		listItems = []userListItem{}
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"count":      len(listItems),
		"limit":      pagination.Limit,
		"offset":     pagination.Offset,
		"source":     source,
		"is_stubbed": source == "stub",
		"users":      listItems,
	})
}

// ── GET /users/staff ───────────────────────────────────────────────────────────

type staffListItem struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

func (h *UserHandlers) listStaff(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	members, err := NewStaffDirectory(h.kc).ListStaff(r.Context(), orgID)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to list staff.")
		return
	}
	staff := make([]staffListItem, 0, len(members))
	for _, m := range members {
		staff = append(staff, staffListItem{ID: m.ID, Name: m.DisplayName(), Roles: m.Roles})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(staff),
		"staff":  staff,
	})
}

// ── GET /users/{id} ────────────────────────────────────────────────────────────

func (h *UserHandlers) getUser(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
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
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
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

	newUserID, ok := h.createKeycloakUser(w, r, req.Email, req.FirstName, req.LastName, req.OrgID)
	if !ok {
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

	if err := h.kc.UpdateUser(r.Context(), newUserID, keycloak.UpdateUserFields{Attributes: registrationAttributes}); err != nil {
		// Without these attributes the account is not in the approval queue (and
		// has no org), so do not leave a half-registered user behind.
		if derr := h.kc.DeleteUser(r.Context(), newUserID); derr != nil {
			slog.Error("register: cleanup of half-registered user failed", "user_id", newUserID, "error", derr)
		}
		writeKeycloakAdminError(w, r, err, "Failed to record the registration. Please try again.")
		return
	}

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
		_ = appendAudit(h.auditService, audit.AppendRequest{
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req createUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
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
	if req.Role != "" && !requireAssignableRole(w, r, principal, req.Role, "assign") {
		return
	}

	newUserID, ok := h.createKeycloakUser(w, r, req.Email, req.FirstName, req.LastName, orgID)
	if !ok {
		return
	}

	// Set the password directly.
	if pwdErr := h.kc.SetUserPassword(r.Context(), newUserID, req.Password, req.TemporaryPwd); pwdErr != nil {
		// Best-effort cleanup: delete the orphaned user.
		_ = h.kc.DeleteUser(r.Context(), newUserID)
		writeKeycloakAdminError(w, r, pwdErr, "Failed to set user password.")
		return
	}

	h.assignInitialRole(r.Context(), principal, newUserID, req.Role)

	// Auto-create a linked household if requested.
	var householdID string
	if req.CreateHousehold && h.membershipService != nil {
		hh, hhErr := h.createLinkedHousehold(principal, orgID, req.FirstName, req.LastName, req.Email, newUserID)
		if hhErr == nil {
			householdID = hh.ID
		}
	}

	h.emitAudit(principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
		"email":        req.Email,
		"role":         req.Role,
		"method":       "direct_create",
		"household_id": householdID,
	})

	writeUserCreated(w, newUserID, householdID)
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	var req inviteUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
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
	if req.Role != "" && !requireAssignableRole(w, r, principal, req.Role, "assign") {
		return
	}

	newUserID, ok := h.createKeycloakUser(w, r, req.Email, req.FirstName, req.LastName, orgID)
	if !ok {
		return
	}

	h.assignInitialRole(r.Context(), principal, newUserID, req.Role)

	// If requested and a members service is available, auto-create a linked household.
	var householdID string
	if req.CreateHousehold && h.membershipService != nil {
		hh, hhErr := h.createLinkedHousehold(principal, orgID, req.FirstName, req.LastName, req.Email, newUserID)
		if hhErr != nil {
			// Non-fatal: user is already created; log the failure and continue.
			h.emitAudit(principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
				"email":           req.Email,
				"household_error": hhErr.Error(),
			})
		} else {
			householdID = hh.ID
		}
	}

	// Send welcome / verification email.
	_ = h.kc.SendVerificationEmail(r.Context(), newUserID)

	h.emitAudit(principal, audit.ActionAuthUserInvite, newUserID, map[string]string{
		"email":        req.Email,
		"role":         req.Role,
		"household_id": householdID,
	})

	writeUserCreated(w, newUserID, householdID)
}

// ── PUT /users/{id} ────────────────────────────────────────────────────────────

type updateUserRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

func (h *UserHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	var req updateUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}

	fields := keycloak.UpdateUserFields{
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		Email:     strings.TrimSpace(strings.ToLower(req.Email)),
	}

	if err := h.kc.UpdateUser(r.Context(), userID, fields); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to update user.")
		return
	}

	h.emitAudit(principal, audit.ActionAuthUserUpdate, userID, map[string]string{
		"email":      req.Email,
		"first_name": req.FirstName,
		"last_name":  req.LastName,
	})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

// ── POST /users/{id}/disable ───────────────────────────────────────────────────

func (h *UserHandlers) disableUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.DisableUser(r.Context(), userID); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to disable user.")
		return
	}

	h.emitAudit(principal, audit.ActionAuthUserDisable, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
}

// ── POST /users/{id}/enable ────────────────────────────────────────────────────

func (h *UserHandlers) enableUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.EnableUser(r.Context(), userID); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to enable user.")
		return
	}

	h.emitAudit(principal, audit.ActionAuthUserEnable, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "enabled"})
}

// ── POST /users/{id}/approve ───────────────────────────────────────────────────

func (h *UserHandlers) approveUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	user, ok := h.requireUserInOrg(w, r, userID, orgID)
	if !ok {
		return
	}

	requestedRole := normalizeUserRole(firstUserAttribute(user, "requested_role"))
	if requestedRole == "" {
		requestedRole = "parent"
	}

	// Approval must not let an approver grant a role above their own tier —
	// the same guard applied to direct create/invite/assign-role paths.
	// Self-registration can request any role, so this is the only gate that
	// prevents a lower-tier approver from indirectly granting a role above
	// their own. It runs before any change so a refusal leaves the account untouched.
	if actorMaxTier(principal.Roles) < roleTier[requestedRole] {
		httpx.WriteError(w, r, http.StatusForbidden, "insufficient_role_tier", "You cannot approve a user for a role above your own.", nil)
		return
	}

	if err := h.kc.EnableUser(r.Context(), userID); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to enable user.")
		return
	}

	attrs := cloneUserAttributes(user)
	setUserAttribute(attrs, "org_id", user.OrgID())
	setUserAttribute(attrs, "registration_status", "active")
	// Each remaining step is reported, never swallowed. The user is already
	// enabled, and every step is idempotent, so a retry of Approve completes it.
	if err := h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{Attributes: attrs}); err != nil {
		writeKeycloakAdminError(w, r, err, "User was enabled but the registration status could not be updated. Approve again to retry.")
		return
	}

	// Assign the requested role if the user has no realm roles yet.
	existingRoles, err := h.kc.GetUserRealmRoles(r.Context(), userID)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "User was enabled but their roles could not be checked. Approve again to retry.")
		return
	}
	if len(existingRoles) == 0 {
		if err := h.kc.AssignRealmRole(r.Context(), userID, requestedRole); err != nil {
			writeKeycloakAdminError(w, r, err, "User was enabled but the requested role could not be assigned. Approve again to retry.")
			return
		}
	}

	h.emitAudit(principal, audit.ActionAuthUserApprove, userID, map[string]string{
		"email":          user.Email,
		"requested_role": requestedRole,
	})

	resp := map[string]any{"status": "approved"}
	// Trigger welcome / verification email; the approval stands if it fails,
	// but the caller is told.
	if err := h.kc.SendVerificationEmail(r.Context(), userID); err != nil {
		slog.Error("approve user: verification email failed", "user_id", userID, "error", err)
		resp["warnings"] = []string{"verification_email_failed"}
	}
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

// ── POST /users/{id}/reject ────────────────────────────────────────────────────

type rejectUserRequest struct {
	Reason string `json:"reason"`
}

func (h *UserHandlers) rejectUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
		writeKeycloakUserError(w, r, err, "Failed to disable user.")
		return
	}

	attrs := cloneUserAttributes(user)
	setUserAttribute(attrs, "org_id", user.OrgID())
	setUserAttribute(attrs, "registration_status", "rejected")
	if err := h.kc.UpdateUser(r.Context(), userID, keycloak.UpdateUserFields{Attributes: attrs}); err != nil {
		writeKeycloakAdminError(w, r, err, "User was disabled but the registration status could not be updated. Reject again to retry.")
		return
	}

	meta := map[string]string{"email": user.Email}
	if req.Reason != "" {
		meta["reason"] = req.Reason
	}
	h.emitAudit(principal, audit.ActionAuthUserReject, userID, meta)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

// ── DELETE /users/{id} ─────────────────────────────────────────────────────────

func (h *UserHandlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
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
	targetIsAdmin, err := h.userIsOrgAdmin(r.Context(), userID)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Could not verify the user's roles; nothing was deleted.")
		return
	}
	if targetIsAdmin {
		onlyOne, err := h.orgHasOnlyOneAdmin(r.Context(), orgID)
		if err != nil {
			writeKeycloakAdminError(w, r, err, "Could not verify the org's admin count; nothing was deleted.")
			return
		}
		if onlyOne {
			httpx.WriteError(w, r, http.StatusConflict, "last_admin", "Cannot delete the last admin in this org.", nil)
			return
		}
	}

	if err := h.kc.DeleteUser(r.Context(), userID); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to delete user.")
		return
	}

	h.emitAudit(principal, audit.ActionAuthUserDelete, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

// ── GET /users/{id}/roles ──────────────────────────────────────────────────────

func (h *UserHandlers) listUserRoles(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	// Assigning a role requires MFA, mirroring the guard on user deletion.
	if !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA authentication is required to assign roles.", nil)
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	var req userAssignRoleRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	req.Role = normalizeUserRole(req.Role)

	if req.Role == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "role is required.", nil)
		return
	}

	if !requireAssignableRole(w, r, principal, req.Role, "assign") {
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

	h.emitAudit(principal, audit.ActionAuthRoleAssign, userID, map[string]string{"role": req.Role})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "assigned"})
}

// ── DELETE /users/{id}/roles/{role} ────────────────────────────────────────────

func (h *UserHandlers) removeRole(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	// Removing a role requires MFA, mirroring the guard on user deletion.
	if !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA authentication is required to remove roles.", nil)
		return
	}

	userID := chi.URLParam(r, "id")
	roleName := normalizeUserRole(chi.URLParam(r, "role"))

	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if !requireAssignableRole(w, r, principal, roleName, "remove") {
		return
	}

	// Last-admin guard: prevent removal of the final admin role in the org.
	if roleName == "admin" {
		onlyOne, err := h.orgHasOnlyOneAdmin(r.Context(), orgID)
		if err != nil {
			writeKeycloakAdminError(w, r, err, "Could not verify the org's admin count; the role was not removed.")
			return
		}
		if onlyOne {
			httpx.WriteError(w, r, http.StatusConflict, "last_admin", "Cannot remove the last admin role assignment.", nil)
			return
		}
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

	h.emitAudit(principal, audit.ActionAuthRoleRemove, userID, map[string]string{"role": roleName})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "removed"})
}

// ── POST /users/{id}/password-reset ────────────────────────────────────────────

func (h *UserHandlers) passwordReset(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if err := h.kc.SendPasswordResetEmail(r.Context(), userID); err != nil {
		writeKeycloakUserError(w, r, err, "Failed to send password reset email.")
		return
	}

	h.emitAudit(principal, audit.ActionAuthUserPasswordReset, userID, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "password_reset_sent"})
}

// ── GET /users/{id}/household ──────────────────────────────────────────────────

func (h *UserHandlers) getUserHousehold(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if !h.requireMembership(w, r) {
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

type swimmerIDBody struct {
	SwimmerID string `json:"swimmer_id"`
}

type householdIDBody struct {
	HouseholdID string `json:"household_id"`
}

// requireMembership writes a 503 when the members service is not wired.
func (h *UserHandlers) requireMembership(w http.ResponseWriter, r *http.Request) bool {
	if h.membershipService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Membership service is not available.", nil)
		return false
	}
	return true
}

// userLinkScope runs the checks shared by the user link/unlink routes: org
// scope, the target user belonging to the org, and the membership service being wired.
func (h *UserHandlers) userLinkScope(w http.ResponseWriter, r *http.Request) (principal *appmiddleware.Principal, orgID, userID string, ok bool) {
	principal, orgID, ok = scopedRequestFromContext(w, r)
	if !ok {
		return nil, "", "", false
	}
	userID = chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return nil, "", "", false
	}
	if !h.requireMembership(w, r) {
		return nil, "", "", false
	}
	return principal, orgID, userID, true
}

// decodeRequiredID decodes a single-ID JSON body and returns the trimmed,
// non-empty value of the field named by pick, or writes a 400 and returns ok=false.
func decodeRequiredID[T any](w http.ResponseWriter, r *http.Request, field string, pick func(T) string) (string, bool) {
	var body T
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return "", false
	}
	id := strings.TrimSpace(pick(body))
	if id == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", field+" is required.", nil)
		return "", false
	}
	return id, true
}

func (h *UserHandlers) linkUserHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, userID, ok := h.userLinkScope(w, r)
	if !ok {
		return
	}

	householdID, ok := decodeRequiredID(w, r, "household_id", func(b householdIDBody) string { return b.HouseholdID })
	if !ok {
		return
	}

	if err := h.membershipService.LinkHouseholdToUser(orgID, householdID, userID); err != nil {
		switch {
		case errors.Is(err, members.ErrHouseholdNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusConflict, "link_conflict", err.Error(), nil)
		}
		return
	}

	h.emitAudit(principal, "user.household.link", userID, map[string]string{
		"household_id": householdID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "linked",
		"household_id": householdID,
	})
}

// ── POST /users/{id}/household/unlink ─────────────────────────────────────────

func (h *UserHandlers) unlinkUserHousehold(w http.ResponseWriter, r *http.Request) {
	principal, orgID, userID, ok := h.userLinkScope(w, r)
	if !ok {
		return
	}

	householdID, ok := decodeRequiredID(w, r, "household_id", func(b householdIDBody) string { return b.HouseholdID })
	if !ok {
		return
	}

	if err := h.membershipService.UnlinkHouseholdFromUser(orgID, householdID); err != nil {
		if errors.Is(err, members.ErrHouseholdNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "service_error", "Failed to unlink household.", err)
		}
		return
	}

	h.emitAudit(principal, "user.household.unlink", userID, map[string]string{
		"household_id": householdID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "unlinked",
		"household_id": householdID,
	})
}

// ── GET /users/{id}/swimmer ────────────────────────────────────────────────

func (h *UserHandlers) getUserSwimmer(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "id")
	if _, ok := h.requireUserInOrg(w, r, userID, orgID); !ok {
		return
	}

	if !h.requireMembership(w, r) {
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

func (h *UserHandlers) linkUserSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, userID, ok := h.userLinkScope(w, r)
	if !ok {
		return
	}

	swimmerID, ok := decodeRequiredID(w, r, "swimmer_id", func(b swimmerIDBody) string { return b.SwimmerID })
	if !ok {
		return
	}

	if err := h.membershipService.LinkSwimmerToUser(orgID, swimmerID, userID); err != nil {
		switch {
		case errors.Is(err, members.ErrSwimmerNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer not found.", nil)
		default:
			httpx.WriteError(w, r, http.StatusConflict, "link_conflict", err.Error(), nil)
		}
		return
	}

	h.emitAudit(principal, "user.swimmer.link", userID, map[string]string{
		"swimmer_id": swimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "linked",
		"swimmer_id": swimmerID,
	})
}

// ── POST /users/{id}/swimmer/unlink ───────────────────────────────────────

func (h *UserHandlers) unlinkUserSwimmer(w http.ResponseWriter, r *http.Request) {
	principal, orgID, userID, ok := h.userLinkScope(w, r)
	if !ok {
		return
	}

	swimmerID, ok := decodeRequiredID(w, r, "swimmer_id", func(b swimmerIDBody) string { return b.SwimmerID })
	if !ok {
		return
	}

	if err := h.membershipService.UnlinkSwimmerFromUser(orgID, swimmerID); err != nil {
		if errors.Is(err, members.ErrSwimmerNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusInternalServerError, "service_error", "Failed to unlink swimmer.", err)
		}
		return
	}

	h.emitAudit(principal, "user.swimmer.unlink", userID, map[string]string{
		"swimmer_id": swimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "unlinked",
		"swimmer_id": swimmerID,
	})
}

// ── POST /users/me/swimmer-link-requests ──────────────────────────────────

type createSwimmerLinkRequestBody struct {
	RequestedSwimmerName string `json:"requested_swimmer_name"`
	Note                 string `json:"note"`
}

func (h *UserHandlers) createSwimmerLinkRequest(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !h.requireMembership(w, r) {
		return
	}

	var body createSwimmerLinkRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}

	req, err := h.membershipService.CreateLinkRequest(orgID, principal.UserID, body.RequestedSwimmerName, body.Note)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create link request.", err)
		return
	}

	h.emitAudit(principal, "user.swimmer_link_request.create", principal.UserID, map[string]string{
		"request_id": req.ID,
	})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"request": req})
}

// ── GET /users/swimmer-link-requests ───────────────────────────────────────

func (h *UserHandlers) listSwimmerLinkRequests(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !h.requireMembership(w, r) {
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !h.requireMembership(w, r) {
		return
	}

	requestID := chi.URLParam(r, "requestID")
	var body resolveSwimmerLinkRequestBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
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

	h.emitAudit(principal, "user.swimmer_link_request.resolve", target.UserID, map[string]string{
		"request_id": requestID,
		"swimmer_id": body.SwimmerID,
	})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "resolved",
		"swimmer_id": body.SwimmerID,
	})
}
