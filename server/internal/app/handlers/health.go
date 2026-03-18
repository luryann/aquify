package handlers

import (
	"net/http"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

type readinessResponse struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks"`
	Timestamp string            `json:"timestamp"`
}

func RegisterHealthRoutes(router chi.Router) {
	router.Get("/live", live)
	router.Get("/ready", ready)
}

func live(w http.ResponseWriter, _ *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func ready(w http.ResponseWriter, _ *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, readinessResponse{
		Status: "ready",
		Checks: map[string]string{
			"database": "not-configured",
			"redis":    "not-configured",
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
