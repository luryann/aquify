package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/authn"
	"github.com/luryann/aquify/server/internal/app/httpx"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

type AuthHandlers struct {
	service              *authn.Service
	auditService         *audit.Service
	validate             *validator.Validate
	allowDevSessionIssue bool
	cookies              appmiddleware.SessionCookies
}

// SetSessionCookies configures cookie issuance (Secure flag, CSRF token factory).
// The zero value is Secure with the production token factory.
func (h *AuthHandlers) SetSessionCookies(c appmiddleware.SessionCookies) { h.cookies = c }

func NewAuthHandlers(service *authn.Service, allowDevSessionIssue bool) *AuthHandlers {
	return &AuthHandlers{
		service:              service,
		validate:             httpx.NewValidator(),
		allowDevSessionIssue: allowDevSessionIssue,
	}
}

// NewAuthHandlersWithAudit returns an AuthHandlers with audit logging enabled.
func NewAuthHandlersWithAudit(service *authn.Service, auditSvc *audit.Service, allowDevSessionIssue bool) *AuthHandlers {
	return &AuthHandlers{
		service:              service,
		auditService:         auditSvc,
		validate:             httpx.NewValidator(),
		allowDevSessionIssue: allowDevSessionIssue,
	}
}

func (h *AuthHandlers) RegisterPublicRoutes(router chi.Router) {
	if h.allowDevSessionIssue {
		router.Post("/sessions/issue", h.issueSession)
	}
	router.Post("/sessions/refresh", h.refreshSession)
	router.Post("/sessions/revoke", h.revokeSession)
}

func (h *AuthHandlers) RegisterNoCSRFRoutes(router chi.Router) {
	router.Post("/sessions/logout", h.logoutSession)
}

func (h *AuthHandlers) RegisterProtectedRoutes(router chi.Router) {
	router.Get("/session", h.currentSession)
	router.Get("/mfa/factors", h.listMFAFactors)
	router.Post("/mfa/enroll", h.enrollMFA)
	router.Post("/mfa/challenge", h.challengeMFA)
	router.Get("/roles/assignments", h.listRoleAssignments)
	router.Post("/roles/assign", h.assignRole)
}

func (h *AuthHandlers) OIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	loginHint := strings.TrimSpace(r.URL.Query().Get("login_hint"))
	orgID := strings.TrimSpace(r.URL.Query().Get("org_id"))

	location, err := h.service.BeginOIDCLogin(loginHint, orgID)
	if err != nil {
		status := http.StatusBadRequest
		code := "invalid_oidc_authorize"
		if errors.Is(err, authn.ErrOIDCNotConfigured) {
			status = http.StatusServiceUnavailable
			code = "oidc_not_configured"
		}
		httpx.WriteError(w, r, status, code, err.Error(), nil)
		return
	}

	// Bind the signed state to this browser so a state/code minted elsewhere
	// cannot be replayed into a victim's session (login CSRF).
	if parsed, perr := url.Parse(location); perr == nil {
		if nonce, nerr := h.service.OIDCStateNonce(parsed.Query().Get("state")); nerr == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     oidcStateCookieName,
				Value:    nonce,
				Path:     "/api/v1/auth/oidc",
				HttpOnly: true,
				Secure:   !h.cookies.Insecure,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   600,
			})
		}
	}

	http.Redirect(w, r, location, http.StatusFound)
}

const oidcStateCookieName = "aquify_oidc_state"

type oidcCallbackRequest struct {
	Code  string `json:"code" validate:"required"`
	State string `json:"state" validate:"required"`
	OrgID string `json:"org_id"`
}

func authSessionMetadata(expiresAt any) map[string]any {
	return map[string]any{
		"expires_at": expiresAt,
	}
}

// existingOrNewCSRFToken keeps the caller's CSRF cookie value when present, else mints a token.
func (h *AuthHandlers) existingOrNewCSRFToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(appmiddleware.CSRFTokenCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value), true
	}
	token, err := h.cookies.NewToken()
	if err != nil {
		writeCSRFGenerationError(w, r, err)
		return "", false
	}
	return token, true
}

func writeCSRFGenerationError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("csrf: token generation failed", "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "csrf_generation_failed", "Failed to generate CSRF token.", nil)
}

