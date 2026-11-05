package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/httpx"
	"github.com/luryann/aquify/server/internal/app/invites"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// InviteHandlers lets an org's admins issue and revoke sign-up invite codes.
type InviteHandlers struct {
	svc   *invites.Service
	audit *audit.Service
}

func NewInviteHandlers(svc *invites.Service, a *audit.Service) *InviteHandlers {
	return &InviteHandlers{svc: svc, audit: a}
}

func (h *InviteHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/invites", func(inv chi.Router) {
		admin := inv.With(appmiddleware.RBACStub("admin"))
		admin.Get("/", h.list)
		admin.Post("/", h.create)
		admin.Delete("/{id}", h.revoke)
	})
}

type createInviteRequest struct {
	Label   string `json:"label"`
	Role    string `json:"role"`
	MaxUses int    `json:"max_uses"`
	TTLDays int    `json:"ttl_days"`
}

func (h *InviteHandlers) create(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var req createInviteRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	inv, code, err := h.svc.Create(r.Context(), orgID, principal.UserID, invites.CreateInput{
		Label: req.Label, Role: strings.ToLower(strings.TrimSpace(req.Role)), MaxUses: req.MaxUses,
		TTL: time.Duration(req.TTLDays) * 24 * time.Hour,
	})
	switch {
	case errors.Is(err, invites.ErrInvalidRole):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_role", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
	default:
		_ = recordAudit(h.audit, principal, orgID, audit.ActionOrgInviteCreate, "org_invite", inv.ID, nil, inv, map[string]string{"role": inv.Role})
		// The plaintext code is returned exactly once.
		_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"invite": inv, "code": code})
	}
}

func (h *InviteHandlers) list(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	list, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not list invites.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": list, "count": len(list)})
}

func (h *InviteHandlers) revoke(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	switch err := h.svc.Revoke(r.Context(), orgID, id); {
	case errors.Is(err, invites.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Invite not found.", nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not revoke invite.", err)
	default:
		_ = recordAudit(h.audit, principal, orgID, audit.ActionOrgInviteRevoke, "org_invite", id, nil, nil, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
