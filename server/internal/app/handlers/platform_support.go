package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/httpx"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/payments"
	"github.com/luryann/aquify/server/internal/keycloak"
)

// PlatformSupport is what the operator plane needs from authn to help a club's
// users: end their sessions, clear their MFA, and see who has MFA enrolled.
type PlatformSupport interface {
	RevokeUserSessions(ctx context.Context, orgID, userID string) (int, error)
	ResetMFA(ctx context.Context, orgID, userID string) (int, error)
	VerifiedMFAUsers(ctx context.Context, orgID string) (map[string]bool, error)
}

// SetSupport enables the club support routes (users, account recovery). Without
// it they answer 501 so a deployment with no identity provider degrades cleanly.
func (h *PlatformHandlers) SetSupport(kc keycloak.Client, support PlatformSupport) {
	h.kc, h.support = kc, support
}

// SetPayments lets the club overview show Stripe Connect status (read-only).
func (h *PlatformHandlers) SetPayments(p *payments.Service) { h.payments = p }

// supportRoles are the club roles an operator can see on a user. platform_admin
// is deliberately not listed: operators are never managed through this plane.
var supportRoles = []string{"admin", "coach", "parent", "swimmer"}

const (
	maxReasonLen = 500
	minReasonLen = 3
)

type reasonRequest struct {
	Reason string `json:"reason"`
}

// supportOrg resolves the club in the path and refuses clubs that no longer exist
// for support purposes. A suspended or offboarding club is still supportable.
func (h *PlatformHandlers) supportOrg(w http.ResponseWriter, r *http.Request) (*orgs.Org, bool) {
	if h.kc == nil || h.support == nil {
		httpx.WriteError(w, r, http.StatusNotImplemented, "support_unavailable", "Club support tools are not configured.", nil)
		return nil, false
	}
	o, err := h.orgs.Get(r.Context(), chi.URLParam(r, "id"))
	switch {
	case errors.Is(err, orgs.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
		return nil, false
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load organization.", err)
		return nil, false
	case o.Status == orgs.StatusDeleted || o.Status == orgs.StatusPurging:
		httpx.WriteError(w, r, http.StatusConflict, "org_unavailable", "This club has been deleted.", nil)
		return nil, false
	case h.isPlatformOrg(o.ID):
		httpx.WriteError(w, r, http.StatusConflict, "platform_org_protected", "Operator accounts are not managed here.", nil)
		return nil, false
	}
	return o, true
}

func (h *PlatformHandlers) isPlatformOrg(id string) bool {
	return h.platformOrgID != "" && orgs.CanonicalID(id) == orgs.CanonicalID(h.platformOrgID)
}

// supportUser loads the user in the path and proves they belong to the club in
// the path. A user of another club is reported as not found, never as forbidden,
// so a mismatched URL cannot be used to probe other clubs' accounts.
func (h *PlatformHandlers) supportUser(w http.ResponseWriter, r *http.Request, o *orgs.Org) (*keycloak.User, bool) {
	u, err := h.kc.GetUser(r.Context(), chi.URLParam(r, "userID"))
	if err != nil {
		writeKeycloakUserError(w, r, err, "Failed to load the user.")
		return nil, false
	}
	if orgs.CanonicalID(u.OrgID()) != orgs.CanonicalID(o.ID) {
		httpx.WriteError(w, r, http.StatusNotFound, "user_not_found", "User not found.", nil)
		return nil, false
	}
	roles, err := h.kc.GetUserRealmRoles(r.Context(), u.ID)
	if err != nil {
		writeKeycloakUserError(w, r, err, "Failed to load the user's roles.")
		return nil, false
	}
	for _, role := range roles {
		if role.Name == PlatformRole {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Operator accounts are not managed here.", nil)
			return nil, false
		}
	}
	return u, true
}

// requireReason reads the mandatory support reason that is stored in the audit trail.
func requireReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req reasonRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "reason_required", "Say why you are doing this (3-500 characters).")
		return "", false
	}
	reason := strings.TrimSpace(req.Reason)
	if n := len([]rune(reason)); n < minReasonLen || n > maxReasonLen {
		httpx.WriteError(w, r, http.StatusBadRequest, "reason_required", "Say why you are doing this (3-500 characters).", nil)
		return "", false
	}
	return reason, true
}

