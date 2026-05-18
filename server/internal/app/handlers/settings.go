package handlers

import (
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
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
	principal, _, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Runtime settings visibility requires owner or admin role.", nil)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"settings": h.payload,
	})
}
