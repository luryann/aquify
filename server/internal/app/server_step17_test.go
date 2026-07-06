package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newStep17Router(t *testing.T, ws workers.Backend) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
		ws,
		nil,
	)
}

func issueStep17Token(t *testing.T, router http.Handler, role string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"user_id": "user-step17",
		"org_id":  "org-step17",
		"roles":   []string{role},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("issue token: status %d body %s", w.Code, w.Body.String())
	}
	return issuedAccessTokenFromResponse(t, w)
}

// ─── Worker unit tests ────────────────────────────────────────────────────────

func TestWorker_EnqueueAndProcess(t *testing.T) {
	srv := workers.NewServer(workers.ServerConfig{Concurrency: 1})

	processed := make(chan string, 1)
	srv.RegisterHandler(workers.TypeParseImport, func(ctx context.Context, task *workers.Task) error {
		processed <- task.Type
		return nil
	})
	srv.Start()
	defer srv.Stop()

	task, err := workers.NewTask(workers.TypeParseImport, map[string]string{"import_id": "imp-1", "org_id": "org-1"})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	info, err := srv.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueImports))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if info.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if info.Queue != workers.QueueImports {
		t.Errorf("queue = %q; want %q", info.Queue, workers.QueueImports)
	}

	select {
	case got := <-processed:
		if got != workers.TypeParseImport {
			t.Errorf("processed type = %q; want %q", got, workers.TypeParseImport)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("task was not processed within timeout")
	}
}

func TestWorker_RetryOnError(t *testing.T) {
	count := 0
	attempts := make(chan int, 5)
	// Use very short backoff for tests.
	srv2 := workers.NewServer(workers.ServerConfig{
		Concurrency: 1,
		RetryPolicies: map[string]workers.RetryPolicy{
			workers.QueueReconcile: {MaxRetries: 2, Backoff: []time.Duration{5 * time.Millisecond}},
		},
	})
	srv2.RegisterHandler(workers.TypeBillingReconcile, func(ctx context.Context, task *workers.Task) error {
		count++
		attempts <- count
		if count < 2 {
			return io.ErrUnexpectedEOF
		}
		return nil
	})
	srv2.Start()
	defer srv2.Stop()

	task, _ := workers.NewTask(workers.TypeBillingReconcile, map[string]string{"org_id": "org-1"})
	_, err := srv2.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueReconcile))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for at least 2 attempts.
	for i := 0; i < 2; i++ {
		select {
		case <-attempts:
		case <-time.After(3 * time.Second):
			t.Fatal("expected 2 processing attempts within timeout")
		}
	}
}