// recordSupport writes the action to the operator log and mirrors it into the club's
// own log, so a club admin reviewing their audit trail sees what support did.
func (h *PlatformHandlers) recordSupport(r *http.Request, action, orgID, resourceType, resourceID string, meta map[string]string, after any) {
	meta = h.recordOperator(r, action, orgID, resourceType, resourceID, meta, after)
	if !h.isPlatformOrg(orgID) {
		_ = recordAudit(h.audit, h.principal(r), orgID, action, resourceType, resourceID, nil, after, meta)
	}
}

// recordOperator writes the action to the operator (platform org) log only.
func (h *PlatformHandlers) recordOperator(r *http.Request, action, orgID, resourceType, resourceID string, meta map[string]string, after any) map[string]string {
	if meta == nil {
		meta = map[string]string{}
	}
	meta["target_org_id"] = orgID
	_ = recordAudit(h.audit, h.principal(r), h.platformOrgID, action, resourceType, resourceID, nil, after, meta)
	return meta
}

// ── PATCH /platform/orgs/{id} ────────────────────────────────────────────────

type updateOrgRequest struct {
	Name   *string `json:"name"`
	Plan   *string `json:"plan"`
	Reason string  `json:"reason"`
}

func (h *PlatformHandlers) updateOrg(w http.ResponseWriter, r *http.Request) {
	var req updateOrgRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	if req.Name == nil && req.Plan == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Provide a name or a plan to change.", nil)
		return
	}
	if n := len([]rune(strings.TrimSpace(req.Reason))); n < minReasonLen || n > maxReasonLen {
		httpx.WriteError(w, r, http.StatusBadRequest, "reason_required", "Say why you are doing this (3-500 characters).", nil)
		return
	}
	id := chi.URLParam(r, "id")
	before, err := h.orgs.Get(r.Context(), id)
	if errors.Is(err, orgs.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load organization.", err)
		return
	}
	updated, err := h.orgs.Update(r.Context(), id, orgs.UpdateInput{Name: req.Name, Plan: req.Plan})
	switch {
	case errors.Is(err, orgs.ErrInvalidField):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
	case errors.Is(err, orgs.ErrInvalidStatus):
		httpx.WriteError(w, r, http.StatusConflict, "org_unavailable", "This club has been deleted.", nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not update organization.", err)
	default:
		h.recordSupport(r, audit.ActionPlatformOrgUpdate, updated.ID, "org", updated.ID,
			map[string]string{"reason": strings.TrimSpace(req.Reason), "previous_name": before.Name, "previous_plan": before.Plan}, updated)
		_ = httpx.WriteJSON(w, http.StatusOK, updated)
	}
}

// ── GET /platform/orgs/{id}/overview ─────────────────────────────────────────

type paymentStatus struct {
	Connected        bool `json:"connected"`
	ChargesEnabled   bool `json:"charges_enabled"`
	PayoutsEnabled   bool `json:"payouts_enabled"`
	DetailsSubmitted bool `json:"details_submitted"`
}

// overview is the support snapshot of one club: registry fields, domains, how many
// people hold each role and whether Stripe Connect is ready. Directory metadata
// only; it never reads a club's swimmers, invoices or households.
func (h *PlatformHandlers) overview(w http.ResponseWriter, r *http.Request) {
	o, ok := h.supportOrg(w, r)
	if !ok {
		return
	}
	domains, err := h.orgs.Domains(r.Context(), o.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load domains.", err)
		return
	}
	roleCounts, err := h.countRoleHolders(r.Context(), o.ID)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to count users.")
		return
	}
	pay := paymentStatus{}
	if h.payments != nil {
		switch acct, err := h.payments.Get(r.Context(), o.ID); {
		case err == nil:
			pay = paymentStatus{Connected: true, ChargesEnabled: acct.ChargesEnabled, PayoutsEnabled: acct.PayoutsEnabled, DetailsSubmitted: acct.DetailsSubmitted}
		case !errors.Is(err, payments.ErrNotFound):
			slog.Warn("platform overview: payment account lookup failed", "org_id", o.ID, "error", err)
		}
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"org": o, "domains": domains, "role_counts": roleCounts, "payments": pay,
	})
}

// ── GET /platform/orgs/{id}/users ────────────────────────────────────────────

