package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ScheduledJob defines a recurring task.
type ScheduledJob struct {
	// Name is a human-readable identifier for logging.
	Name string
	// TaskType is the registered handler task type to enqueue.
	TaskType string
	// Queue is the target queue name.
	Queue string
	// Interval is how often the job fires. A zero value disables the job.
	Interval time.Duration
	// Payload is static JSON payload passed to the handler on each invocation.
	Payload any
}

// Scheduler manages periodic task enqueueing.
type Scheduler struct {
	mu     sync.Mutex
	jobs   []ScheduledJob
	queue  Queue
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates a Scheduler that enqueues jobs via the given Queue.
func NewScheduler(q Queue, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{queue: q, logger: logger}
}

// Register adds a job to the scheduler. Must be called before Start.
func (s *Scheduler) Register(job ScheduledJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
}

// Start launches a ticker goroutine for every registered job.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.mu.Lock()
	jobs := make([]ScheduledJob, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.Unlock()

	for _, job := range jobs {
		if job.Interval <= 0 {
			continue
		}
		job := job // capture loop var
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(job.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.fire(ctx, job)
				}
			}
		}()
	}
	s.logger.Info("scheduler started", "jobs", len(jobs))
}

// Stop signals all scheduler goroutines to exit.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("scheduler stopped")
}

func (s *Scheduler) fire(ctx context.Context, job ScheduledJob) {
	task, err := NewTask(job.TaskType, job.Payload)
	if err != nil {
		s.logger.Error("scheduler: build task", "job", job.Name, "error", err)
		return
	}
	_, err = s.queue.Enqueue(ctx, task, WithQueue(job.Queue))
	if err != nil {
		s.logger.Error("scheduler: enqueue", "job", job.Name, "error", err)
		return
	}
	s.logger.Info("scheduler: enqueued", "job", job.Name, "type", job.TaskType)
}