func (h *AuthHandlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	var req oidcCallbackRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	bound := ""
	if c, cerr := r.Cookie(oidcStateCookieName); cerr == nil {
		bound = c.Value
	}
	// Single use: the binding cookie is cleared whatever the outcome.
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookieName, Value: "", Path: "/api/v1/auth/oidc", HttpOnly: true, Secure: !h.cookies.Insecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if err := h.service.VerifyOIDCStateBinding(req.State, bound); err != nil {
		slog.Warn("oidc callback: state binding failed", "error", err)
		httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_oidc_state", "Sign-in state is invalid or expired. Please start again.", nil)
		return
	}

	result, err := h.service.OIDCCallback(req.Code, req.State, req.OrgID)
	if err != nil {
		switch {
		case errors.Is(err, authn.ErrOIDCNotConfigured):
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "oidc_not_configured", "Single sign-on is not configured.", nil)
		case errors.Is(err, authn.ErrOIDCProviderUnavailable):
			// Details (URLs, dial errors) are logged server-side only.
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "oidc_provider_unavailable", "The sign-in provider is temporarily unavailable. Please try again shortly.", err)
		case errors.Is(err, authn.ErrOIDCStateInvalid):
			slog.Warn("oidc callback: invalid state", "error", err)
			httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_oidc_state", "Sign-in state is invalid or expired. Please start again.", nil)
		case errors.Is(err, authn.ErrOrgNotActive):
			slog.Warn("oidc callback: organization unavailable", "error", err)
			httpx.WriteError(w, r, http.StatusForbidden, "org_unavailable", "Your club's account is not available. Contact your club administrator.", nil)
		case errors.Is(err, authn.ErrUserOrgMismatch):
			slog.Warn("oidc callback: token org does not match the user's org", "error", err)
			httpx.WriteError(w, r, http.StatusForbidden, "org_mismatch", "This account belongs to a different organization.", nil)
		case errors.Is(err, authn.ErrNoAppRole):
			slog.Warn("oidc callback: user has no application role", "error", err)
			httpx.WriteError(w, r, http.StatusForbidden, "no_app_role", "Your account has no role assigned yet. Contact your club administrator.", nil)
		default:
			slog.Warn("oidc callback rejected", "error", err)
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_oidc_callback", "Sign-in could not be completed. Please try again.", nil)
		}
		return
	}

	csrfToken, err := h.cookies.NewToken()
	if err != nil {
		writeCSRFGenerationError(w, r, err)
		return
	}
	h.cookies.Set(w, result.AccessToken, result.RefreshToken, csrfToken, result.ExpiresAt)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"user_id": result.UserID,
			"org_id":  result.OrgID,
			"roles":   result.Roles,
		},
		"session":    authSessionMetadata(result.ExpiresAt),
		"csrf_token": csrfToken,
	})
}

type issueSessionRequest struct {
	UserID           string   `json:"user_id" validate:"required"`
	Email            string   `json:"email"`
	Name             string   `json:"name"`
	OrgID            string   `json:"org_id" validate:"required"`
	HouseholdID      string   `json:"household_id"`
	Roles            []string `json:"roles" validate:"required,min=1"`
	MFAAuthenticated bool     `json:"mfa_authenticated"`
}

func (h *AuthHandlers) issueSession(w http.ResponseWriter, r *http.Request) {
	var req issueSessionRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	session, err := h.service.IssueSession(authn.SessionInput{
		UserID:           req.UserID,
		Email:            req.Email,
		Name:             req.Name,
		OrgID:            req.OrgID,
		HouseholdID:      req.HouseholdID,
		Roles:            req.Roles,
		MFAAuthenticated: req.MFAAuthenticated,
	})
	// swimmer_id is resolved server-side from the persisted keycloak_user_id
	// link (see Service.IssueSession), never accepted from the request body.
	if err != nil {
		if errors.Is(err, authn.ErrOrgNotActive) {
			httpx.WriteError(w, r, http.StatusForbidden, "org_unavailable", "This organization is not available.", nil)
			return
		}
		if errors.Is(err, authn.ErrUserOrgMismatch) {
			httpx.WriteError(w, r, http.StatusForbidden, "org_mismatch", "This account belongs to a different organization.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_session_request", err.Error(), nil)
		return
	}

	csrfToken, err := h.cookies.NewToken()
	if err != nil {
		writeCSRFGenerationError(w, r, err)
		return
	}
	h.cookies.Set(w, session.AccessToken, session.RefreshToken, csrfToken, session.ExpiresAt)

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":     "issued",
		"session":    authSessionMetadata(session.ExpiresAt),
		"csrf_token": csrfToken,
	})
}

func (h *AuthHandlers) currentSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"user_id":           principal.UserID,
			"email":             principal.Email,
			"name":              principal.Name,
			"org_id":            principal.OrgID,
			"household_id":      principal.HouseholdID,
			"swimmer_id":        principal.SwimmerID,
			"roles":             principal.Roles,
			"mfa_authenticated": principal.MFAAuthenticated,
		},
	})
}