type supportUserView struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	FirstName     string   `json:"first_name"`
	LastName      string   `json:"last_name"`
	Enabled       bool     `json:"enabled"`
	EmailVerified bool     `json:"email_verified"`
	MFAEnrolled   bool     `json:"mfa_enrolled"`
	Roles         []string `json:"roles"`
}

func (h *PlatformHandlers) listClubUsers(w http.ResponseWriter, r *http.Request) {
	o, ok := h.supportOrg(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page := parseListPagination(r, listPaginationOptions{DefaultLimit: 50, MaxLimit: 200})
	search := strings.TrimSpace(q.Get("search"))
	roleFilter := strings.ToLower(strings.TrimSpace(q.Get("role")))

	// Search runs on the server so a club with hundreds of accounts is fully
	// searchable. With a role filter, that one role's members are the base set,
	// matched and paged here so the filter applies before paging. Without one the
	// realm search is paged by Keycloak and role labels are resolved for the
	// returned page only, never for every member of every role.
	var users []keycloak.User
	if roleFilter != "" {
		if !slices.Contains(supportRoles, roleFilter) {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Unknown role filter.", nil)
			return
		}
		holders, err := h.kc.ListUsersWithRealmRole(r.Context(), o.ID, roleFilter)
		if err != nil {
			writeKeycloakAdminError(w, r, err, "Failed to load roles.")
			return
		}
		needle := strings.ToLower(search)
		for _, u := range holders {
			if needle == "" || strings.Contains(strings.ToLower(u.Email+" "+u.FirstName+" "+u.LastName), needle) {
				users = append(users, u)
			}
		}
		if page.Offset >= len(users) {
			users = nil
		} else {
			users = users[page.Offset:]
		}
		if len(users) > page.Limit {
			users = users[:page.Limit]
		}
	} else {
		var err error
		users, err = h.kc.ListUsers(r.Context(), o.ID, search, page.Limit, page.Offset)
		if err != nil {
			writeKeycloakAdminError(w, r, err, "Failed to list users.")
			return
		}
	}
	rolesByUser, err := h.supportRolesFor(r.Context(), users)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to load roles.")
		return
	}
	mfa, err := h.support.VerifiedMFAUsers(r.Context(), o.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load MFA status.", err)
		return
	}
	items := make([]supportUserView, 0, len(users))
	for _, u := range users {
		roles := rolesByUser[u.ID]
		if roles == nil {
			roles = []string{}
		}
		items = append(items, supportUserView{
			ID: u.ID, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName,
			Enabled: u.Enabled, EmailVerified: u.EmailVerified, MFAEnrolled: mfa[u.ID], Roles: roles,
		})
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items, "count": len(items), "limit": page.Limit, "offset": page.Offset,
	})
}

// ── POST /platform/orgs/{id}/users/{userID}/{action} ─────────────────────────

// userAction runs one account-recovery action against a club user: it checks the
// user belongs to the club, demands a reason, performs the action, and audits it.
func (h *PlatformHandlers) userAction(action string, run func(ctx context.Context, o *orgs.Org, u *keycloak.User) (map[string]string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, ok := h.supportOrg(w, r)
		if !ok {
			return
		}
		u, ok := h.supportUser(w, r, o)
		if !ok {
			return
		}
		reason, ok := requireReason(w, r)
		if !ok {
			return
		}
		extra, err := run(r.Context(), o, u)
		meta := map[string]string{"reason": reason, "target_user_id": u.ID, "target_email": u.Email}
		for k, v := range extra {
			meta[k] = v
		}
		if err != nil {
			// Multi-step actions can change state before a later step fails, so
			// a failed attempt is audited too, marked so reviewers can tell.
			meta["outcome"] = "failed"
			h.recordSupport(r, action, o.ID, "keycloak_user", u.ID, meta, nil)
			writeKeycloakAdminError(w, r, err, "That step didn't finish. Try again.")
			return
		}
		h.recordSupport(r, action, o.ID, "keycloak_user", u.ID, meta, nil)
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "action": action, "user_id": u.ID})
	}
}

func (h *PlatformHandlers) passwordReset() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserPasswordReset, func(ctx context.Context, _ *orgs.Org, u *keycloak.User) (map[string]string, error) {
		return nil, h.kc.SendPasswordResetEmail(ctx, u.ID)
	})
}

func (h *PlatformHandlers) resendVerification() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserResendVerification, func(ctx context.Context, _ *orgs.Org, u *keycloak.User) (map[string]string, error) {
		return nil, h.kc.SendVerificationEmail(ctx, u.ID)
	})
}

