package workers

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// Several instances starting against one Redis must not multiply the initial
// scheduled-job enqueue.
func TestRedisStartDedupesInitialScheduledJobAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	var backends []*RedisBackend
	for i := 0; i < 3; i++ {
		b, err := NewRedisBackend(RedisBackendConfig{RedisURL: "redis://" + mr.Addr()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		b.RegisterHandler("sched-test", func(context.Context, *Task) error { return nil })
		b.RegisterScheduledJob(ScheduledJob{Name: "j", TaskType: "sched-test", Queue: QueueDefault, Interval: time.Hour, Payload: map[string]string{"org_id": "o"}})
		backends = append(backends, b)
	}
	for _, b := range backends {
		if err := b.Start(); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := backends[0].ListTasks(QueueDefault, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("want 1 initial scheduled task across 3 instances, got %d", len(tasks))
	}
	if tasks[0].OrgID != "o" {
		t.Fatalf("org id not surfaced from payload: %q", tasks[0].OrgID)
	}
}

func TestScheduledTaskIDIsDeterministicPerBucket(t *testing.T) {
	j := ScheduledJob{Name: "n", TaskType: "t"}
	if scheduledTaskID(j, "tick", 5) != scheduledTaskID(j, "tick", 5) || scheduledTaskID(j, "tick", 5) == scheduledTaskID(j, "tick", 6) {
		t.Fatal("task id must be stable within a bucket and differ across buckets")
	}
}