func (h *AuthHandlers) listMFAFactors(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}

	factors := h.service.ListMFAFactors(principal.OrgID, principal.UserID)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"factors": factors,
		"count":   len(factors),
	})
}

func (h *AuthHandlers) listRoleAssignments(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Role assignment visibility requires admin role.", nil)
		return
	}

	assignments := h.service.ListRoleAssignments(orgID)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"assignments": assignments,
		"count":       len(assignments),
	})
}

type refreshSessionRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (h *AuthHandlers) refreshSession(w http.ResponseWriter, r *http.Request) {
	refreshToken := readRefreshToken(r)
	if refreshToken == "" {
		var req refreshSessionRequest
		if !h.decodeAndValidate(w, r, &req) {
			return
		}
		refreshToken = req.RefreshToken
	}

	session, err := h.service.RefreshSession(refreshToken)
	if err != nil {
		status := http.StatusUnauthorized
		code := "invalid_session"
		switch {
		case errors.Is(err, authn.ErrSessionNotFound):
			code = "session_not_found"
		case errors.Is(err, authn.ErrSessionExpired):
			code = "session_expired"
		case errors.Is(err, authn.ErrSessionRevoked):
			code = "session_revoked"
		default:
			// Not a known session outcome: the store failed, so tell the
			// client to retry instead of signing it out.
			w.Header().Set("Retry-After", "1")
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Session refresh is temporarily unavailable.", err)
			return
		}
		httpx.WriteError(w, r, status, code, "Unable to refresh session.", nil)
		return
	}

	csrfToken, ok := h.existingOrNewCSRFToken(w, r)
	if !ok {
		return
	}
	h.cookies.Set(w, session.AccessToken, session.RefreshToken, csrfToken, session.ExpiresAt)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "refreshed",
		"session":    authSessionMetadata(session.ExpiresAt),
		"csrf_token": csrfToken,
	})
}

type revokeSessionRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (h *AuthHandlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	// Clear session cookies FIRST — before any WriteHeader call.
	// Go's net/http ignores w.Header().Add(...) after WriteHeader is called,
	// so a defer would silently drop the Set-Cookie headers on every code path.
	h.cookies.Clear(w)

	refreshToken, ok := refreshTokenFromCookieOrBody(w, r)
	if !ok {
		return
	}

	// Attempt to revoke the server-side session record. Ignore not-found errors:
	// the session may not exist after a server restart, but the cookies have
	// already been cleared above so the client is effectively signed out.
	if refreshToken != "" {
		if err := h.service.RevokeSession(refreshToken); err != nil && !errors.Is(err, authn.ErrSessionNotFound) {
			// Cookies are already cleared; the client may retry with the token.
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "session_revocation_failed", "Unable to revoke the session. Retry.", err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandlers) logoutSession(w http.ResponseWriter, r *http.Request) {
	// Clear session cookies FIRST — before any WriteHeader call.
	h.cookies.Clear(w)

	accessToken, _, _ := appmiddleware.ExtractAuthToken(r)
	refreshToken, ok := refreshTokenFromCookieOrBody(w, r)
	if !ok {
		return
	}

	result := h.service.LogoutSessionWithAccess(refreshToken, accessToken)
	if result.RevokeErr != nil && !errors.Is(result.RevokeErr, authn.ErrSessionNotFound) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "session_revocation_failed", "Unable to revoke the session. Retry.", result.RevokeErr)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "logged_out",
		"logout_url":   result.LogoutURL,
		"redirect_url": result.RedirectURL,
	})
}

type enrollMFARequest struct {
	FactorType string `json:"factor_type" validate:"required,oneof=totp backup_code"`
	Label      string `json:"label"`
}

func (h *AuthHandlers) enrollMFA(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}

	var req enrollMFARequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	// Once a factor exists, adding another is a security-sensitive step-up
	// action: require a session that already passed MFA, otherwise a stolen
	// plain session could mint its own factor and self-issue step-up.
	// Only verified factors gate this: an abandoned (never verified) enrollment
	// cannot be completed (its secret is shown once) and is replaced below.
	hasVerified := false
	for _, f := range h.service.ListMFAFactors(principal.OrgID, principal.UserID) {
		if f.IsVerified {
			hasVerified = true
			break
		}
	}
	if hasVerified && !principal.MFAAuthenticated {
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA authentication is required to enroll an additional factor.", nil)
		return
	}
	if !hasVerified {
		if err := h.service.PurgeUnverifiedMFAFactors(principal.OrgID, principal.UserID, strings.TrimSpace(strings.ToLower(req.FactorType))); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "mfa_enroll_failed", "Could not reset the pending MFA enrollment.", err)
			return
		}
	}

	factor, err := h.service.EnrollMFA(principal.UserID, principal.Email, principal.OrgID, req.FactorType, req.Label)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_mfa_factor", err.Error(), nil)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status": "enrolled",
		"factor": factor,
		"next":   "Use POST /api/v1/auth/mfa/challenge with a valid factor challenge code to receive a step-up session token.",
	})
}

