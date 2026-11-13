package workers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Listing must walk every page of a queue (asynq pages are 1-indexed, 100 per page)
// without skipping or repeating tasks. This runs against the real Redis from REDIS_URL
// on a throwaway queue name, because the paging maths lives inside asynq's Lua/ZSET
// scripts and an in-process fake would not prove it.
func TestRedisBackendListsEveryTaskOncePerStatusAcrossPages(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if url == "" {
		t.Skip("REDIS_URL is not set")
	}
	b, err := NewRedisBackend(RedisBackendConfig{RedisURL: url, Logger: cov1xQuietLogger(), Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	queue := "pagetest-" + uuid.NewString()
	t.Cleanup(func() { _ = b.inspector.DeleteQueue(queue, true) })
	ctx := context.Background()
	// Enqueue records job metrics under shared keys; use a private prefix so this test
	// neither pollutes the real counters nor leaves keys behind.
	metricsPrefix := "aquify:test:" + uuid.NewString()
	b.metrics = NewRedisJobMetrics(b.redisClient, metricsPrefix)
	t.Cleanup(func() {
		keys := []string{metricsPrefix + ":types", metricsPrefix + ":history", metricsPrefix + ":type:pagetest"}
		if err := b.redisClient.Del(context.Background(), keys...).Err(); err != nil {
			t.Errorf("remove test metrics keys: %v", err)
		}
	})
	task, err := NewTask("pagetest", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	const pending, scheduled, archived = 250, 130, 120
	for i := 0; i < pending; i++ {
		if _, err := b.Enqueue(ctx, task, WithQueue(queue)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < scheduled; i++ {
		if _, err := b.EnqueueIn(ctx, time.Hour, task, WithQueue(queue)); err != nil {
			t.Fatal(err)
		}
	}
	// Archive a third batch of scheduled tasks so the dead-letter paging is exercised too.
	var toArchive []string
	for i := 0; i < archived; i++ {
		info, err := b.EnqueueIn(ctx, 2*time.Hour, task, WithQueue(queue))
		if err != nil {
			t.Fatal(err)
		}
		toArchive = append(toArchive, info.ID)
	}
	for _, id := range toArchive {
		if err := b.inspector.ArchiveTask(queue, id); err != nil {
			t.Fatal(err)
		}
	}

	assertExactlyOnce := func(label string, got []TaskInfo, want int) {
		t.Helper()
		seen := make(map[string]bool, len(got))
		for _, ti := range got {
			if seen[ti.ID] {
				t.Fatalf("%s: task %s listed twice", label, ti.ID)
			}
			seen[ti.ID] = true
		}
		if len(got) != want {
			t.Fatalf("%s: listed %d tasks, want %d", label, len(got), want)
		}
	}
	// The pending view is the pending list plus the scheduled list: two lists, each
	// spanning several pages, and none of the archived tasks.
	got, err := b.ListTasks(queue, StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	assertExactlyOnce("pending+scheduled", got, pending+scheduled)

	dlq, err := b.DLQTasks()
	if err != nil {
		t.Fatal(err)
	}
	var mine []TaskInfo
	for _, ti := range dlq {
		if ti.Queue == queue {
			mine = append(mine, ti)
		}
	}
	assertExactlyOnce("dead-letter", mine, archived)

	stats, err := b.QueueStats()
	if err != nil {
		t.Fatal(err)
	}
	var qs *QueueStats
	for i := range stats {
		if stats[i].Name == queue {
			qs = &stats[i]
		}
	}
	if qs == nil {
		t.Fatalf("queue %s missing from stats %+v", queue, stats)
	}
	if qs.Pending != pending+scheduled || qs.DeadLetter != archived {
		t.Fatalf("stats = %+v, want pending(+scheduled) %d and dead-letter %d", *qs, pending+scheduled, archived)
	}
}

// A queue nobody has enqueued to yet has no tasks; it is not an error (the admin queue
// views list queues by name, including ones that only exist in configuration).
func TestRedisBackendListsUnknownQueueAsEmpty(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if url == "" {
		t.Skip("REDIS_URL is not set")
	}
	b, err := NewRedisBackend(RedisBackendConfig{RedisURL: url, Logger: cov1xQuietLogger(), Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	queue := "never-used-" + uuid.NewString()
	for _, status := range []TaskStatus{StatusPending, StatusActive, StatusRetrying, StatusCompleted, StatusDeadLetter} {
		got, err := b.ListTasks(queue, status)
		if err != nil || len(got) != 0 {
			t.Fatalf("status %s on an unknown queue: %v err %v", status, got, err)
		}
	}
	// Listing must not have created the queue as a side effect.
	queues, err := b.inspector.Queues()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queues {
		if q == queue {
			t.Fatalf("listing created queue %q", queue)
		}
	}
	// Other failures are still surfaced: a closed backend cannot list.
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ListTasks(queue, StatusPending); err == nil {
		t.Fatal("a closed backend must report an error, not an empty list")
	}
}