func (h *PlatformHandlers) signOutUser() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserSignOut, func(ctx context.Context, o *orgs.Org, u *keycloak.User) (map[string]string, error) {
		n, err := h.support.RevokeUserSessions(ctx, o.ID, u.ID)
		return map[string]string{"revoked_sessions": itoa(n)}, err
	})
}

// resetMFA clears the user's authenticator and backup codes, then ends their sessions
// so nothing signed in under the old factor survives. They re-enroll at next sign-in.
func (h *PlatformHandlers) resetMFA() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserMFAReset, func(ctx context.Context, o *orgs.Org, u *keycloak.User) (map[string]string, error) {
		removed, err := h.support.ResetMFA(ctx, o.ID, u.ID)
		if err != nil {
			return nil, err
		}
		revoked, err := h.support.RevokeUserSessions(ctx, o.ID, u.ID)
		return map[string]string{"removed_factors": itoa(removed), "revoked_sessions": itoa(revoked)}, err
	})
}

func (h *PlatformHandlers) disableUser() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserDisable, func(ctx context.Context, o *orgs.Org, u *keycloak.User) (map[string]string, error) {
		if err := h.kc.DisableUser(ctx, u.ID); err != nil {
			return nil, err
		}
		n, err := h.support.RevokeUserSessions(ctx, o.ID, u.ID)
		return map[string]string{"disabled": "true", "revoked_sessions": itoa(n)}, err
	})
}

func (h *PlatformHandlers) enableUser() http.HandlerFunc {
	return h.userAction(audit.ActionPlatformUserEnable, func(ctx context.Context, _ *orgs.Org, u *keycloak.User) (map[string]string, error) {
		return nil, h.kc.EnableUser(ctx, u.ID)
	})
}

// ── POST /platform/orgs/{id}/admins ──────────────────────────────────────────

type addAdminRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Reason    string `json:"reason"`
}

// addAdmin gives a club a new administrator: the usual "our admin left" request.
// An existing user of the club is promoted; otherwise a new one is created and
// invited. The email belongs to one club only, so an address in use elsewhere is refused.
func (h *PlatformHandlers) addAdmin(w http.ResponseWriter, r *http.Request) {
	o, ok := h.supportOrg(w, r)
	if !ok {
		return
	}
	var req addAdminRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	first, last := strings.TrimSpace(req.FirstName), strings.TrimSpace(req.LastName)
	reason := strings.TrimSpace(req.Reason)
	if n := len([]rune(reason)); n < minReasonLen || n > maxReasonLen {
		httpx.WriteError(w, r, http.StatusBadRequest, "reason_required", "Say why you are doing this (3-500 characters).", nil)
		return
	}
	if !strings.Contains(email, "@") || len(email) > 254 || first == "" || last == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "An email, first name and last name are required.", nil)
		return
	}

	userID, promoted := "", false
	existing, err := h.findClubUserByEmail(r.Context(), o.ID, email)
	if err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to look up the user.")
		return
	}
	if u := existing; u != nil {
		roles, err := h.kc.GetUserRealmRoles(r.Context(), u.ID)
		if err != nil {
			writeKeycloakUserError(w, r, err, "Failed to load the user's roles.")
			return
		}
		if slices.ContainsFunc(roles, func(role keycloak.Role) bool { return role.Name == PlatformRole }) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Operator accounts are not managed here.", nil)
			return
		}
		if !u.Enabled {
			httpx.WriteError(w, r, http.StatusConflict, "user_disabled", "That account is disabled. Enable it first, then make it an administrator.", nil)
			return
		}
		userID, promoted = u.ID, true
	}
	if !promoted {
		id, err := h.kc.CreateUser(r.Context(), email, first, last, o.ID)
		if errors.Is(err, keycloak.ErrConflict) {
			httpx.WriteError(w, r, http.StatusConflict, "admin_email_in_use", "That email already belongs to a user in another club.", nil)
			return
		}
		if err != nil {
			writeKeycloakAdminError(w, r, err, "Failed to create the user.")
			return
		}
		userID = id
	}
	if err := h.kc.AssignRealmRole(r.Context(), userID, "admin"); err != nil {
		writeKeycloakAdminError(w, r, err, "Failed to grant the admin role.")
		return
	}
	if !promoted {
		// The invite is best effort: the account and role exist, so a mail failure is
		// reported to the operator, who can resend from the user's row.
		if err := h.kc.SendVerificationEmail(r.Context(), userID); err != nil {
			slog.Warn("platform add admin: verification email failed", "org_id", o.ID, "error", err)
		}
		if err := h.kc.SendPasswordResetEmail(r.Context(), userID); err != nil {
			slog.Warn("platform add admin: password email failed", "org_id", o.ID, "error", err)
		}
	}
	h.recordSupport(r, audit.ActionPlatformUserAdminAdd, o.ID, "keycloak_user", userID,
		map[string]string{"reason": reason, "target_user_id": userID, "target_email": email, "promoted_existing": boolString(promoted)}, nil)
	status := http.StatusCreated
	if promoted {
		status = http.StatusOK
	}
	_ = httpx.WriteJSON(w, status, map[string]any{"user_id": userID, "promoted": promoted})
}

