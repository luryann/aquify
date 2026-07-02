package workers

import (
	"testing"
	"time"
)

func TestJobMetricsSnapshotsAndHistoryLimit(t *testing.T) {
	t.Parallel()

	metrics := NewJobMetrics()
	metrics.RecordEnqueue(TypeParseImport)
	metrics.RecordSuccess(JobExecution{
		TaskID:     "task-1",
		Type:       TypeParseImport,
		Queue:      QueueImports,
		Status:     StatusCompleted,
		StartedAt:  time.Now().Add(-time.Second),
		FinishedAt: time.Now(),
		Duration:   1000,
	})
	metrics.RecordFailure(JobExecution{
		TaskID: "task-2",
		Type:   TypeParseImport,
		Queue:  QueueImports,
		Status: StatusFailed,
		Err:    "boom",
	})
	metrics.RecordDLQ(TypeParseImport)

	snapshot := metrics.TypeSnapshot()[TypeParseImport]
	if snapshot.Enqueued != 1 || snapshot.Succeeded != 1 || snapshot.Failed != 1 || snapshot.DLQ != 1 {
		t.Fatalf("TypeSnapshot() = %+v, want one count for each counter", snapshot)
	}

	recent := metrics.RecentExecutions(1)
	if len(recent) != 1 || recent[0].TaskID != "task-2" {
		t.Fatalf("RecentExecutions(1) = %+v, want most recent failure", recent)
	}
	all := metrics.RecentExecutions(0)
	if len(all) != 2 {
		t.Fatalf("RecentExecutions(0) len = %d, want all executions", len(all))
	}
}
