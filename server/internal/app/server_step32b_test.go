package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/storage"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

func TestDurableJobsSurviveAPIRestart(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer redisServer.Close()

	redisURL := fmt.Sprintf("redis://%s/0", redisServer.Addr())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	backend1, err := workers.NewRedisBackend(workers.RedisBackendConfig{
		RedisURL: redisURL,
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewRedisBackend(1): %v", err)
	}

	task, err := workers.NewTask(workers.TypeParseImport, map[string]string{
		"import_id": "imp-restart",
		"org_id":    "org-step17",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	enqueued, err := backend1.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueImports))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := backend1.Close(); err != nil {
		t.Fatalf("backend1.Close: %v", err)
	}

	backend2, err := workers.NewRedisBackend(workers.RedisBackendConfig{
		RedisURL: redisURL,
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewRedisBackend(2): %v", err)
	}
	defer func() { _ = backend2.Close() }()

	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
		backend2,
		nil)
	token := issueStep17Token(t, router, "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?queue=imports", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-step17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list jobs status = %d; body: %s", w.Code, w.Body.String())
	}

	var jobsResp struct {
		Tasks []workers.TaskInfo `json:"tasks"`
	}
	if err := json.NewDecoder(w.Body).Decode(&jobsResp); err != nil {
		t.Fatalf("decode jobs response: %v", err)
	}
	found := false
	for _, job := range jobsResp.Tasks {
		if job.ID == enqueued.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected queued task %q after API restart, got %+v", enqueued.ID, jobsResp.Tasks)
	}

	processed := make(chan string, 1)
	workerBackend, err := workers.NewRedisBackend(workers.RedisBackendConfig{
		RedisURL: redisURL,
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewRedisBackend(worker): %v", err)
	}
	defer func() { _ = workerBackend.Close() }()

	workerBackend.RegisterHandler(workers.TypeParseImport, func(ctx context.Context, task *workers.Task) error {
		processed <- task.Type
		return nil
	})
	if err := workerBackend.Start(); err != nil {
		t.Fatalf("workerBackend.Start: %v", err)
	}
	defer func() {
		if err := workerBackend.Stop(context.Background()); err != nil {
			t.Fatalf("workerBackend.Stop: %v", err)
		}
	}()

	select {
	case got := <-processed:
		if got != workers.TypeParseImport {
			t.Fatalf("processed type = %q; want %q", got, workers.TypeParseImport)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("durable task was not processed after worker start")
	}
}

func TestBuildWorkersRegistersFullProductionHandlerSet(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	commsService := comms.NewService()
	complianceService := compliance.NewService()
	exportRequest, err := complianceService.CreateDataExportRequest("org-step32b", "hh-step32b", "admin-user")
	if err != nil {
		t.Fatalf("CreateDataExportRequest: %v", err)
	}
	deletionRequest, err := complianceService.CreateDataDeletionRequest("org-step32b", "hh-step32b", "admin-user", "test")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest: %v", err)
	}
	objectStore := storage.NewStubStorage("")
	billingService := billing.NewService()
	stripeClient := &stripelib.StubClient{}
	workerServer, _ := BuildWorkers(
		Config{},
		logger,
		exports.NewService(),
		reports.NewService(nil, nil, nil, nil),
		billingService,
		stripeClient,
		commsService,
		complianceService,
		members.NewService(),
		documents.NewService(),
		objectStore,
		keycloak.NewStubClient(),
	)
	workerServer.Start()
	defer workerServer.Stop()

	testCases := []struct {
		name     string
		taskType string
		queue    string
		payload  any
	}{
		{
			name:     "generate export",
			taskType: workers.TypeGenerateExport,
			queue:    workers.QueueDefault,
			payload: map[string]string{
				"export_id": "exp-full-set",
				"meet_id":   "meet-step32b",
			},
		},
		{
			name:     "billing reconcile",
			taskType: workers.TypeBillingReconcile,
			queue:    workers.QueueReconcile,
			payload: map[string]string{
				"org_id":       "org-step32b",
				"triggered_by": "test",
			},
		},
		{
			name:     "nightly integrity",
			taskType: workers.TypeNightlyIntegrity,
			queue:    workers.QueueBilling,
			payload: map[string]string{
				"org_id": "org-step32b",
				"date":   "2026-03-25",
			},
		},
		{
			name:     "purge expired files",
			taskType: workers.TypePurgeExpiredFiles,
			queue:    workers.QueueDefault,
			payload: map[string]string{
				"org_id": "org-step32b",
			},
		},
		{
			name:     "process data export",
			taskType: workers.TypeProcessDataExport,
			queue:    workers.QueueDefault,
			payload: map[string]string{
				"request_id":   exportRequest.ID,
				"org_id":       "org-step32b",
				"household_id": "hh-step32b",
			},
		},
		{
			name:     "process data deletion",
			taskType: workers.TypeProcessDataDeletion,
			queue:    workers.QueueDefault,
			payload: map[string]string{
				"request_id":   deletionRequest.ID,
				"org_id":       "org-step32b",
				"household_id": "hh-step32b",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task, err := workers.NewTask(tc.taskType, tc.payload)
			if err != nil {
				t.Fatalf("NewTask: %v", err)
			}
			if _, err := workerServer.Enqueue(context.Background(), task, workers.WithQueue(tc.queue)); err != nil {
				t.Fatalf("Enqueue(%s): %v", tc.taskType, err)
			}
		})
	}
}

func TestDurableWorkerModeRegistersAndProcessesSharedHandlers(t *testing.T) {
	backend, cleanup := newStep32BRedisBackend(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
		backend,
		nil)

	if err := backend.Start(); err != nil {
		t.Fatalf("backend.Start: %v", err)
	}
	defer func() {
		if err := backend.Stop(context.Background()); err != nil {
			t.Fatalf("backend.Stop: %v", err)
		}
	}()

	task, err := workers.NewTask(workers.TypeBillingReconcile, map[string]string{
		"org_id":       "org-step32b",
		"triggered_by": "test",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	enqueued, err := backend.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueReconcile))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitForTaskCompletion(t, backend, workers.QueueReconcile, enqueued.ID, workers.TypeBillingReconcile, 5*time.Second)
}

func TestDurableWorkerModeExecutesScheduledReconcileJobs(t *testing.T) {
	backend, cleanup := newStep32BRedisBackend(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	NewRouterWithServices(
		Config{
			Auth: AuthConfig{EnableDevStubTokens: true},
			Worker: WorkerConfig{
				ReconcileInterval: "50ms",
			},
		},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
		backend,
		nil)
	RegisterDurableScheduledJobs(backend, Config{Worker: WorkerConfig{ReconcileInterval: "50ms"}})

	if err := backend.Start(); err != nil {
		t.Fatalf("backend.Start: %v", err)
	}
	defer func() {
		if err := backend.Stop(context.Background()); err != nil {
			t.Fatalf("backend.Stop: %v", err)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, exec := range backend.Metrics().RecentExecutions(20) {
			if exec.Type == workers.TypeBillingReconcile && exec.Status == workers.StatusCompleted {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("expected scheduled billing reconcile job to complete in durable worker mode")
}

func newStep32BRedisBackend(t *testing.T) (*workers.RedisBackend, func()) {
	t.Helper()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	redisURL := fmt.Sprintf("redis://%s/0", redisServer.Addr())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, err := workers.NewRedisBackend(workers.RedisBackendConfig{
		RedisURL: redisURL,
		Logger:   logger,
	})
	if err != nil {
		redisServer.Close()
		t.Fatalf("NewRedisBackend: %v", err)
	}

	return backend, func() {
		_ = backend.Close()
		redisServer.Close()
	}
}

func waitForTaskCompletion(t *testing.T, backend workers.Backend, queue, taskID, taskType string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, exec := range backend.Metrics().RecentExecutions(50) {
			if exec.TaskID == taskID {
				if exec.Type != taskType {
					t.Fatalf("completed task type = %q; want %q", exec.Type, taskType)
				}
				if exec.Queue != queue {
					t.Fatalf("completed task queue = %q; want %q", exec.Queue, queue)
				}
				if exec.Status == workers.StatusCompleted {
					return
				}
				t.Fatalf("task %q finished with status %q and error %q", taskID, exec.Status, exec.Err)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("task %q did not complete within %s", taskID, timeout)
}
