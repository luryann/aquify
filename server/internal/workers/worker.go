package workers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// RetryPolicy defines backoff behaviour for a queue.
type RetryPolicy struct {
	MaxRetries int
	// Backoff[i] is the wait duration before the (i+1)-th retry.
	// If i >= len(Backoff), the last entry is reused.
	Backoff []time.Duration
}

// DefaultRetryPolicies returns sensible per-queue retry policies.
func DefaultRetryPolicies() map[string]RetryPolicy {
	return map[string]RetryPolicy{
		QueueImports: {
			MaxRetries: 5,
			Backoff:    []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour},
		},
		QueueBilling: {
			MaxRetries: 3,
			Backoff:    []time.Duration{time.Minute, 10 * time.Minute, time.Hour},
		},
		QueueEmail: {
			MaxRetries: 3,
			Backoff:    []time.Duration{30 * time.Second, 5 * time.Minute, 30 * time.Minute},
		},
		QueueReconcile: {
			MaxRetries: 2,
			Backoff:    []time.Duration{5 * time.Minute, 30 * time.Minute},
		},
		QueueDefault: {
			MaxRetries: 3,
			Backoff:    []time.Duration{30 * time.Second, 5 * time.Minute, 30 * time.Minute},
		},
	}
}

// Handler is a function that processes one task. It should return a non-nil
// error if the task should be retried.
type Handler func(ctx context.Context, task *Task) error

// internalTask augments Task with runtime scheduling metadata.
type internalTask struct {
	TaskInfo
	task    *Task
	handler Handler
}

// maxCompletedHistory is the maximum number of completed/failed (non-DLQ) tasks
// retained in allTasks before oldest entries are evicted.
const maxCompletedHistory = 500

// Server is the in-memory background job worker. It satisfies the Queue
// interface for enqueueing, and exposes Start/Stop for lifecycle management.
//
// Production note: replace with github.com/hibiken/asynq by creating an
// asynqQueue adapter that wraps asynq.Client and asynq.Server behind the
// same Queue interface — all handler functions stay identical.
type Server struct {
	mu             sync.RWMutex
	closed         bool
	logger         *slog.Logger
	handlers       map[string]Handler
	retryPolicies  map[string]RetryPolicy
	queues         map[string]chan *internalTask // one buffered chan per queue name
	dlq            []*internalTask
	allTasks       map[string]*internalTask // by ID for admin lookups
	uniqueKeyIndex map[string]string        // uniqueKey → taskID; prevents duplicate enqueues
	completedIDs   []string                 // bounded FIFO of recently completed/failed task IDs
	metrics        *JobMetrics
	wg             sync.WaitGroup
	ctx            context.Context    // server lifecycle context
	cancel         context.CancelFunc
	concurrency    int // goroutines per queue
}

// ServerConfig holds worker server settings.
type ServerConfig struct {
	// Concurrency is the number of goroutines that process each queue.
	// Defaults to 2.
	Concurrency int
	// RetryPolicies overrides the default per-queue policies.
	RetryPolicies map[string]RetryPolicy
	Logger        *slog.Logger
}

// NewServer creates a new in-memory worker server.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	policies := DefaultRetryPolicies()
	for q, p := range cfg.RetryPolicies {
		policies[q] = p
	}

	queueNames := []string{QueueImports, QueueBilling, QueueEmail, QueueReconcile, QueueDefault}
	queues := make(map[string]chan *internalTask, len(queueNames))
	for _, name := range queueNames {
		queues[name] = make(chan *internalTask, 256)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		logger:         cfg.Logger,
		handlers:       make(map[string]Handler),
		retryPolicies:  policies,
		queues:         queues,
		allTasks:       make(map[string]*internalTask),
		uniqueKeyIndex: make(map[string]string),
		metrics:        NewJobMetrics(),
		concurrency:    cfg.Concurrency,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// RegisterHandler registers the handler function for a task type.
// Must be called before Start.
func (s *Server) RegisterHandler(taskType string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[taskType] = h
}

// Metrics returns the job metrics store.
func (s *Server) Metrics() MetricsReader { return s.metrics }

// Start launches worker goroutines. It is non-blocking: goroutines run until
// Stop is called.
func (s *Server) Start() {
	for qName, ch := range s.queues {
		for i := 0; i < s.concurrency; i++ {
			s.wg.Add(1)
			go s.runWorker(s.ctx, qName, ch)
		}
	}
	s.logger.Info("workers started", "concurrency_per_queue", s.concurrency, "queues", len(s.queues))
}

// Stop signals all workers to finish their current task and exit.
func (s *Server) Stop() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("workers stopped")
}

