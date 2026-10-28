package handlers

import (
	"errors"
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/go-chi/chi/v5"
)

// OrgSettingsHandlers exposes read/write for org-level export identity
// (team code, team name, LSC code) used to populate USA-Swimming meet reports.
type OrgSettingsHandlers struct {
	service      *orgsettings.Service
	auditService *audit.Service
}

// NewOrgSettingsHandlers returns a new OrgSettingsHandlers.
func NewOrgSettingsHandlers(svc *orgsettings.Service, auditSvc *audit.Service) *OrgSettingsHandlers {
	return &OrgSettingsHandlers{service: svc, auditService: auditSvc}
}

// RegisterRoutes mounts org settings routes on the protected router.
func (h *OrgSettingsHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/org-settings", h.get)
	router.Put("/org-settings", h.update)
}

func (h *OrgSettingsHandlers) get(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	// Read access is broad (coach needs team identity for reports UI);
	// writes are admin only.
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}

	settings, err := h.service.Get(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load org settings.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"org_settings": settings})
}

type updateOrgSettingsBody struct {
	TeamCode           *string `json:"team_code"`
	TeamName           *string `json:"team_name"`
	LSCCode            *string `json:"lsc_code"`
	EmailSenderName    *string `json:"email_sender_name"`
	EmailSenderAddress *string `json:"email_sender_address"`
	PublicSlug         *string `json:"public_slug"`
	PublicSiteEnabled  *bool   `json:"public_site_enabled"`
	PublicShowCoaches  *bool   `json:"public_show_coaches"`
	PublicAbout        *string `json:"public_about"`
	PublicContactEmail *string `json:"public_contact_email"`
	PublicContactPhone *string `json:"public_contact_phone"`
}

func (h *OrgSettingsHandlers) update(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin may update org settings.", nil)
		return
	}

	var body updateOrgSettingsBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}

	settings, err := h.service.Update(orgID, orgsettings.Updates{
		TeamCode:           body.TeamCode,
		TeamName:           body.TeamName,
		LSCCode:            body.LSCCode,
		EmailSenderName:    body.EmailSenderName,
		EmailSenderAddress: body.EmailSenderAddress,
		PublicSlug:         body.PublicSlug,
		PublicSiteEnabled:  body.PublicSiteEnabled,
		PublicShowCoaches:  body.PublicShowCoaches,
		PublicAbout:        body.PublicAbout,
		PublicContactEmail: body.PublicContactEmail,
		PublicContactPhone: body.PublicContactPhone,
	}, principal.UserID)
	if errors.Is(err, orgsettings.ErrInvalidSlug) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_slug", err.Error(), nil)
		return
	}
	if errors.Is(err, orgsettings.ErrSlugTaken) {
		httpx.WriteError(w, r, http.StatusConflict, "slug_taken", err.Error(), nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update org settings.", err)
		return
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "org_settings.update",
			ResourceType: "org_settings",
			ResourceID:   orgID,
			After:        settings,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"org_settings": settings})
}
