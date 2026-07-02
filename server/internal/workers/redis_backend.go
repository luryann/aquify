package workers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const defaultTaskRetention = 7 * 24 * time.Hour

type RedisBackendConfig struct {
	RedisURL    string
	Logger      *slog.Logger
	Concurrency int
	Retention   time.Duration
}

// RedisBackend provides durable background jobs using Redis + Asynq.
// It can be used by API processes for enqueue/admin access and by worker
// processes for task execution once Start is called.
type RedisBackend struct {
	logger      *slog.Logger
	redisURL    string
	redisOpt    asynq.RedisConnOpt
	redisClient redis.UniversalClient
	client      *asynq.Client
	inspector   *asynq.Inspector
	metrics     *RedisJobMetrics
	concurrency int
	retention   time.Duration
	mu          sync.Mutex
	handlers    map[string]Handler
	scheduled   []ScheduledJob
	server      *asynq.Server
	scheduler   *asynq.Scheduler
	started     bool
}

func NewRedisBackend(cfg RedisBackendConfig) (*RedisBackend, error) {
	if strings.TrimSpace(cfg.RedisURL) == "" {
		return nil, fmt.Errorf("redis backend: redis url is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.Retention <= 0 {
		cfg.Retention = defaultTaskRetention
	}

	opt, err := asynq.ParseRedisURI(strings.TrimSpace(cfg.RedisURL))
	if err != nil {
		return nil, fmt.Errorf("redis backend: parse redis url: %w", err)
	}
	redisClient, err := redis.ParseURL(strings.TrimSpace(cfg.RedisURL))
	if err != nil {
		return nil, fmt.Errorf("redis backend: parse redis client url: %w", err)
	}
	client := redis.NewClient(redisClient)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis backend: ping: %w", err)
	}

	return &RedisBackend{
		logger:      cfg.Logger,
		redisURL:    strings.TrimSpace(cfg.RedisURL),
		redisOpt:    opt,
		redisClient: client,
		client:      asynq.NewClient(opt),
		inspector:   asynq.NewInspector(opt),
		metrics:     NewRedisJobMetrics(client, ""),
		concurrency: cfg.Concurrency,
		retention:   cfg.Retention,
		handlers:    make(map[string]Handler),
	}, nil
}

func (b *RedisBackend) RegisterHandler(taskType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[taskType] = h
}

func (b *RedisBackend) RegisterScheduledJob(job ScheduledJob) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scheduled = append(b.scheduled, job)
}

func (b *RedisBackend) Register(job ScheduledJob) {
	b.RegisterScheduledJob(job)
}

func (b *RedisBackend) Metrics() MetricsReader {
	return b.metrics
}

func (b *RedisBackend) Ready(ctx context.Context) error {
	if b.redisClient == nil {
		return fmt.Errorf("redis client unavailable")
	}
	return b.redisClient.Ping(ctx).Err()
}

func (b *RedisBackend) Enqueue(ctx context.Context, task *Task, opts ...Option) (*TaskInfo, error) {
	return b.enqueue(ctx, time.Time{}, task, opts...)
}

func (b *RedisBackend) EnqueueIn(ctx context.Context, delay time.Duration, task *Task, opts ...Option) (*TaskInfo, error) {
	processAt := time.Now().Add(delay)
	return b.enqueue(ctx, processAt, task, opts...)
}