// ─── Queue interface ──────────────────────────────────────────────────────────

// Enqueue implements Queue. It dispatches the task to the appropriate queue
// channel for immediate processing.
func (s *Server) Enqueue(ctx context.Context, task *Task, opts ...Option) (*TaskInfo, error) {
	return s.enqueueAt(ctx, time.Now(), task, opts...)
}

// EnqueueIn implements Queue. It dispatches the task after the given delay.
func (s *Server) EnqueueIn(ctx context.Context, delay time.Duration, task *Task, opts ...Option) (*TaskInfo, error) {
	return s.enqueueAt(ctx, time.Now().Add(delay), task, opts...)
}

func (s *Server) enqueueAt(ctx context.Context, processAt time.Time, task *Task, opts ...Option) (*TaskInfo, error) {
	o := &EnqueueOptions{Queue: QueueDefault}
	for _, opt := range opts {
		opt(o)
	}

	policy := s.policyFor(o.Queue)
	maxRetries := policy.MaxRetries
	if o.MaxRetries != nil {
		maxRetries = *o.MaxRetries
	}

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate task id: %w", err)
	}

	// Acquire write lock for all critical checks and insertions.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrWorkerClosed
	}
	h, ok := s.handlers[task.Type]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no handler registered for task type %q", task.Type)
	}
	if o.UniqueKey != "" {
		if _, exists := s.uniqueKeyIndex[o.UniqueKey]; exists {
			s.mu.Unlock()
			return nil, ErrDuplicateTask
		}
	}
	it := &internalTask{
		TaskInfo: TaskInfo{
			ID:         id,
			Type:       task.Type,
			Queue:      o.Queue,
			Status:     StatusPending,
			EnqueuedAt: time.Now(),
			ProcessAt:  processAt,
			MaxRetries: maxRetries,
			UniqueKey:  o.UniqueKey,
		},
		task:    task,
		handler: h,
	}
	s.allTasks[id] = it
	if o.UniqueKey != "" {
		s.uniqueKeyIndex[o.UniqueKey] = id
	}
	s.mu.Unlock()

	s.metrics.RecordEnqueue(task.Type)

	// Snapshot TaskInfo before dispatching — the worker goroutine may mutate
	// the internalTask struct (Status, CompletedAt, etc.) the moment it is
	// sent to the channel, causing a race on the returned value.
	info := it.TaskInfo

	ch := s.channelFor(o.Queue)

	if delay := time.Until(processAt); delay > 0 {
		// Use the server's own context (s.ctx), NOT the caller's context, so that
		// a short-lived HTTP request context cancellation does not silently drop
		// the task. On server shutdown (s.ctx.Done), move the task to the DLQ.
		go func() {
			select {
			case <-s.ctx.Done():
				s.mu.Lock()
				it.LastErr = "server_shutdown"
				if it.UniqueKey != "" {
					delete(s.uniqueKeyIndex, it.UniqueKey)
				}
				s.dlq = append(s.dlq, it)
				s.mu.Unlock()
				s.setStatus(it, StatusDeadLetter)
			case <-time.After(delay):
				ch <- it
			}
		}()
	} else {
		select {
		case ch <- it:
		case <-ctx.Done():
			// Immediate enqueue: remove from allTasks if we can't dispatch.
			s.mu.Lock()
			delete(s.allTasks, id)
			if o.UniqueKey != "" {
				delete(s.uniqueKeyIndex, o.UniqueKey)
			}
			s.mu.Unlock()
			return nil, ctx.Err()
		}
	}

	return &info, nil
}

