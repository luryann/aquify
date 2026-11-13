package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/httpx"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/offboarding"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/payments"
	"github.com/luryann/aquify/server/internal/app/provisioning"
	"github.com/luryann/aquify/server/internal/keycloak"
)

// PlatformRole is the realm role held by Aquify operators. It is deliberately
// absent from the assignable org role tiers: org admins cannot grant it.
const PlatformRole = "platform_admin"

// PlatformHandlers is the operator plane: tenant lifecycle across orgs. Access
// requires the platform role, an account that belongs to the dedicated platform
// org, and MFA. Nothing here reads another org's business data.
type PlatformHandlers struct {
	orgs          *orgs.Service
	prov          *provisioning.Provisioner
	audit         *audit.Service
	offboarding   *offboarding.Service
	platformOrgID string
	// kc, support and payments back the club support routes (platform_support.go).
	kc       keycloak.Client
	support  PlatformSupport
	payments *payments.Service
	// LookupTXT resolves DNS TXT records for domain verification (injectable in tests).
	LookupTXT func(ctx context.Context, name string) ([]string, error)
}

func NewPlatformHandlers(o *orgs.Service, p *provisioning.Provisioner, a *audit.Service, platformOrgID string) *PlatformHandlers {
	return &PlatformHandlers{orgs: o, prov: p, audit: a, platformOrgID: strings.TrimSpace(platformOrgID),
		LookupTXT: func(ctx context.Context, name string) ([]string, error) {
			return net.DefaultResolver.LookupTXT(ctx, name)
		}}
}

// SetOffboarding enables org export and permanent deletion (needs a database).
func (h *PlatformHandlers) SetOffboarding(s *offboarding.Service) { h.offboarding = s }

// DomainChallengeName / DomainChallengeValue define the DNS proof of ownership.
func DomainChallengeName(domain string) string { return "_aquify-verify." + domain }
func DomainChallengeValue(orgID string) string { return "aquify-verify=" + orgID }

func (h *PlatformHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/platform", func(p chi.Router) {
		p.Use(h.requireOperator)
		p.Get("/orgs", h.list)
		p.Post("/orgs", h.create)
		p.Get("/orgs/{id}", h.get)
		p.Patch("/orgs/{id}", h.updateOrg)
		p.Get("/orgs/{id}/overview", h.overview)
		p.Get("/orgs/{id}/activity", h.activity)
		p.Get("/orgs/{id}/users", h.listClubUsers)
		p.Post("/orgs/{id}/admins", h.addAdmin)
		p.Post("/orgs/{id}/users/{userID}/password-reset", h.passwordReset())
		p.Post("/orgs/{id}/users/{userID}/resend-verification", h.resendVerification())
		p.Post("/orgs/{id}/users/{userID}/sign-out", h.signOutUser())
		p.Post("/orgs/{id}/users/{userID}/reset-mfa", h.resetMFA())
		p.Post("/orgs/{id}/users/{userID}/disable", h.disableUser())
		p.Post("/orgs/{id}/users/{userID}/enable", h.enableUser())
		p.Post("/orgs/{id}/suspend", h.suspend)
		p.Post("/orgs/{id}/resume", h.resume)
		p.Post("/orgs/{id}/offboard", h.offboard)
		p.Post("/orgs/{id}/cancel-offboarding", h.cancelOffboarding)
		p.Post("/orgs/{id}/export", h.exportOrg)
		p.Post("/orgs/{id}/purge", h.purge)
		p.Post("/orgs/{id}/restore", h.restore)
		p.Post("/orgs/{id}/domains", h.addDomain)
		p.Post("/orgs/{id}/domains/{domain}/verify", h.verifyDomain)
	})
}

func (h *PlatformHandlers) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr, ok := appmiddleware.PrincipalFromContext(r.Context())
		if !ok || pr == nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
			return
		}
		if h.platformOrgID == "" || pr.OrgID != h.platformOrgID || !slices.Contains(pr.Roles, PlatformRole) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to access this resource.", nil)
			return
		}
		if !pr.MFAAuthenticated {
			httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "Multi-factor authentication is required for platform operations.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *PlatformHandlers) principal(r *http.Request) *appmiddleware.Principal {
	pr, _ := appmiddleware.PrincipalFromContext(r.Context())
	return pr
}

// operatorOnlyActions are lifecycle events that must not be written into the
// club's own audit log: after a purge the club's tenant data is gone (a new row
// would resurrect it), and a restore replaces that data wholesale.
var operatorOnlyActions = map[string]bool{
	audit.ActionPlatformOrgPurge:   true,
	audit.ActionPlatformOrgRestore: true,
}

