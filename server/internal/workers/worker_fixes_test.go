package workers

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestEnqueueEmptyQueueNameUsesDefault(t *testing.T) {
	s := NewServer(ServerConfig{})
	defer s.Stop()
	s.RegisterHandler("test", func(context.Context, *Task) error { return nil })
	info, err := s.Enqueue(context.Background(), &Task{Type: "test"}, WithQueue(""))
	if err != nil {
		t.Fatal(err)
	}
	if info.Queue != QueueDefault {
		t.Fatalf("queue = %q, want %q", info.Queue, QueueDefault)
	}
	if got, _ := s.ListTasks(QueueDefault, ""); len(got) != 1 {
		t.Fatalf("task not listed under default queue: %d", len(got))
	}
}

func TestRetryDLQTaskKeepsTaskWhenDispatchCanceled(t *testing.T) {
	s := NewServer(ServerConfig{}) // not started: queue channels are never drained
	defer s.Stop()
	s.RegisterHandler("test", func(context.Context, *Task) error { return nil })
	it := &internalTask{TaskInfo: TaskInfo{ID: "dead-1", Type: "test", Queue: QueueDefault, Status: StatusDeadLetter}}
	s.allTasks[it.ID] = it
	s.dlq = append(s.dlq, it)
	ch := s.queues[QueueDefault]
	for len(ch) < cap(ch) {
		ch <- &internalTask{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.RetryDLQTask(ctx, it.ID); err == nil {
		t.Fatal("expected context error")
	}
	if tasks, _ := s.DLQTasks(); len(tasks) != 1 || tasks[0].ID != it.ID || tasks[0].Status != StatusDeadLetter {
		t.Fatalf("task lost from DLQ after failed retry: %+v", tasks)
	}
}

func TestRedisDLQActionsOnlyTouchArchivedTasks(t *testing.T) {
	mr := miniredis.RunT(t)
	b, err := NewRedisBackend(RedisBackendConfig{RedisURL: "redis://" + mr.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	task, _ := NewTask("test", map[string]string{})
	info, err := b.EnqueueIn(context.Background(), time.Hour, task, WithQueue(QueueDefault))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteDLQTask(info.ID); err != ErrTaskNotFound {
		t.Fatalf("DeleteDLQTask on scheduled task = %v, want ErrTaskNotFound", err)
	}
	if err := b.RetryDLQTask(context.Background(), info.ID); err != ErrTaskNotFound {
		t.Fatalf("RetryDLQTask on scheduled task = %v, want ErrTaskNotFound", err)
	}
	if got, err := b.inspector.GetTaskInfo(QueueDefault, info.ID); err != nil || got.State != asynq.TaskStateScheduled {
		t.Fatalf("scheduled task was disturbed: %v %+v", err, got)
	}

	if err := b.inspector.ArchiveTask(QueueDefault, info.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteDLQTask(info.ID); err != nil {
		t.Fatalf("DeleteDLQTask on archived task: %v", err)
	}
}
