package workers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type captureQueue struct {
	mu    sync.Mutex
	tasks []*Task
}

func (c *captureQueue) Enqueue(_ context.Context, t *Task, _ ...Option) (*TaskInfo, error) {
	c.mu.Lock()
	c.tasks = append(c.tasks, t)
	c.mu.Unlock()
	return &TaskInfo{}, nil
}
func (c *captureQueue) EnqueueIn(ctx context.Context, _ time.Duration, t *Task, o ...Option) (*TaskInfo, error) {
	return c.Enqueue(ctx, t, o...)
}

func TestSchedulerFanOutFollowsLiveOrgSet(t *testing.T) {
	q := &captureQueue{}
	s := NewScheduler(q, nil)
	orgs := []string{"a"}
	var mu sync.Mutex
	job := ScheduledJob{Name: "j", TaskType: "x", Queue: QueueDefault, Interval: time.Hour,
		FanOut: func(context.Context) ([]FanOutItem, error) {
			mu.Lock()
			defer mu.Unlock()
			items := []FanOutItem{}
			for _, o := range orgs {
				items = append(items, FanOutItem{Key: o, Payload: map[string]string{"org_id": o}})
			}
			return items, nil
		}}
	s.fire(context.Background(), job)
	mu.Lock()
	orgs = []string{"a", "b"} // an org is provisioned between firings
	mu.Unlock()
	s.fire(context.Background(), job)

	got := map[string]int{}
	for _, task := range q.tasks {
		var p map[string]string
		_ = json.Unmarshal(task.Payload, &p)
		got[p["org_id"]]++
	}
	if got["a"] != 2 || got["b"] != 1 || len(q.tasks) != 3 {
		t.Fatalf("fan-out enqueues: %v (%d tasks)", got, len(q.tasks))
	}
}