func (h *PlatformHandlers) record(r *http.Request, action, orgID string, after any) {
	if operatorOnlyActions[action] {
		h.recordOperator(r, action, orgID, "org", orgID, nil, after)
		return
	}
	h.recordSupport(r, action, orgID, "org", orgID, nil, after)
}

func (h *PlatformHandlers) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.orgs.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not list organizations.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": list, "count": len(list)})
}

func (h *PlatformHandlers) get(w http.ResponseWriter, r *http.Request) {
	o, err := h.orgs.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, orgs.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load organization.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, o)
}

type createOrgRequest struct {
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Plan           string `json:"plan"`
	Region         string `json:"region"`
	TeamCode       string `json:"team_code"`
	AdminEmail     string `json:"admin_email"`
	AdminFirstName string `json:"admin_first_name"`
	AdminLastName  string `json:"admin_last_name"`
}

func (h *PlatformHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	res, err := h.prov.Provision(r.Context(), provisioning.Input{
		Name: req.Name, Slug: req.Slug, Plan: req.Plan, Region: req.Region, TeamCode: req.TeamCode,
		AdminEmail: req.AdminEmail, AdminFirstName: req.AdminFirstName, AdminLastName: req.AdminLastName,
		ActorUserID: h.principal(r).UserID,
	})
	switch {
	case err == nil:
		h.record(r, audit.ActionPlatformOrgCreate, res.Org.ID, res.Org)
		status := http.StatusCreated
		if res.Resumed {
			status = http.StatusOK
		}
		_ = httpx.WriteJSON(w, status, res)
	case errors.Is(err, orgs.ErrSlugTaken), errors.Is(err, provisioning.ErrAlreadyActive):
		httpx.WriteError(w, r, http.StatusConflict, "slug_taken", "An organization with this slug already exists.", nil)
	case errors.Is(err, provisioning.ErrAdminEmailInUse):
		httpx.WriteError(w, r, http.StatusConflict, "admin_email_in_use", "That admin email already belongs to a user in another organization.", nil)
	case errors.Is(err, orgs.ErrInvalidSlug):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_slug", err.Error(), nil)
	case res != nil && res.Org != nil:
		// Partially provisioned: report the org so the operator can retry the same request.
		httpx.WriteError(w, r, http.StatusBadGateway, "provisioning_incomplete", "Provisioning did not finish. Repeat the request to resume.", err)
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
	}
}

func (h *PlatformHandlers) transition(w http.ResponseWriter, r *http.Request, to, action string) {
	id := chi.URLParam(r, "id")
	if h.platformOrgID != "" && orgs.CanonicalID(id) == orgs.CanonicalID(h.platformOrgID) && to != orgs.StatusActive {
		httpx.WriteError(w, r, http.StatusConflict, "platform_org_protected", "The platform organization cannot be suspended.", nil)
		return
	}
	o, err := h.orgs.SetStatus(r.Context(), id, to)
	switch {
	case errors.Is(err, orgs.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
	case errors.Is(err, orgs.ErrInvalidStatus):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not update organization.", err)
	default:
		h.record(r, action, o.ID, o)
		_ = httpx.WriteJSON(w, http.StatusOK, o)
	}
}

func (h *PlatformHandlers) offboard(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, orgs.StatusOffboarding, audit.ActionPlatformOrgOffboard)
}

func (h *PlatformHandlers) cancelOffboarding(w http.ResponseWriter, r *http.Request) {
	// Back to suspended: access stays off until an operator deliberately resumes the org.
	id := chi.URLParam(r, "id")
	o, err := h.orgs.Get(r.Context(), id)
	if err == nil && o.Status != orgs.StatusOffboarding {
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "The organization is not being offboarded.", nil)
		return
	}
	h.transition(w, r, orgs.StatusSuspended, audit.ActionPlatformOrgOffboardCancel)
}

// offboardingConfigured answers 501 when the deployment has no database-backed offboarding.
func (h *PlatformHandlers) offboardingConfigured(w http.ResponseWriter, r *http.Request) bool {
	if h.offboarding != nil {
		return true
	}
	httpx.WriteError(w, r, http.StatusNotImplemented, "offboarding_unavailable", "Offboarding is not configured.", nil)
	return false
}

func (h *PlatformHandlers) offboardingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgs.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
	case errors.Is(err, offboarding.ErrNotOffboarding), errors.Is(err, offboarding.ErrCoolingOff), errors.Is(err, offboarding.ErrProtectedOrg):
		httpx.WriteError(w, r, http.StatusConflict, "offboarding_precondition", err.Error(), nil)
	case errors.Is(err, orgs.ErrInvalidStatus):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "The organization changed state; retry after checking its status.", nil)
	case errors.Is(err, offboarding.ErrConfirmMismatch):
		httpx.WriteError(w, r, http.StatusBadRequest, "confirmation_mismatch", err.Error(), nil)
	case errors.Is(err, offboarding.ErrNoDatabase), errors.Is(err, offboarding.ErrStorageUnsupport):
		httpx.WriteError(w, r, http.StatusNotImplemented, "offboarding_unavailable", err.Error(), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Offboarding step failed.", err)
	}
}