// ── GET /platform/orgs/{id}/activity ─────────────────────────────────────────

// activity lists what operators have done to this club, newest first, from the
// club's own audit log (the same entries the club's admins can see).
func (h *PlatformHandlers) activity(w http.ResponseWriter, r *http.Request) {
	o, err := h.orgs.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, orgs.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load organization.", err)
		return
	}
	if h.audit == nil {
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []audit.AuditEvent{}, "count": 0})
		return
	}
	events, err := h.audit.Query(audit.Filter{OrgID: o.ID, ActionPrefix: "platform.", Limit: 50})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load activity.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": events, "count": len(events)})
}

// roleLookupWorkers bounds concurrent Keycloak calls made for one request.
const roleLookupWorkers = 8

// countRoleHolders counts the club's members of each support role. The four role
// queries are independent, so they run concurrently.
func (h *PlatformHandlers) countRoleHolders(ctx context.Context, orgID string) (map[string]int, error) {
	counts := make([]int, len(supportRoles))
	errs := make([]error, len(supportRoles))
	var wg sync.WaitGroup
	for i, role := range supportRoles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users, err := h.kc.ListUsersWithRealmRole(ctx, orgID, role)
			counts[i], errs[i] = len(users), err
		}()
	}
	wg.Wait()
	out := make(map[string]int, len(supportRoles))
	for i, role := range supportRoles {
		if errs[i] != nil {
			return nil, errs[i]
		}
		out[role] = counts[i]
	}
	return out, nil
}

// supportRolesFor resolves the support-visible roles of just the given users
// (one page), with bounded concurrency. Roles outside supportRoles, such as
// operator roles, are never reported.
func (h *PlatformHandlers) supportRolesFor(ctx context.Context, users []keycloak.User) (map[string][]string, error) {
	out := make(map[string][]string, len(users))
	if len(users) == 0 {
		return out, nil
	}
	resolved := make([][]string, len(users))
	errs := make([]error, len(users))
	sem := make(chan struct{}, roleLookupWorkers)
	var wg sync.WaitGroup
	for i := range users {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			roles, err := h.kc.GetUserRealmRoles(ctx, users[i].ID)
			if err != nil {
				errs[i] = err
				return
			}
			for _, name := range supportRoles { // stable display order
				if slices.ContainsFunc(roles, func(r keycloak.Role) bool { return r.Name == name }) {
					resolved[i] = append(resolved[i], name)
				}
			}
		}()
	}
	wg.Wait()
	for i, u := range users {
		if errs[i] != nil {
			return nil, errs[i]
		}
		out[u.ID] = resolved[i]
	}
	return out, nil
}

// findClubUserByEmail returns the club's user whose email is exactly email, or
// nil. Keycloak's search is a substring match, so the exact account may sit past
// the first page of hits; page until it is found or the hits run out.
func (h *PlatformHandlers) findClubUserByEmail(ctx context.Context, orgID, email string) (*keycloak.User, error) {
	const pageSize, maxPages = 50, 20
	for page := 0; page < maxPages; page++ {
		users, err := h.kc.ListUsers(ctx, orgID, email, pageSize, page*pageSize)
		if err != nil {
			return nil, err
		}
		for i := range users {
			if strings.EqualFold(users[i].Email, email) {
				return &users[i], nil
			}
		}
		if len(users) < pageSize {
			return nil, nil
		}
	}
	return nil, nil
}

func itoa(n int) string { return strconv.Itoa(n) }

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