func TestWorker_DLQ(t *testing.T) {
	srv := workers.NewServer(workers.ServerConfig{
		Concurrency: 1,
		RetryPolicies: map[string]workers.RetryPolicy{
			workers.QueueDefault: {MaxRetries: 1, Backoff: []time.Duration{5 * time.Millisecond}},
		},
	})
	executed := make(chan struct{}, 5) // buffered so we don't panic on multiple calls
	srv.RegisterHandler(workers.TypePurgeExpiredFiles, func(ctx context.Context, task *workers.Task) error {
		executed <- struct{}{}
		return io.ErrUnexpectedEOF // always fail → will exhaust retries → DLQ
	})
	srv.Start()
	defer srv.Stop()

	task, _ := workers.NewTask(workers.TypePurgeExpiredFiles, map[string]string{})
	_, err := srv.Enqueue(context.Background(), task)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for first execution.
	select {
	case <-executed:
	case <-time.After(2 * time.Second):
		t.Fatal("task never executed")
	}

	// Allow time for the retry to execute and exhaust, landing in DLQ.
	// MaxRetries=1 means at most 2 attempts total, with 5ms backoff.
	deadline := time.After(2 * time.Second)
	for {
		dlq, err := srv.DLQTasks()
		if err != nil {
			t.Fatalf("DLQTasks: %v", err)
		}
		if len(dlq) > 0 {
			return // success
		}
		select {
		case <-deadline:
			t.Fatal("expected at least one task in DLQ after retries exhausted")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestWorker_Metrics(t *testing.T) {
	srv := workers.NewServer(workers.ServerConfig{Concurrency: 1})
	done := make(chan struct{})
	srv.RegisterHandler(workers.TypeGenerateExport, func(ctx context.Context, task *workers.Task) error {
		close(done)
		return nil
	})
	srv.Start()
	defer srv.Stop()

	task, _ := workers.NewTask(workers.TypeGenerateExport, map[string]string{"export_id": "exp-1", "meet_id": "meet-1"})
	_, err := srv.Enqueue(context.Background(), task)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task never executed")
	}

	// Allow the metrics to be recorded (they happen after handler returns).
	time.Sleep(20 * time.Millisecond)

	m := srv.Metrics()
	snap := m.TypeSnapshot()
	if snap[workers.TypeGenerateExport].Enqueued == 0 {
		t.Error("expected enqueued > 0")
	}
	if snap[workers.TypeGenerateExport].Succeeded == 0 {
		t.Error("expected succeeded > 0")
	}
}

func TestWorker_Scheduler(t *testing.T) {
	srv := workers.NewServer(workers.ServerConfig{Concurrency: 1})
	fired := make(chan struct{}, 3)
	srv.RegisterHandler(workers.TypePurgeExpiredFiles, func(ctx context.Context, task *workers.Task) error {
		fired <- struct{}{}
		return nil
	})
	srv.Start()
	defer srv.Stop()

	sched := workers.NewScheduler(srv, nil)
	sched.Register(workers.ScheduledJob{
		Name:     "test-cleanup",
		TaskType: workers.TypePurgeExpiredFiles,
		Queue:    workers.QueueDefault,
		Interval: 20 * time.Millisecond,
		Payload:  map[string]string{},
	})
	sched.Start()
	defer sched.Stop()

	// Expect at least 2 fires within 200ms.
	count := 0
	deadline := time.After(500 * time.Millisecond)
	for count < 2 {
		select {
		case <-fired:
			count++
		case <-deadline:
			t.Fatalf("scheduler: only got %d fires, want 2", count)
		}
	}
}

// ─── HTTP admin endpoint tests ────────────────────────────────────────────────

func TestJobsEndpoints_Auth(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/jobs"},
		{http.MethodGet, "/api/v1/jobs/queues"},
		{http.MethodGet, "/api/v1/jobs/metrics"},
		{http.MethodGet, "/api/v1/jobs/dlq"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Org-Id", "org-step17")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d; want 200; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestJobsEndpoints_Forbidden(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "parent") // parent cannot see jobs

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", w.Code)
	}
}

func TestJobsEndpoints_QueueStats(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/queues", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Queues []workers.QueueStats `json:"queues"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Queues) == 0 {
		t.Error("expected at least one queue in stats")
	}
}

func TestJobsEndpoints_DLQRetry(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	// Retry a non-existent task — should 404.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/dlq/no-such-task/retry", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestJobsEndpoints_DLQDelete(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	// Delete a non-existent DLQ task — should 404.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/dlq/no-such-task", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestJobsEndpoints_ListJobsFilter(t *testing.T) {
	srv := buildTestWorkerServer(t)
	// Enqueue a task so the list is non-trivial.
	task, _ := workers.NewTask(workers.TypeParseImport, map[string]string{"import_id": "x", "org_id": "y"})
	_, _ = srv.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueImports))

	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	// Filter by queue.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?queue=imports", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Tasks []workers.TaskInfo `json:"tasks"`
		Count int                `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, task := range resp.Tasks {
		if task.Queue != workers.QueueImports {
			t.Errorf("unexpected queue %q in filtered list", task.Queue)
		}
	}
}

func TestJobsEndpoints_Metrics(t *testing.T) {
	srv := buildTestWorkerServer(t)
	router := newStep17Router(t, srv)
	token := issueStep17Token(t, router, "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["by_type"]; !ok {
		t.Error("expected 'by_type' in metrics response")
	}
	if _, ok := resp["recent_executions"]; !ok {
		t.Error("expected 'recent_executions' in metrics response")
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildTestWorkerServer(t *testing.T) *workers.Server {
	t.Helper()
	srv := workers.NewServer(workers.ServerConfig{Concurrency: 1})
	srv.RegisterHandler(workers.TypeParseImport, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeGenerateExport, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeSendCampaignBatch, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeBillingReconcile, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeNightlyIntegrity, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypePurgeExpiredFiles, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeProcessDataExport, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.RegisterHandler(workers.TypeProcessDataDeletion, func(ctx context.Context, _ *workers.Task) error { return nil })
	srv.Start()
	t.Cleanup(func() { srv.Stop() })
	return srv
}
