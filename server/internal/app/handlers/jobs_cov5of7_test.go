package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/workers"
	"github.com/redis/go-redis/v9"
)

const cov5xJobsOrg = "org-cov5x-jobs"

// cov5xBackend is a scripted workers.Backend. The queue is a true external
// boundary (Redis), so failures are injected here; the handler under test is real.
type cov5xBackend struct {
	workers.Backend // unimplemented methods panic, which would fail the test loudly

	tasks    []workers.TaskInfo
	dlq      []workers.TaskInfo
	stats    []workers.QueueStats
	listErr  error
	dlqErr   error
	statsErr error
	retryErr error
	delErr   error

	listQueue  string
	listStatus workers.TaskStatus
	retried    []string
	deleted    []string
	metrics    workers.MetricsReader
}

func (b *cov5xBackend) ListTasks(queue string, status workers.TaskStatus) ([]workers.TaskInfo, error) {
	b.listQueue, b.listStatus = queue, status
	return b.tasks, b.listErr
}
func (b *cov5xBackend) DLQTasks() ([]workers.TaskInfo, error)     { return b.dlq, b.dlqErr }
func (b *cov5xBackend) QueueStats() ([]workers.QueueStats, error) { return b.stats, b.statsErr }
func (b *cov5xBackend) RetryDLQTask(_ context.Context, id string) error {
	b.retried = append(b.retried, id)
	return b.retryErr
}
func (b *cov5xBackend) DeleteDLQTask(id string) error {
	b.deleted = append(b.deleted, id)
	return b.delErr
}
func (b *cov5xBackend) Metrics() workers.MetricsReader { return b.metrics }

type cov5xMetrics struct{}

func (cov5xMetrics) TypeSnapshot() map[string]workers.TypeMetricSnapshot {
	return map[string]workers.TypeMetricSnapshot{"global:type": {Enqueued: 9, Succeeded: 8, Failed: 1}}
}
func (cov5xMetrics) RecentExecutions(limit int) []workers.JobExecution {
	return []workers.JobExecution{{TaskID: fmt.Sprintf("limit-%d", limit), Type: "global:type"}}
}

func cov5xJobsRouter(b workers.Backend, principals tokenResolver) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(principals))
		p.Use(appmiddleware.RequireOrgScope)
		NewJobsHandlers(b).RegisterRoutes(p)
	})
	return r
}

func cov5xJobsTokens() tokenResolver {
	return tokenResolver{
		"admin":          {UserID: "u1", OrgID: cov5xJobsOrg, Roles: []string{"admin"}},
		"coach":          {UserID: "u2", OrgID: cov5xJobsOrg, Roles: []string{"coach"}},
		"operator":       {UserID: "u3", OrgID: cov5xJobsOrg, Roles: []string{"admin", PlatformRole}, MFAAuthenticated: true},
		"operator-nomfa": {UserID: "u4", OrgID: cov5xJobsOrg, Roles: []string{"admin", PlatformRole}},
	}
}

func cov5xJobsDo(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov5xJobsOrg)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

var cov5xJobRoutes = []struct{ method, path string }{
	{"GET", "/jobs"}, {"GET", "/jobs/queues"}, {"GET", "/jobs/metrics"}, {"GET", "/jobs/dlq"},
	{"POST", "/jobs/dlq/t1/retry"}, {"DELETE", "/jobs/dlq/t1"},
}

func TestCov5xJobsRoutesEnforceAdminAndScope(t *testing.T) {
	t.Parallel()
	b := &cov5xBackend{}
	router := cov5xJobsRouter(b, cov5xJobsTokens())
	for _, rt := range cov5xJobRoutes {
		label := rt.method + " " + rt.path
		cov5xWant(t, label+" coach", cov5xJobsDo(router, rt.method, rt.path, "coach"), http.StatusForbidden, "requires admin role")
		cov5xWant(t, label+" anonymous", cov5xJobsDo(router, rt.method, rt.path, ""), http.StatusUnauthorized, "")
	}
	if len(b.retried)+len(b.deleted) != 0 {
		t.Fatalf("a refused request must not mutate the queue: %v %v", b.retried, b.deleted)
	}

	// Authenticated but without the org scope middleware: the handler itself refuses.
	h := NewJobsHandlers(b)
	bare := chi.NewRouter()
	bare.Use(appmiddleware.Auth(cov5xJobsTokens()))
	h.RegisterRoutes(bare)
	for _, rt := range cov5xJobRoutes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		req.Header.Set("Authorization", "Bearer admin")
		rr := httptest.NewRecorder()
		bare.ServeHTTP(rr, req)
		cov5xWant(t, rt.method+" "+rt.path+" no org", rr, http.StatusBadRequest, "missing_org_scope")
	}
}

