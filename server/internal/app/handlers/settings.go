package handlers

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
)

type SettingsHandlers struct {
	payload map[string]any
}

func NewSettingsHandlers(payload map[string]any) *SettingsHandlers {
	return &SettingsHandlers{payload: payload}
}

func (h *SettingsHandlers) RegisterRoutes(router chi.Router) {
	router.Route("/settings", func(r chi.Router) {
		r.Get("/runtime", h.runtimeSettings)
	})
}

func (h *SettingsHandlers) runtimeSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Runtime settings visibility requires owner or admin role.", nil)
		return
	}

	orgID := strings.TrimSpace(r.Header.Get("X-Org-Id"))
	if orgID == "" {
		orgID = strings.TrimSpace(principal.OrgID)
	}
	if strings.TrimSpace(principal.OrgID) != strings.TrimSpace(orgID) {
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Principal org_id must match X-Org-Id scope.", nil)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"settings": h.payload,
	})
}