func (b *RedisBackend) enqueue(ctx context.Context, processAt time.Time, task *Task, opts ...Option) (*TaskInfo, error) {
	if task == nil {
		return nil, fmt.Errorf("redis backend: task is required")
	}
	o := &EnqueueOptions{Queue: QueueDefault}
	for _, opt := range opts {
		opt(o)
	}
	if !processAt.IsZero() {
		o.ProcessIn = time.Until(processAt)
	}

	asynqTask := asynq.NewTask(task.Type, task.Payload)
	asynqOpts := []asynq.Option{
		asynq.Queue(defaultQueue(o.Queue)),
		asynq.Retention(b.retention),
	}
	if o.MaxRetries != nil {
		asynqOpts = append(asynqOpts, asynq.MaxRetry(*o.MaxRetries))
	} else {
		asynqOpts = append(asynqOpts, asynq.MaxRetry(b.maxRetriesForQueue(defaultQueue(o.Queue))))
	}
	if o.ProcessIn > 0 {
		asynqOpts = append(asynqOpts, asynq.ProcessIn(o.ProcessIn))
	}
	if o.UniqueKey != "" {
		asynqOpts = append(asynqOpts, asynq.Unique(24*time.Hour))
	}

	info, err := b.client.EnqueueContext(ctx, asynqTask, asynqOpts...)
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return nil, ErrDuplicateTask
		}
		return nil, err
	}
	b.metrics.RecordEnqueue(task.Type)
	return convertAsynqTaskInfo(info), nil
}

func (b *RedisBackend) ListTasks(queue string, status TaskStatus) ([]TaskInfo, error) {
	queues, err := b.queueNames(queue)
	if err != nil {
		return nil, err
	}
	var out []TaskInfo
	for _, q := range queues {
		items, err := b.listQueueTasks(q, status)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProcessAt.After(out[j].ProcessAt)
	})
	return out, nil
}

func (b *RedisBackend) QueueStats() ([]QueueStats, error) {
	queues, err := b.queueNames("")
	if err != nil {
		return nil, err
	}
	out := make([]QueueStats, 0, len(queues))
	for _, q := range queues {
		pending, err := b.collectTaskInfos(q, b.inspector.ListPendingTasks)
		if err != nil {
			return nil, err
		}
		scheduled, err := b.collectTaskInfos(q, b.inspector.ListScheduledTasks)
		if err != nil {
			return nil, err
		}
		active, err := b.collectTaskInfos(q, b.inspector.ListActiveTasks)
		if err != nil {
			return nil, err
		}
		retrying, err := b.collectTaskInfos(q, b.inspector.ListRetryTasks)
		if err != nil {
			return nil, err
		}
		completed, err := b.collectTaskInfos(q, b.inspector.ListCompletedTasks)
		if err != nil {
			return nil, err
		}
		archived, err := b.collectTaskInfos(q, b.inspector.ListArchivedTasks)
		if err != nil {
			return nil, err
		}
		out = append(out, QueueStats{
			Name:       q,
			Pending:    len(pending) + len(scheduled) + len(retrying),
			Active:     len(active),
			Completed:  len(completed),
			Failed:     len(retrying),
			DeadLetter: len(archived),
		})
	}
	return out, nil
}

func (b *RedisBackend) DLQTasks() ([]TaskInfo, error) {
	queues, err := b.queueNames("")
	if err != nil {
		return nil, err
	}
	var out []TaskInfo
	for _, q := range queues {
		items, err := b.collectTaskInfos(q, b.inspector.ListArchivedTasks)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			out = append(out, *convertAsynqTaskInfo(item))
		}
	}
	return out, nil
}

func (b *RedisBackend) RetryDLQTask(_ context.Context, taskID string) error {
	queue, _, err := b.findTask(taskID)
	if err != nil {
		return err
	}
	if err := b.inspector.RunTask(queue, taskID); err != nil {
		return normalizeAsynqTaskError(err)
	}
	return nil
}

func (b *RedisBackend) DeleteDLQTask(taskID string) error {
	queue, _, err := b.findTask(taskID)
	if err != nil {
		return err
	}
	if err := b.inspector.DeleteTask(queue, taskID); err != nil {
		return normalizeAsynqTaskError(err)
	}
	return nil
}