func TestCov5xJobsBlankTaskIDIsRejected(t *testing.T) {
	t.Parallel()
	b := &cov5xBackend{}
	router := cov5xJobsRouter(b, cov5xJobsTokens())
	cov5xWant(t, "retry blank", cov5xJobsDo(router, "POST", "/jobs/dlq/%20/retry", "admin"), http.StatusBadRequest, "missing_param")
	cov5xWant(t, "delete blank", cov5xJobsDo(router, "DELETE", "/jobs/dlq/%20", "admin"), http.StatusBadRequest, "missing_param")
	if len(b.retried)+len(b.deleted) != 0 {
		t.Fatal("blank ids must never reach the backend")
	}
}

func TestCov5xJobsBackendFailuresMapToQueueErrors(t *testing.T) {
	t.Parallel()
	down := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	broken := errors.New("corrupt payload")
	own := workers.TaskInfo{ID: "t1", Type: "x", Queue: "default", Status: workers.StatusDeadLetter, OrgID: cov5xJobsOrg}

	cases := []struct {
		name   string
		b      *cov5xBackend
		method string
		path   string
		token  string
	}{
		{"list", &cov5xBackend{listErr: down}, "GET", "/jobs", "admin"},
		{"dlq list", &cov5xBackend{dlqErr: down}, "GET", "/jobs/dlq", "admin"},
		{"operator queues", &cov5xBackend{statsErr: down}, "GET", "/jobs/queues", "operator"},
		{"club queues list", &cov5xBackend{listErr: down}, "GET", "/jobs/queues", "admin"},
		{"club queues dlq", &cov5xBackend{dlqErr: down}, "GET", "/jobs/queues", "admin"},
		{"club metrics list", &cov5xBackend{listErr: down}, "GET", "/jobs/metrics", "admin"},
		{"club metrics dlq", &cov5xBackend{dlqErr: down}, "GET", "/jobs/metrics", "admin"},
		{"retry ownership lookup", &cov5xBackend{dlqErr: down}, "POST", "/jobs/dlq/t1/retry", "admin"},
		{"retry", &cov5xBackend{dlq: []workers.TaskInfo{own}, retryErr: down}, "POST", "/jobs/dlq/t1/retry", "admin"},
		{"delete", &cov5xBackend{dlq: []workers.TaskInfo{own}, delErr: down}, "DELETE", "/jobs/dlq/t1", "admin"},
	}
	for _, tc := range cases {
		rr := cov5xJobsDo(cov5xJobsRouter(tc.b, cov5xJobsTokens()), tc.method, tc.path, tc.token)
		cov5xWant(t, tc.name, rr, http.StatusServiceUnavailable, "queue_unavailable")
	}

	// Non-connectivity errors are internal errors whose cause is not exposed.
	for _, tc := range []struct {
		name string
		b    *cov5xBackend
		path string
		verb string
		msg  string
	}{
		{"list", &cov5xBackend{listErr: broken}, "/jobs", "GET", "Failed to load jobs."},
		{"retry", &cov5xBackend{dlq: []workers.TaskInfo{own}, retryErr: broken}, "/jobs/dlq/t1/retry", "POST", "Failed to retry task."},
		{"delete", &cov5xBackend{dlq: []workers.TaskInfo{own}, delErr: broken}, "/jobs/dlq/t1", "DELETE", "Failed to delete task."},
		{"dlq", &cov5xBackend{dlqErr: broken}, "/jobs/dlq", "GET", "Failed to load dead-letter queue."},
	} {
		rr := cov5xJobsDo(cov5xJobsRouter(tc.b, cov5xJobsTokens()), tc.verb, tc.path, "admin")
		cov5xWant(t, tc.name, rr, http.StatusInternalServerError, tc.msg)
		if strings.Contains(rr.Body.String(), "corrupt payload") {
			t.Errorf("%s leaked the backend error: %s", tc.name, rr.Body.String())
		}
	}
}

