// Package workers provides the background job queue infrastructure for the
// DARE platform. It defines a Queue interface and an in-memory Worker that
// processes jobs using goroutine pools. The design mirrors Asynq semantics
// so that the real Redis-backed Asynq client can be dropped in as a
// replacement without changing callers.
//
// Queue names by domain:
//
//	imports   — file parsing and import processing
//	billing   — billing reconciliation and integrity checks
//	email     — campaign batch sending
//	reconcile — Stripe reconciliation
//	default   — general-purpose and cleanup jobs
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Queue name constants (domain-scoped).
const (
	QueueImports   = "imports"
	QueueBilling   = "billing"
	QueueEmail     = "email"
	QueueReconcile = "reconcile"
	QueueDefault   = "default"
)

// Task type constants. These mirror Asynq task-type naming conventions.
const (
	TypeParseImport         = "imports:parse"
	TypeGenerateExport      = "exports:generate"
	TypeBillingReconcile    = "billing:reconcile"
	TypeNightlyIntegrity    = "integrity:nightly"
	TypePurgeExpiredFiles   = "cleanup:purge_expired"
	TypeProcessDataExport   = "compliance:data_export"
	TypeProcessDataDeletion = "compliance:data_deletion"
	TypeBillingRecurringRun = "billing:recurring_run"
	TypeBillingLateFees     = "billing:late_fees"
)

// TaskStatus represents the lifecycle state of a queued task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusActive     TaskStatus = "active"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusRetrying   TaskStatus = "retrying"
	StatusDeadLetter TaskStatus = "dead_letter"
)

// Task is the unit of work enqueued by callers.
type Task struct {
	// Type identifies the handler that will process this task.
	Type string `json:"type"`
	// Payload is arbitrary JSON-encoded data consumed by the handler.
	Payload json.RawMessage `json:"payload"`
}

// NewTask constructs a Task with a JSON-encoded payload.
func NewTask(taskType string, payload any) (*Task, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Task{Type: taskType, Payload: raw}, nil
}

// EnqueueOptions configures optional task behaviour.
type EnqueueOptions struct {
	// Queue overrides the default queue for this task.
	Queue string
	// MaxRetries overrides the per-queue default retry limit.
	MaxRetries *int
	// ProcessIn delays the task by this duration.
	ProcessIn time.Duration
	// UniqueKey prevents duplicate tasks with the same key from being enqueued.
	UniqueKey string
}

// Option is a functional option for Enqueue.
type Option func(*EnqueueOptions)

// WithQueue sets the target queue.
func WithQueue(q string) Option {
	return func(o *EnqueueOptions) { o.Queue = q }
}

// WithMaxRetries sets the maximum retry count.
func WithMaxRetries(n int) Option {
	return func(o *EnqueueOptions) { o.MaxRetries = &n }
}

// WithDelay delays task processing.
func WithDelay(d time.Duration) Option {
	return func(o *EnqueueOptions) { o.ProcessIn = d }
}

// WithUniqueKey prevents duplicate task enqueue within the queue.
func WithUniqueKey(key string) Option {
	return func(o *EnqueueOptions) { o.UniqueKey = key }
}

// TaskInfo is returned by Enqueue and describes the queued task.
type TaskInfo struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Queue       string     `json:"queue"`
	Status      TaskStatus `json:"status"`
	EnqueuedAt  time.Time  `json:"enqueued_at"`
	ProcessAt   time.Time  `json:"process_at"`
	MaxRetries  int        `json:"max_retries"`
	Retried     int        `json:"retried"`
	LastErr     string     `json:"last_error,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	UniqueKey   string     `json:"unique_key,omitempty"`
}

// QueueStats summarises the state of a single named queue.
type QueueStats struct {
	Name       string `json:"name"`
	Pending    int    `json:"pending"`
	Active     int    `json:"active"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
	DeadLetter int    `json:"dead_letter"`
}

// MetricsReader exposes queue execution counters and recent execution history.
type MetricsReader interface {
	TypeSnapshot() map[string]TypeMetricSnapshot
	RecentExecutions(limit int) []JobExecution
}

// Queue is the interface for enqueuing background tasks. The in-memory
// implementation is used for local development and tests; the real Asynq
// client satisfies the same interface when Redis is available.
type Queue interface {
	// Enqueue adds a task to the specified queue for immediate processing.
	Enqueue(ctx context.Context, task *Task, opts ...Option) (*TaskInfo, error)
	// EnqueueIn adds a task that will not be processed before the delay elapses.
	EnqueueIn(ctx context.Context, delay time.Duration, task *Task, opts ...Option) (*TaskInfo, error)
}

// Backend is the app-facing background-job interface used by HTTP handlers and
// domain services. Both the in-memory worker server and the Redis/Asynq-backed
// runtime satisfy this contract.
type Backend interface {
	Queue
	RegisterHandler(taskType string, h Handler)
	ListTasks(queue string, status TaskStatus) ([]TaskInfo, error)
	QueueStats() ([]QueueStats, error)
	DLQTasks() ([]TaskInfo, error)
	RetryDLQTask(ctx context.Context, taskID string) error
	DeleteDLQTask(taskID string) error
	Metrics() MetricsReader
}

// Sentinel errors.
var (
	ErrTaskNotFound  = errors.New("task not found")
	ErrWorkerClosed  = errors.New("worker server is closed")
	ErrDuplicateTask = errors.New("task with this unique key is already enqueued")
)
