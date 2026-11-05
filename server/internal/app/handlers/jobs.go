package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/httpx"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/workers"
	"github.com/redis/go-redis/v9"
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

// isQueueUnavailable reports whether err is a connectivity failure talking to
// the queue backend (Redis down, refused, reset, timed out, pool exhausted)
// rather than an application error. It matches on error types and sentinels,
// never on message text, so e.g. a payload decode "unexpected EOF" stays a 500.
func isQueueUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, redis.ErrClosed) || errors.Is(err, redis.ErrPoolTimeout) || errors.Is(err, redis.ErrPoolExhausted) {
		return true
	}
	for _, errno := range []syscall.Errno{syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE, syscall.EHOSTUNREACH, syscall.ENETUNREACH} {
		if errors.Is(err, errno) {
			return true
		}
	}
	// Dial/read/write failures on the redis socket, and DNS failures.
	var opErr *net.OpError
	var dnsErr *net.DNSError
	if errors.As(err, &opErr) || errors.As(err, &dnsErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// go-redis surfaces a connection dropped mid-reply as a bare io.EOF.
	// Decode errors that happen to wrap io.EOF are application errors.
	if errors.Is(err, io.EOF) {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		return !errors.As(err, &syntaxErr) && !errors.As(err, &typeErr)
	}
	return false
}

// writeQueueError maps a queue backend error to 503 queue_unavailable for
// connectivity failures and 500 internal_error otherwise.
func writeQueueError(w http.ResponseWriter, r *http.Request, err error, message string) {
	if isQueueUnavailable(err) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "queue_unavailable", "The job queue backend is unavailable. Try again shortly.", err)
		return
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", message, err)
}

// requireAdminOrg is requireAdmin plus the validated org scope of the request.
func requireAdminOrg(w http.ResponseWriter, r *http.Request, forbiddenMessage string) (string, bool) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return "", false
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", forbiddenMessage, nil)
		return "", false
	}
	return orgID, true
}

// filterTasksByOrg keeps only tasks stamped with orgID. Tasks without org
// metadata (global/system jobs) are never shown to a tenant admin.
func filterTasksByOrg(tasks []workers.TaskInfo, orgID string) []workers.TaskInfo {
	out := make([]workers.TaskInfo, 0, len(tasks))
	for _, t := range tasks {
		if t.OrgID != "" && t.OrgID == orgID {
			out = append(out, t)
		}
	}
	return out
}

// dlqTaskInOrg writes 404 (never revealing other orgs' tasks) unless taskID is
// a dead-letter task owned by orgID.
func (h *JobsHandlers) dlqTaskInOrg(w http.ResponseWriter, r *http.Request, taskID, orgID string) bool {
	tasks, err := h.server.DLQTasks()
	if err != nil {
		writeQueueError(w, r, err, "Failed to load dead-letter queue.")
		return false
	}
	for _, t := range tasks {
		if t.ID == taskID && t.OrgID != "" && t.OrgID == orgID {
			return true
		}
	}
	httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Task not found in DLQ.", nil)
	return false
}

// listJobs handles GET /api/v1/jobs
// Query params: queue (optional), status (optional)
func (h *JobsHandlers) listJobs(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "Job visibility requires admin role.")
	if !ok {
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
		writeQueueError(w, r, err, "Failed to load jobs.")
		return
	}
	tasks = filterTasksByOrg(tasks, orgID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// isPlatformOperator reports whether the principal holds the operator role.
// The role is only honoured for the platform org elsewhere; here it merely
// widens the job views from the caller's own org to the whole fleet.
func isPlatformOperator(r *http.Request) bool {
	pr, ok := appmiddleware.PrincipalFromContext(r.Context())
	return ok && pr != nil && slices.Contains(pr.Roles, PlatformRole) && pr.MFAAuthenticated
}

// orgTasks returns every task (including dead-lettered ones) owned by orgID.
func (h *JobsHandlers) orgTasks(orgID string) ([]workers.TaskInfo, error) {
	tasks, err := h.server.ListTasks("", "")
	if err != nil {
		return nil, err
	}
	dlq, err := h.server.DLQTasks()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		seen[t.ID] = true
	}
	for _, d := range dlq {
		if !seen[d.ID] {
			tasks = append(tasks, d)
		}
	}
	return filterTasksByOrg(tasks, orgID), nil
}