func (b *RedisBackend) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return nil
	}

	mux := asynq.NewServeMux()
	for taskType, handler := range b.handlers {
		mux.HandleFunc(taskType, b.wrapHandler(taskType, handler))
	}
	b.server = asynq.NewServer(b.redisOpt, asynq.Config{
		Concurrency: b.concurrency,
		Queues: map[string]int{
			QueueImports:   1,
			QueueBilling:   1,
			QueueEmail:     1,
			QueueReconcile: 1,
			QueueDefault:   1,
		},
	})
	if err := b.server.Start(mux); err != nil {
		return err
	}

	if len(b.scheduled) > 0 {
		scheduler := asynq.NewScheduler(b.redisOpt, &asynq.SchedulerOpts{})
		for _, job := range b.scheduled {
			if job.Interval <= 0 {
				continue
			}
			task, err := NewTask(job.TaskType, job.Payload)
			if err != nil {
				return err
			}
			if _, err := b.enqueue(context.Background(), time.Time{}, task, WithQueue(job.Queue)); err != nil && !errors.Is(err, ErrDuplicateTask) {
				return fmt.Errorf("enqueue initial scheduled job %q: %w", job.Name, err)
			}
			_, err = scheduler.Register(
				fmt.Sprintf("@every %s", job.Interval),
				asynq.NewTask(task.Type, task.Payload),
				asynq.Queue(defaultQueue(job.Queue)),
				asynq.Retention(b.retention),
			)
			if err != nil {
				return err
			}
		}
		if err := scheduler.Start(); err != nil {
			return err
		}
		b.scheduler = scheduler
	}

	b.started = true
	return nil
}

func (b *RedisBackend) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.scheduler != nil {
		b.scheduler.Shutdown()
		b.scheduler = nil
	}
	if b.server != nil {
		b.server.Shutdown()
		b.server = nil
	}
	b.started = false
	return nil
}

func (b *RedisBackend) Close() error {
	if err := b.Stop(context.Background()); err != nil {
		return err
	}
	if b.inspector != nil {
		_ = b.inspector.Close()
	}
	if b.client != nil {
		_ = b.client.Close()
	}
	if b.redisClient != nil {
		return b.redisClient.Close()
	}
	return nil
}

func (b *RedisBackend) wrapHandler(taskType string, handler Handler) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		started := time.Now().UTC()
		err := handler(ctx, &Task{
			Type:    task.Type(),
			Payload: task.Payload(),
		})
		finished := time.Now().UTC()
		taskID, _ := asynq.GetTaskID(ctx)
		queueName, _ := asynq.GetQueueName(ctx)
		retryCount, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)

		exec := JobExecution{
			TaskID:     taskID,
			Type:       taskType,
			Queue:      queueName,
			StartedAt:  started,
			FinishedAt: finished,
			Duration:   finished.Sub(started).Milliseconds(),
			Retried:    retryCount,
		}
		if err == nil {
			exec.Status = StatusCompleted
			b.metrics.RecordSuccess(exec)
			return nil
		}
		exec.Status = StatusFailed
		exec.Err = err.Error()
		b.metrics.RecordFailure(exec)
		if retryCount >= maxRetry {
			b.metrics.RecordDLQ(taskType)
		}
		return err
	}
}

