package handlers

import (
	"github.com/go-chi/chi/v5"
	"net/http"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// cov4xRouter mounts routes behind the real Auth + RequireOrgScope middleware.
func cov4xRouter(tokens tokenResolver, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

// cov4xBareRouter mounts routes with no auth middleware at all.
func cov4xBareRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	register(r)
	return r
}