func TestCov5xJobsListPassesFiltersAndScopesToOrg(t *testing.T) {
	t.Parallel()
	b := &cov5xBackend{tasks: []workers.TaskInfo{
		{ID: "mine", OrgID: cov5xJobsOrg}, {ID: "theirs", OrgID: "other"}, {ID: "global"},
	}}
	router := cov5xJobsRouter(b, cov5xJobsTokens())
	rr := cov5xJobsDo(router, "GET", "/jobs?queue=%20billing%20&status=pending", "admin")
	cov5xWant(t, "list", rr, http.StatusOK, "")
	if b.listQueue != "billing" || b.listStatus != workers.StatusPending {
		t.Fatalf("filters not forwarded: %q %q", b.listQueue, b.listStatus)
	}
	var body struct {
		Tasks []workers.TaskInfo `json:"tasks"`
		Count int                `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || body.Tasks[0].ID != "mine" {
		t.Fatalf("only the caller's own org tasks may be listed: %+v", body)
	}
}

func TestCov5xJobsDLQRetryAndDeleteOutcomes(t *testing.T) {
	t.Parallel()
	own := workers.TaskInfo{ID: "t1", OrgID: cov5xJobsOrg, Status: workers.StatusDeadLetter}
	b := &cov5xBackend{dlq: []workers.TaskInfo{own}}
	router := cov5xJobsRouter(b, cov5xJobsTokens())

	rr := cov5xJobsDo(router, "POST", "/jobs/dlq/t1/retry", "admin")
	cov5xWant(t, "retry", rr, http.StatusOK, "task re-enqueued for processing")
	rr = cov5xJobsDo(router, "DELETE", "/jobs/dlq/t1", "admin")
	if rr.Code != http.StatusNoContent || rr.Body.Len() != 0 {
		t.Fatalf("delete: %d %q", rr.Code, rr.Body.String())
	}
	if len(b.retried) != 1 || b.retried[0] != "t1" || len(b.deleted) != 1 || b.deleted[0] != "t1" {
		t.Fatalf("backend calls: %v %v", b.retried, b.deleted)
	}

	// The task disappears between the ownership check and the action.
	b = &cov5xBackend{dlq: []workers.TaskInfo{own}, retryErr: workers.ErrTaskNotFound, delErr: workers.ErrTaskNotFound}
	router = cov5xJobsRouter(b, cov5xJobsTokens())
	cov5xWant(t, "retry raced", cov5xJobsDo(router, "POST", "/jobs/dlq/t1/retry", "admin"), http.StatusNotFound, "Task not found in DLQ.")
	cov5xWant(t, "delete raced", cov5xJobsDo(router, "DELETE", "/jobs/dlq/t1", "admin"), http.StatusNotFound, "Task not found in DLQ.")
}

func TestCov5xJobsQueueStatsPlatformOperatorSeesFleet(t *testing.T) {
	t.Parallel()
	b := &cov5xBackend{stats: []workers.QueueStats{{Name: "default", Pending: 7}}}
	router := cov5xJobsRouter(b, cov5xJobsTokens())
	rr := cov5xJobsDo(router, "GET", "/jobs/queues", "operator")
	cov5xWant(t, "operator", rr, http.StatusOK, `"pending":7`)

	// Without MFA the operator role is not honoured and the view stays org-scoped.
	rr = cov5xJobsDo(router, "GET", "/jobs/queues", "operator-nomfa")
	cov5xWant(t, "operator without mfa", rr, http.StatusOK, `"pending":0`)
}

func TestCov5xJobsQueueStatsForClubAdminCountsOnlyOwnTasks(t *testing.T) {
	t.Parallel()
	mk := func(id, queue string, st workers.TaskStatus, org string) workers.TaskInfo {
		return workers.TaskInfo{ID: id, Queue: queue, Status: st, OrgID: org}
	}
	b := &cov5xBackend{
		stats: []workers.QueueStats{{Name: "default", Pending: 100}, {Name: "email", Pending: 100}},
		tasks: []workers.TaskInfo{
			mk("a", "default", workers.StatusPending, cov5xJobsOrg),
			mk("b", "default", workers.StatusRetrying, cov5xJobsOrg),
			mk("c", "default", workers.StatusActive, cov5xJobsOrg),
			mk("d", "billing", workers.StatusCompleted, cov5xJobsOrg), // queue unknown to QueueStats
			mk("e", "billing", workers.StatusFailed, cov5xJobsOrg),
			mk("f", "default", workers.StatusPending, "someone-else"),
		},
		dlq: []workers.TaskInfo{
			mk("g", "default", workers.StatusDeadLetter, cov5xJobsOrg),
			mk("a", "default", workers.StatusPending, cov5xJobsOrg), // duplicate of a listed task: counted once
		},
	}
	rr := cov5xJobsDo(cov5xJobsRouter(b, cov5xJobsTokens()), "GET", "/jobs/queues", "admin")
	cov5xWant(t, "stats", rr, http.StatusOK, "")
	var body struct {
		Queues []workers.QueueStats `json:"queues"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []workers.QueueStats{
		{Name: "billing", Completed: 1, Failed: 1},
		{Name: "default", Pending: 2, Active: 1, DeadLetter: 1},
		{Name: "email"},
	}
	if len(body.Queues) != len(want) {
		t.Fatalf("queues: %+v", body.Queues)
	}
	for i, w := range want {
		if body.Queues[i] != w {
			t.Errorf("queue %d: got %+v want %+v", i, body.Queues[i], w)
		}
	}
}

func TestCov5xJobsQueueStatsToleratesSeedFailure(t *testing.T) {
	t.Parallel()
	b := &cov5xBackend{statsErr: io.ErrClosedPipe, tasks: []workers.TaskInfo{{ID: "a", Queue: "default", Status: workers.StatusActive, OrgID: cov5xJobsOrg}}}
	rr := cov5xJobsDo(cov5xJobsRouter(b, cov5xJobsTokens()), "GET", "/jobs/queues", "admin")
	cov5xWant(t, "stats", rr, http.StatusOK, `"active":1`)
}

func TestCov5xJobsMetrics(t *testing.T) {
	t.Parallel()
	router := cov5xJobsRouter(&cov5xBackend{metrics: cov5xMetrics{}}, cov5xJobsTokens())
	rr := cov5xJobsDo(router, "GET", "/jobs/metrics", "operator")
	cov5xWant(t, "operator metrics", rr, http.StatusOK, "global:type")
	cov5xWant(t, "operator metrics limit", rr, http.StatusOK, "limit-50")

	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-time.Hour)
	var tasks []workers.TaskInfo
	tasks = append(tasks,
		workers.TaskInfo{ID: "ok", Type: "email:send", Queue: "email", Status: workers.StatusCompleted, OrgID: cov5xJobsOrg, CompletedAt: &now},
		workers.TaskInfo{ID: "retry", Type: "email:send", Queue: "email", Status: workers.StatusRetrying, OrgID: cov5xJobsOrg, LastFailedAt: &older, LastErr: "smtp down", Retried: 2},
		workers.TaskInfo{ID: "dead", Type: "billing:run", Queue: "billing", Status: workers.StatusDeadLetter, OrgID: cov5xJobsOrg},
		workers.TaskInfo{ID: "failed", Type: "billing:run", Queue: "billing", Status: workers.StatusFailed, OrgID: cov5xJobsOrg},
		workers.TaskInfo{ID: "foreign", Type: "email:send", Queue: "email", Status: workers.StatusCompleted, OrgID: "other", CompletedAt: &now},
	)
	b := &cov5xBackend{tasks: tasks}
	rr = cov5xJobsDo(cov5xJobsRouter(b, cov5xJobsTokens()), "GET", "/jobs/metrics", "admin")
	cov5xWant(t, "club metrics", rr, http.StatusOK, "")
	var body struct {
		ByType map[string]workers.TypeMetricSnapshot `json:"by_type"`
		Recent []workers.JobExecution                `json:"recent_executions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := body.ByType["email:send"]; got != (workers.TypeMetricSnapshot{Enqueued: 2, Succeeded: 1, Failed: 1}) {
		t.Errorf("email:send: %+v", got)
	}
	if got := body.ByType["billing:run"]; got != (workers.TypeMetricSnapshot{Enqueued: 2, Failed: 2, DLQ: 1}) {
		t.Errorf("billing:run: %+v", got)
	}
	if len(body.Recent) != 2 || body.Recent[0].TaskID != "ok" || body.Recent[1].TaskID != "retry" ||
		body.Recent[1].Err != "smtp down" || body.Recent[1].Retried != 2 {
		t.Errorf("recent executions must be newest first and include only finished own-org tasks: %+v", body.Recent)
	}
}

func TestCov5xJobsMetricsCapsRecentExecutionsAtFifty(t *testing.T) {
	t.Parallel()
	base := time.Now().UTC()
	var tasks []workers.TaskInfo
	for i := 0; i < 60; i++ {
		done := base.Add(time.Duration(i) * time.Second)
		tasks = append(tasks, workers.TaskInfo{ID: fmt.Sprintf("t%02d", i), Type: "x", Status: workers.StatusCompleted, OrgID: cov5xJobsOrg, CompletedAt: &done})
	}
	rr := cov5xJobsDo(cov5xJobsRouter(&cov5xBackend{tasks: tasks}, cov5xJobsTokens()), "GET", "/jobs/metrics", "admin")
	var body struct {
		Recent []workers.JobExecution `json:"recent_executions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Recent) != 50 || body.Recent[0].TaskID != "t59" || body.Recent[49].TaskID != "t10" {
		t.Fatalf("expected the 50 newest, got %d first=%s", len(body.Recent), body.Recent[0].TaskID)
	}
}

