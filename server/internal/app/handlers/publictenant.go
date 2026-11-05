package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/httpx"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
)

// PublicTenantHandlers resolves a public club handle (slug or verified custom
// domain) to the club's display identity. It never returns the org id: clients
// pass the slug back and the server resolves the tenant itself (tracker D6).
type PublicTenantHandlers struct {
	orgs     *orgs.Service
	settings func(slug string) (*orgsettings.OrgSettings, error)
	limiter  func(http.Handler) http.Handler
}

func NewPublicTenantHandlers(o *orgs.Service, settings func(slug string) (*orgsettings.OrgSettings, error), limiter func(http.Handler) http.Handler) *PublicTenantHandlers {
	return &PublicTenantHandlers{orgs: o, settings: settings, limiter: limiter}
}

func (h *PublicTenantHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Group(func(r chi.Router) {
		if h.limiter != nil {
			r.Use(h.limiter)
		}
		r.Get("/public/tenant", h.resolve)
	})
}

type publicTenant struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (h *PublicTenantHandlers) resolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	slug := strings.ToLower(strings.TrimSpace(q.Get("slug")))
	host := strings.TrimSpace(q.Get("host"))
	var (
		org *orgs.Org
		err error
	)
	switch {
	case host != "":
		org, err = h.orgs.ResolveDomain(r.Context(), host)
	case slug != "":
		org, err = h.bySlug(r.Context(), slug)
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "slug or host is required.", nil)
		return
	}
	if err != nil && !errors.Is(err, orgs.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not resolve club.", err)
		return
	}
	// Unknown, unverified and inactive all look identical: no existence oracle.
	if org == nil || org.Status != orgs.StatusActive || org.Slug == "" {
		httpx.WriteError(w, r, http.StatusNotFound, "club_not_found", "Club not found.", nil)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, publicTenant{Slug: org.Slug, Name: org.Name})
}

// bySlug prefers the registry and falls back to the slug clubs set in their
// settings before the registry existed.
func (h *PublicTenantHandlers) bySlug(ctx context.Context, slug string) (*orgs.Org, error) {
	return h.orgs.BySlugOrLegacy(ctx, slug, func(slug string) (string, error) {
		if h.settings == nil {
			return "", nil
		}
		st, err := h.settings(slug)
		if err != nil || st == nil || st.PublicSlug != slug {
			return "", err
		}
		return st.OrgID, nil
	})
}
