package handlers

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/go-chi/chi/v5"
)

// JobsHandlers exposes admin HTTP endpoints for background job visibility.
type JobsHandlers struct {
	server workers.Backend
}

// NewJobsHandlers returns a new JobsHandlers backed by the given worker Server.
func NewJobsHandlers(srv workers.Backend) *JobsHandlers {
	return &JobsHandlers{server: srv}
}

// RegisterRoutes mounts all jobs admin routes.
// All routes require admin role.
func (h *JobsHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/jobs", h.listJobs)
	router.Get("/jobs/queues", h.queueStats)
	router.Get("/jobs/metrics", h.jobMetrics)
	router.Get("/jobs/dlq", h.listDLQ)
	router.Post("/jobs/dlq/{taskID}/retry", h.retryDLQTask)
	router.Delete("/jobs/dlq/{taskID}", h.deleteDLQTask)
}

// listJobs handles GET /api/v1/jobs
// Query params: queue (optional), status (optional)
func (h *JobsHandlers) listJobs(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Job visibility requires admin role.", nil)
		return
	}

	queue := strings.TrimSpace(r.URL.Query().Get("queue"))
	statusRaw := strings.TrimSpace(r.URL.Query().Get("status"))
	var status workers.TaskStatus
	if statusRaw != "" {
		status = workers.TaskStatus(statusRaw)
	}

	tasks, err := h.server.ListTasks(queue, status)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load jobs.", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// queueStats handles GET /api/v1/jobs/queues
func (h *JobsHandlers) queueStats(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Queue stats requires admin role.", nil)
		return
	}

	queues, err := h.server.QueueStats()
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load queue stats.", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"queues": queues,
	})
}

// jobMetrics handles GET /api/v1/jobs/metrics
func (h *JobsHandlers) jobMetrics(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Job metrics requires admin role.", nil)
		return
	}

	m := h.server.Metrics()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"by_type":            m.TypeSnapshot(),
		"recent_executions":  m.RecentExecutions(50),
	})
}

// listDLQ handles GET /api/v1/jobs/dlq
func (h *JobsHandlers) listDLQ(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "DLQ access requires admin role.", nil)
		return
	}

	tasks, err := h.server.DLQTasks()
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load dead-letter queue.", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"dead_letter_tasks": tasks,
		"count":             len(tasks),
	})
}

// retryDLQTask handles POST /api/v1/jobs/dlq/{taskID}/retry
func (h *JobsHandlers) retryDLQTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "DLQ retry requires admin role.", nil)
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if strings.TrimSpace(taskID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_param", "taskID is required.", nil)
		return
	}

	if err := h.server.RetryDLQTask(r.Context(), taskID); err != nil {
		if err == workers.ErrTaskNotFound {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Task not found in DLQ.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to retry task.", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "task re-enqueued for processing",
		"task_id": taskID,
	})
}

// deleteDLQTask handles DELETE /api/v1/jobs/dlq/{taskID}
func (h *JobsHandlers) deleteDLQTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "DLQ delete requires admin role.", nil)
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if strings.TrimSpace(taskID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_param", "taskID is required.", nil)
		return
	}

	if err := h.server.DeleteDLQTask(taskID); err != nil {
		if err == workers.ErrTaskNotFound {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Task not found in DLQ.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to delete task.", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