// ─── Admin accessors ──────────────────────────────────────────────────────────

// ListTasks returns all known tasks optionally filtered by queue and/or status.
func (s *Server) ListTasks(queue string, status TaskStatus) ([]TaskInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaskInfo, 0, len(s.allTasks))
	for _, it := range s.allTasks {
		if queue != "" && it.Queue != queue {
			continue
		}
		if status != "" && it.Status != status {
			continue
		}
		out = append(out, it.TaskInfo)
	}
	return out, nil
}

// GetTask returns a single task by ID.
func (s *Server) GetTask(id string) (TaskInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.allTasks[id]
	if !ok {
		return TaskInfo{}, ErrTaskNotFound
	}
	return it.TaskInfo, nil
}

// QueueStats returns per-queue depth and completion counters.
func (s *Server) QueueStats() ([]QueueStats, error) {
	s.mu.RLock()
	counts := make(map[string]map[TaskStatus]int)
	for _, it := range s.allTasks {
		if counts[it.Queue] == nil {
			counts[it.Queue] = make(map[TaskStatus]int)
		}
		counts[it.Queue][it.Status]++
	}
	s.mu.RUnlock()

	out := make([]QueueStats, 0, len(s.queues))
	for name := range s.queues {
		c := counts[name]
		out = append(out, QueueStats{
			Name:       name,
			Pending:    c[StatusPending] + c[StatusRetrying],
			Active:     c[StatusActive],
			Completed:  c[StatusCompleted],
			Failed:     c[StatusFailed],
			DeadLetter: c[StatusDeadLetter],
		})
	}
	return out, nil
}

// DLQTasks returns all tasks in the dead-letter queue.
func (s *Server) DLQTasks() ([]TaskInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaskInfo, len(s.dlq))
	for i, it := range s.dlq {
		out[i] = it.TaskInfo
	}
	return out, nil
}

// RetryDLQTask re-enqueues a dead-letter task for immediate processing.
func (s *Server) RetryDLQTask(ctx context.Context, taskID string) error {
	s.mu.Lock()
	var found *internalTask
	newDLQ := make([]*internalTask, 0, len(s.dlq))
	for _, it := range s.dlq {
		if it.ID == taskID {
			found = it
		} else {
			newDLQ = append(newDLQ, it)
		}
	}
	if found == nil {
		s.mu.Unlock()
		return ErrTaskNotFound
	}
	s.dlq = newDLQ
	found.Status = StatusPending
	found.Retried = 0
	found.LastErr = ""
	// Re-register the unique key if set.
	if found.UniqueKey != "" {
		s.uniqueKeyIndex[found.UniqueKey] = found.ID
	}
	s.mu.Unlock()

	ch := s.channelFor(found.Queue)
	select {
	case ch <- found:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.metrics.RecordEnqueue(found.Type)
	return nil
}

// DeleteDLQTask permanently removes a task from the dead-letter queue.
func (s *Server) DeleteDLQTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	newDLQ := make([]*internalTask, 0, len(s.dlq))
	found := false
	for _, it := range s.dlq {
		if it.ID == taskID {
			found = true
			delete(s.allTasks, it.ID)
			if it.UniqueKey != "" {
				delete(s.uniqueKeyIndex, it.UniqueKey)
			}
		} else {
			newDLQ = append(newDLQ, it)
		}
	}
	if !found {
		return ErrTaskNotFound
	}
	s.dlq = newDLQ
	return nil
}

// ─── Internal goroutines ──────────────────────────────────────────────────────

func (s *Server) runWorker(ctx context.Context, qName string, ch <-chan *internalTask) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case it, ok := <-ch:
			if !ok {
				return
			}
			s.processTask(ctx, it)
		}
	}
}

