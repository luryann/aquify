package handlers

import (
	"context"
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

type ReadinessReport struct {
	Ready  bool
	Checks map[string]string
}

type ReadinessChecker interface {
	CheckReadiness(ctx context.Context) ReadinessReport
}

func RegisterHealthRoutes(router chi.Router, checker ReadinessChecker) {
	router.Get("/live", live)
	router.Get("/ready", ready(checker))
}

func live(w http.ResponseWriter, _ *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func ready(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := ReadinessReport{
			Ready: false,
			Checks: map[string]string{
				"server": "not_ready: readiness checker not configured",
			},
		}
		if checker != nil {
			report = checker.CheckReadiness(r.Context())
		}

		statusCode := http.StatusOK
		status := "ready"
		if !report.Ready {
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
		}

		_ = httpx.WriteJSON(w, statusCode, readinessResponse{
			Status:    status,
			Checks:    report.Checks,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}
