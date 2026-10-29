package workers

import (
	"sync"
	"sync/atomic"
	"time"
)

// JobMetrics tracks execution counters for each task type. All operations are
// safe for concurrent use.
type JobMetrics struct {
	mu      sync.RWMutex
	byType  map[string]*typeCounters
	history []JobExecution // rolling window — last 500 executions
}

type typeCounters struct {
	enqueued  atomic.Int64
	succeeded atomic.Int64
	failed    atomic.Int64
	dlq       atomic.Int64
}

// JobExecution is a single recorded task run, kept in the rolling history.
type JobExecution struct {
	TaskID     string     `json:"task_id"`
	Type       string     `json:"type"`
	Queue      string     `json:"queue"`
	Status     TaskStatus `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt time.Time  `json:"finished_at"`
	Duration   int64      `json:"duration_ms"` // wall-clock milliseconds
	Err        string     `json:"error,omitempty"`
	Retried    int        `json:"retried"`
}

// NewJobMetrics creates an empty metrics store.
func NewJobMetrics() *JobMetrics {
	return &JobMetrics{
		byType: make(map[string]*typeCounters),
	}
}

// counters returns the typeCounters for a task type, creating it if absent.
// Callers that may create entries must use this (acquires write lock).
func (m *JobMetrics) counters(taskType string) *typeCounters {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.byType[taskType]; ok {
		return c
	}
	c := &typeCounters{}
	m.byType[taskType] = c
	return c
}

// RecordEnqueue increments the enqueued counter for the task type.
func (m *JobMetrics) RecordEnqueue(taskType string) {
	m.counters(taskType).enqueued.Add(1)
}

// RecordSuccess increments the succeeded counter and appends to history.
func (m *JobMetrics) RecordSuccess(exec JobExecution) {
	m.counters(exec.Type).succeeded.Add(1)
	m.appendHistory(exec)
}

// RecordFailure increments the failed counter and appends to history.
func (m *JobMetrics) RecordFailure(exec JobExecution) {
	m.counters(exec.Type).failed.Add(1)
	m.appendHistory(exec)
}

// RecordDLQ increments the DLQ counter (task exhausted retries).
func (m *JobMetrics) RecordDLQ(taskType string) {
	m.counters(taskType).dlq.Add(1)
}

func (m *JobMetrics) appendHistory(exec JobExecution) {
	const maxHistory = 500
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, exec)
	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}
}

// TypeSnapshot returns a point-in-time snapshot of counters per task type.
func (m *JobMetrics) TypeSnapshot() map[string]TypeMetricSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]TypeMetricSnapshot, len(m.byType))
	for k, c := range m.byType {
		out[k] = TypeMetricSnapshot{
			Enqueued:  c.enqueued.Load(),
			Succeeded: c.succeeded.Load(),
			Failed:    c.failed.Load(),
			DLQ:       c.dlq.Load(),
		}
	}
	return out
}

// TypeMetricSnapshot is a point-in-time copy of one task type's counters.
type TypeMetricSnapshot struct {
	Enqueued  int64 `json:"enqueued"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	DLQ       int64 `json:"dlq"`
}

// RecentExecutions returns a copy of the rolling execution history.
func (m *JobMetrics) RecentExecutions(limit int) []JobExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := len(m.history)
	if limit <= 0 || limit > total {
		limit = total
	}
	// Return the most recent `limit` entries.
	out := make([]JobExecution, limit)
	copy(out, m.history[total-limit:])
	return out
}