func (b *RedisBackend) listQueueTasks(queue string, status TaskStatus) ([]TaskInfo, error) {
	var batches [][]*asynq.TaskInfo
	switch status {
	case StatusPending:
		pending, err := b.collectTaskInfos(queue, b.inspector.ListPendingTasks)
		if err != nil {
			return nil, err
		}
		scheduled, err := b.collectTaskInfos(queue, b.inspector.ListScheduledTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, pending, scheduled)
	case StatusActive:
		items, err := b.collectTaskInfos(queue, b.inspector.ListActiveTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, items)
	case StatusRetrying:
		items, err := b.collectTaskInfos(queue, b.inspector.ListRetryTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, items)
	case StatusCompleted:
		items, err := b.collectTaskInfos(queue, b.inspector.ListCompletedTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, items)
	case StatusDeadLetter:
		items, err := b.collectTaskInfos(queue, b.inspector.ListArchivedTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, items)
	case StatusFailed:
		items, err := b.collectTaskInfos(queue, b.inspector.ListRetryTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, items)
	default:
		pending, err := b.collectTaskInfos(queue, b.inspector.ListPendingTasks)
		if err != nil {
			return nil, err
		}
		scheduled, err := b.collectTaskInfos(queue, b.inspector.ListScheduledTasks)
		if err != nil {
			return nil, err
		}
		active, err := b.collectTaskInfos(queue, b.inspector.ListActiveTasks)
		if err != nil {
			return nil, err
		}
		retrying, err := b.collectTaskInfos(queue, b.inspector.ListRetryTasks)
		if err != nil {
			return nil, err
		}
		completed, err := b.collectTaskInfos(queue, b.inspector.ListCompletedTasks)
		if err != nil {
			return nil, err
		}
		archived, err := b.collectTaskInfos(queue, b.inspector.ListArchivedTasks)
		if err != nil {
			return nil, err
		}
		batches = append(batches, pending, scheduled, active, retrying, completed, archived)
	}

	out := make([]TaskInfo, 0)
	for _, batch := range batches {
		for _, item := range batch {
			out = append(out, *convertAsynqTaskInfo(item))
		}
	}
	return out, nil
}

func (b *RedisBackend) collectTaskInfos(queue string, listFn func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error)) ([]*asynq.TaskInfo, error) {
	var out []*asynq.TaskInfo
	page := 0
	for {
		items, err := listFn(queue, asynq.Page(page), asynq.PageSize(100))
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) < 100 {
			return out, nil
		}
		page++
	}
}

func (b *RedisBackend) queueNames(queue string) ([]string, error) {
	if q := strings.TrimSpace(queue); q != "" {
		return []string{q}, nil
	}
	queues, err := b.inspector.Queues()
	if err != nil {
		return nil, err
	}
	if len(queues) == 0 {
		return []string{QueueImports, QueueBilling, QueueEmail, QueueReconcile, QueueDefault}, nil
	}
	sort.Strings(queues)
	return queues, nil
}

func (b *RedisBackend) findTask(taskID string) (string, *asynq.TaskInfo, error) {
	queues, err := b.queueNames("")
	if err != nil {
		return "", nil, err
	}
	for _, queue := range queues {
		task, err := b.inspector.GetTaskInfo(queue, taskID)
		if err == nil {
			return queue, task, nil
		}
	}
	return "", nil, ErrTaskNotFound
}

func convertAsynqTaskInfo(info *asynq.TaskInfo) *TaskInfo {
	if info == nil {
		return nil
	}
	status := StatusPending
	switch info.State {
	case asynq.TaskStateActive:
		status = StatusActive
	case asynq.TaskStatePending, asynq.TaskStateScheduled:
		status = StatusPending
	case asynq.TaskStateRetry:
		status = StatusRetrying
	case asynq.TaskStateArchived:
		status = StatusDeadLetter
	case asynq.TaskStateCompleted:
		status = StatusCompleted
	}
	out := &TaskInfo{
		ID:         info.ID,
		Type:       info.Type,
		Queue:      info.Queue,
		Status:     status,
		ProcessAt:  info.NextProcessAt.UTC(),
		MaxRetries: info.MaxRetry,
		Retried:    info.Retried,
		LastErr:    info.LastErr,
	}
	if completedAt := normalizeTimestamp(info.CompletedAt); completedAt != nil {
		out.CompletedAt = completedAt
	}
	return out
}

func normalizeAsynqTaskError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return ErrTaskNotFound
	}
	return err
}

func defaultQueue(queue string) string {
	if strings.TrimSpace(queue) == "" {
		return QueueDefault
	}
	return strings.TrimSpace(queue)
}

func (b *RedisBackend) maxRetriesForQueue(queue string) int {
	policies := DefaultRetryPolicies()
	if policy, ok := policies[queue]; ok {
		return policy.MaxRetries
	}
	return policies[QueueDefault].MaxRetries
}