type cov5xTimeoutErr struct{}

func (cov5xTimeoutErr) Error() string   { return "i/o timeout" }
func (cov5xTimeoutErr) Timeout() bool   { return true }
func (cov5xTimeoutErr) Temporary() bool { return true }

type cov5xFastErr struct{}

func (cov5xFastErr) Error() string   { return "not a timeout" }
func (cov5xFastErr) Timeout() bool   { return false }
func (cov5xFastErr) Temporary() bool { return false }

func TestCov5xIsQueueUnavailableClassifiesByType(t *testing.T) {
	t.Parallel()
	var syntaxErr error = &json.SyntaxError{}
	var typeErr error = &json.UnmarshalTypeError{Value: "string", Type: nil}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", fmt.Errorf("op: %w", context.DeadlineExceeded), true},
		{"closed conn", net.ErrClosed, true},
		{"redis closed", redis.ErrClosed, true},
		{"pool timeout", redis.ErrPoolTimeout, true},
		{"pool exhausted", redis.ErrPoolExhausted, true},
		{"refused", fmt.Errorf("x: %w", syscall.ECONNREFUSED), true},
		{"reset", syscall.ECONNRESET, true},
		{"broken pipe", syscall.EPIPE, true},
		{"host unreachable", syscall.EHOSTUNREACH, true},
		{"net unreachable", syscall.ENETUNREACH, true},
		{"op error", &net.OpError{Op: "read", Err: errors.New("boom")}, true},
		{"dns", &net.DNSError{Err: "no such host", Name: "redis"}, true},
		{"timeout net.Error", cov5xTimeoutErr{}, true},
		{"non-timeout net.Error", cov5xFastErr{}, false},
		{"bare EOF from a dropped reply", io.EOF, true},
		{"EOF inside a JSON syntax error", errors.Join(io.EOF, syntaxErr), false},
		{"EOF inside a JSON type error", errors.Join(io.EOF, typeErr), false},
		{"application error", errors.New("validation failed"), false},
	}
	for _, tc := range cases {
		if got := isQueueUnavailable(tc.err); got != tc.want {
			t.Errorf("%s: isQueueUnavailable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