func (s *Server) processTask(ctx context.Context, it *internalTask) {
	started := time.Now()
	s.setStatus(it, StatusActive)

	// Capture retry count under read lock to avoid races with RetryDLQTask.
	s.mu.RLock()
	currentRetry := it.Retried
	s.mu.RUnlock()

	s.logger.Info("job started", "id", it.ID, "type", it.Type, "queue", it.Queue, "retry", currentRetry)

	err := it.handler(ctx, it.task)
	finished := time.Now()

	exec := JobExecution{
		TaskID:     it.ID,
		Type:       it.Type,
		Queue:      it.Queue,
		StartedAt:  started,
		FinishedAt: finished,
		Duration:   finished.Sub(started).Milliseconds(),
		Retried:    currentRetry,
	}

	if err == nil {
		now := finished
		s.mu.Lock()
		it.TaskInfo.CompletedAt = &now
		// Evict oldest completed task if cap exceeded.
		s.completedIDs = append(s.completedIDs, it.ID)
		if len(s.completedIDs) > maxCompletedHistory {
			evictID := s.completedIDs[0]
			s.completedIDs = s.completedIDs[1:]
			delete(s.allTasks, evictID)
		}
		if it.UniqueKey != "" {
			delete(s.uniqueKeyIndex, it.UniqueKey)
		}
		s.mu.Unlock()
		s.setStatus(it, StatusCompleted)
		exec.Status = StatusCompleted
		s.metrics.RecordSuccess(exec)
		s.logger.Info("job succeeded", "id", it.ID, "type", it.Type, "duration_ms", finished.Sub(started).Milliseconds())
		return
	}

	exec.Status = StatusFailed
	exec.Err = err.Error()
	s.metrics.RecordFailure(exec)
	s.logger.Warn("job failed", "id", it.ID, "type", it.Type, "error", err, "retried", currentRetry, "max_retries", it.MaxRetries)

	// Atomically update Retried and capture values for retry/DLQ decision.
	s.mu.Lock()
	it.LastErr = err.Error()
	it.Retried++
	retried := it.Retried
	maxRetries := it.MaxRetries
	s.mu.Unlock()

	if retried > maxRetries {
		s.setStatus(it, StatusDeadLetter)
		s.metrics.RecordDLQ(it.Type)
		s.mu.Lock()
		s.dlq = append(s.dlq, it)
		if it.UniqueKey != "" {
			delete(s.uniqueKeyIndex, it.UniqueKey)
		}
		s.mu.Unlock()
		s.logger.Error("job moved to DLQ", "id", it.ID, "type", it.Type, "retried", retried)
		return
	}

	// Schedule retry with backoff.
	policy := s.policyFor(it.Queue)
	backoff := s.backoffFor(policy, retried-1)
	s.setStatus(it, StatusRetrying)
	s.logger.Info("job scheduled for retry", "id", it.ID, "type", it.Type, "backoff", backoff)

	ch := s.channelFor(it.Queue)
	go func() {
		select {
		case <-time.After(backoff):
			ch <- it
		case <-ctx.Done():
			// Server shutting down during retry wait — move to DLQ.
			s.mu.Lock()
			it.LastErr = "server_shutdown"
			s.dlq = append(s.dlq, it)
			if it.UniqueKey != "" {
				delete(s.uniqueKeyIndex, it.UniqueKey)
			}
			s.mu.Unlock()
			s.setStatus(it, StatusDeadLetter)
		}
	}()
}

func (s *Server) setStatus(it *internalTask, status TaskStatus) {
	s.mu.Lock()
	it.Status = status
	s.mu.Unlock()
}

func (s *Server) policyFor(queue string) RetryPolicy {
	if p, ok := s.retryPolicies[queue]; ok {
		return p
	}
	return s.retryPolicies[QueueDefault]
}

func (s *Server) backoffFor(policy RetryPolicy, attempt int) time.Duration {
	if len(policy.Backoff) == 0 {
		return 30 * time.Second
	}
	if attempt >= len(policy.Backoff) {
		return policy.Backoff[len(policy.Backoff)-1]
	}
	return policy.Backoff[attempt]
}

func (s *Server) channelFor(queue string) chan *internalTask {
	if ch, ok := s.queues[queue]; ok {
		return ch
	}
	return s.queues[QueueDefault]
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