// queueStats handles GET /api/v1/jobs/queues. Platform operators see fleet-wide
// depth; a club admin sees counts derived only from that club's own tasks.
func (h *JobsHandlers) queueStats(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "Queue stats requires admin role.")
	if !ok {
		return
	}
	if isPlatformOperator(r) {
		queues, err := h.server.QueueStats()
		if err != nil {
			writeQueueError(w, r, err, "Failed to load queue stats.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"queues": queues})
		return
	}
	tasks, err := h.orgTasks(orgID)
	if err != nil {
		writeQueueError(w, r, err, "Failed to load queue stats.")
		return
	}
	byQueue := map[string]*workers.QueueStats{}
	// Seed every configured queue (names only) so the club sees zero rows, not gaps.
	if all, err := h.server.QueueStats(); err == nil {
		for _, q := range all {
			byQueue[q.Name] = &workers.QueueStats{Name: q.Name}
		}
	}
	for _, t := range tasks {
		q := byQueue[t.Queue]
		if q == nil {
			q = &workers.QueueStats{Name: t.Queue}
			byQueue[t.Queue] = q
		}
		switch t.Status {
		case workers.StatusPending, workers.StatusRetrying:
			q.Pending++
		case workers.StatusActive:
			q.Active++
		case workers.StatusCompleted:
			q.Completed++
		case workers.StatusFailed:
			q.Failed++
		case workers.StatusDeadLetter:
			q.DeadLetter++
		}
	}
	queues := make([]workers.QueueStats, 0, len(byQueue))
	for _, q := range byQueue {
		queues = append(queues, *q)
	}
	sort.Slice(queues, func(i, j int) bool { return queues[i].Name < queues[j].Name })
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"queues": queues})
}

// jobMetrics handles GET /api/v1/jobs/metrics. The execution history is not
// stamped with an org, so a club admin gets counters and recent runs derived
// from that club's own tasks; only platform operators see the global history.
func (h *JobsHandlers) jobMetrics(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "Job metrics requires admin role.")
	if !ok {
		return
	}
	if isPlatformOperator(r) {
		m := h.server.Metrics()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"by_type":           m.TypeSnapshot(),
			"recent_executions": m.RecentExecutions(50),
		})
		return
	}
	tasks, err := h.orgTasks(orgID)
	if err != nil {
		writeQueueError(w, r, err, "Failed to load job metrics.")
		return
	}
	byType := map[string]workers.TypeMetricSnapshot{}
	execs := make([]workers.JobExecution, 0)
	for _, t := range tasks {
		c := byType[t.Type]
		c.Enqueued++
		switch t.Status {
		case workers.StatusCompleted:
			c.Succeeded++
		case workers.StatusFailed, workers.StatusRetrying:
			c.Failed++
		case workers.StatusDeadLetter:
			c.Failed++
			c.DLQ++
		}
		byType[t.Type] = c
		var finished *time.Time
		if t.CompletedAt != nil {
			finished = t.CompletedAt
		} else if t.LastFailedAt != nil {
			finished = t.LastFailedAt
		}
		if finished != nil {
			execs = append(execs, workers.JobExecution{
				TaskID: t.ID, Type: t.Type, Queue: t.Queue, Status: t.Status,
				StartedAt: *finished, FinishedAt: *finished, Err: t.LastErr, Retried: t.Retried,
			})
		}
	}
	sort.Slice(execs, func(i, j int) bool { return execs[i].FinishedAt.After(execs[j].FinishedAt) })
	if len(execs) > 50 {
		execs = execs[:50]
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"by_type": byType, "recent_executions": execs})
}

// listDLQ handles GET /api/v1/jobs/dlq
func (h *JobsHandlers) listDLQ(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "DLQ access requires admin role.")
	if !ok {
		return
	}

	tasks, err := h.server.DLQTasks()
	if err != nil {
		writeQueueError(w, r, err, "Failed to load dead-letter queue.")
		return
	}
	tasks = filterTasksByOrg(tasks, orgID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"dead_letter_tasks": tasks,
		"count":             len(tasks),
	})
}

// retryDLQTask handles POST /api/v1/jobs/dlq/{taskID}/retry
func (h *JobsHandlers) retryDLQTask(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "DLQ retry requires admin role.")
	if !ok {
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if strings.TrimSpace(taskID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_param", "taskID is required.", nil)
		return
	}
	if !h.dlqTaskInOrg(w, r, taskID, orgID) {
		return
	}

	if err := h.server.RetryDLQTask(r.Context(), taskID); err != nil {
		if errors.Is(err, workers.ErrTaskNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Task not found in DLQ.", nil)
			return
		}
		writeQueueError(w, r, err, "Failed to retry task.")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "task re-enqueued for processing",
		"task_id": taskID,
	})
}

// deleteDLQTask handles DELETE /api/v1/jobs/dlq/{taskID}
func (h *JobsHandlers) deleteDLQTask(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireAdminOrg(w, r, "DLQ delete requires admin role.")
	if !ok {
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if strings.TrimSpace(taskID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_param", "taskID is required.", nil)
		return
	}
	if !h.dlqTaskInOrg(w, r, taskID, orgID) {
		return
	}

	if err := h.server.DeleteDLQTask(taskID); err != nil {
		if errors.Is(err, workers.ErrTaskNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Task not found in DLQ.", nil)
			return
		}
		writeQueueError(w, r, err, "Failed to delete task.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