type challengeMFARequest struct {
	FactorID      string `json:"factor_id" validate:"required"`
	ChallengeCode string `json:"challenge_code" validate:"required"`
}

func (h *AuthHandlers) challengeMFA(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}

	var req challengeMFARequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	accessToken, _, _ := appmiddleware.ExtractAuthToken(r)
	session, err := h.service.ChallengeMFA(accessToken, principal, req.FactorID, req.ChallengeCode)
	if err != nil {
		status := http.StatusUnauthorized
		code := "mfa_challenge_failed"
		switch {
		case errors.Is(err, authn.ErrInvalidMFAFactor):
			status = http.StatusBadRequest
			code = "invalid_mfa_factor"
		case errors.Is(err, authn.ErrInvalidMFACode):
			status = http.StatusUnauthorized
			code = "invalid_mfa_code"
		case errors.Is(err, authn.ErrUnsupportedMFAFactor):
			status = http.StatusNotImplemented
			code = "unsupported_mfa_factor"
		case errors.Is(err, authn.ErrMFATooManyAttempts):
			status = http.StatusTooManyRequests
			code = "mfa_locked"
		}
		httpx.WriteError(w, r, status, code, "MFA challenge failed.", nil)
		return
	}

	csrfToken, ok := h.existingOrNewCSRFToken(w, r)
	if !ok {
		return
	}
	h.cookies.Set(w, session.AccessToken, session.RefreshToken, csrfToken, session.ExpiresAt)

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "verified",
		"session":    authSessionMetadata(session.ExpiresAt),
		"csrf_token": csrfToken,
	})
}

type assignRoleRequest struct {
	OrgID        string `json:"org_id" validate:"required"`
	TargetUserID string `json:"target_user_id" validate:"required"`
	Role         string `json:"role" validate:"required"`
	ScopeType    string `json:"scope_type"`
	ScopeID      string `json:"scope_id"`
	Reason       string `json:"reason" validate:"required"`
}

func (h *AuthHandlers) assignRole(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}

	if err := appmiddleware.CheckPermission(principal, appmiddleware.ActionAssignRole); err != nil {
		switch {
		case errors.Is(err, appmiddleware.ErrMFARequired):
			httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "MFA step-up is required for role assignment.", nil)
		default:
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to assign roles.", nil)
		}
		return
	}

	var req assignRoleRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.OrgID) != strings.TrimSpace(orgID) {
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Role assignment org_id must match X-Org-Id scope.", map[string]any{
			"header_org_id": orgID,
			"body_org_id":   req.OrgID,
		})
		return
	}

	if !requireAssignableRole(w, r, principal, normalizeUserRole(req.Role), "assign") {
		return
	}

	assignment, err := h.service.AssignRole(authn.RoleAssignmentInput{
		OrgID:        req.OrgID,
		TargetUserID: req.TargetUserID,
		Role:         req.Role,
		ScopeType:    req.ScopeType,
		ScopeID:      req.ScopeID,
		Reason:       req.Reason,
		ActorUserID:  principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role_assignment", err.Error(), nil)
		return
	}

	recordAudit(h.auditService, principal, orgID, audit.ActionAuthRoleAssign, "user_role", req.TargetUserID, nil, assignment, map[string]string{"role": req.Role, "reason": req.Reason})

	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":     "assigned",
		"assignment": assignment,
	})
}

func (h *AuthHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateBadRequest(w, r, dst, h.validate)
}

// refreshTokenFromCookieOrBody prefers the refresh cookie and falls back to an
// optional JSON body (some clients send the token there). It answers 400 and
// returns ok=false when a present body cannot be parsed; a missing token is "".
func refreshTokenFromCookieOrBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	if token := readRefreshToken(r); token != "" {
		return token, true
	}
	var req revokeSessionRequest
	if err := decodeOptionalJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request_body", "")
		return "", false
	}
	return req.RefreshToken, true
}

func readRefreshToken(r *http.Request) string {
	cookie, err := r.Cookie(appmiddleware.RefreshTokenCookieName)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