func (h *PlatformHandlers) exportOrg(w http.ResponseWriter, r *http.Request) {
	if !h.offboardingConfigured(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := h.offboarding.Export(r.Context(), id)
	if err != nil {
		h.offboardingError(w, r, err)
		return
	}
	h.record(r, audit.ActionPlatformOrgExport, id, map[string]any{"key": res.Key, "tables": len(res.Tables), "files": res.Files})
	url, _ := h.offboarding.Store.PresignedGetURL(res.Key, 24*time.Hour)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"export": res, "download_url": url})
}

type purgeRequest struct {
	ConfirmSlug string `json:"confirm_slug"`
}

func (h *PlatformHandlers) purge(w http.ResponseWriter, r *http.Request) {
	if !h.offboardingConfigured(w, r) {
		return
	}
	var req purgeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	id := chi.URLParam(r, "id")
	res, err := h.offboarding.Purge(r.Context(), id, req.ConfirmSlug)
	if err != nil {
		h.offboardingError(w, r, err)
		return
	}
	h.record(r, audit.ActionPlatformOrgPurge, id, res)
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

type restoreRequest struct {
	ExportKey string `json:"export_key"`
}

// restore loads an earlier export of this org back into the database (single-org recovery).
func (h *PlatformHandlers) restore(w http.ResponseWriter, r *http.Request) {
	if !h.offboardingConfigured(w, r) {
		return
	}
	var req restoreRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	id := chi.URLParam(r, "id")
	// Only an archive stored under this org's own offboarding prefix can be restored into it.
	if err := validateObjectKeyUnderPrefix(offboarding.ExportKeyPrefix(id), req.ExportKey); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_export_key", "export_key must be an offboarding export of this organization.", nil)
		return
	}
	data, _, err := h.offboarding.Store.GetObject(r.Context(), req.ExportKey)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "export_not_found", "That export was not found.", nil)
		return
	}
	res, err := h.offboarding.Restore(r.Context(), id, data)
	if errors.Is(err, offboarding.ErrRestoreOrgMissing) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	if err != nil {
		h.offboardingError(w, r, err)
		return
	}
	h.record(r, audit.ActionPlatformOrgRestore, id, res)
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *PlatformHandlers) suspend(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, orgs.StatusSuspended, audit.ActionPlatformOrgSuspend)
}

func (h *PlatformHandlers) resume(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, orgs.StatusActive, audit.ActionPlatformOrgResume)
}

type addDomainRequest struct {
	Domain string `json:"domain"`
}

func (h *PlatformHandlers) addDomain(w http.ResponseWriter, r *http.Request) {
	var req addDomainRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	id := chi.URLParam(r, "id")
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	switch err := h.orgs.AddDomain(r.Context(), id, domain); {
	case errors.Is(err, orgs.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Organization not found.", nil)
	case errors.Is(err, orgs.ErrInvalidDomain):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_domain", err.Error(), nil)
	case errors.Is(err, orgs.ErrDomainTaken):
		httpx.WriteError(w, r, http.StatusConflict, "domain_taken", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not add domain.", err)
	default:
		h.record(r, audit.ActionPlatformOrgDomainAdd, id, map[string]string{"domain": domain})
		_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"domain": domain, "verified": false,
			"challenge": map[string]string{"type": "TXT", "name": DomainChallengeName(domain), "value": DomainChallengeValue(id)},
		})
	}
}

func (h *PlatformHandlers) verifyDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	domain := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "domain")))
	records, err := h.LookupTXT(r.Context(), DomainChallengeName(domain))
	if err != nil && len(records) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "challenge_not_found", "The DNS TXT record was not found yet.", nil)
		return
	}
	want := DomainChallengeValue(id)
	if !slices.ContainsFunc(records, func(v string) bool { return strings.TrimSpace(v) == want }) {
		httpx.WriteError(w, r, http.StatusConflict, "challenge_mismatch", "The DNS TXT record does not match.", nil)
		return
	}
	switch err := h.orgs.VerifyDomain(r.Context(), id, domain); {
	case errors.Is(err, orgs.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Domain not found for this organization.", nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not verify domain.", err)
	default:
		h.record(r, audit.ActionPlatformOrgDomainVerify, id, map[string]string{"domain": domain})
		_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"domain": domain, "verified": true})
	}
}
