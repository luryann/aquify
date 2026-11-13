package workers

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
)

func TestCov1xServerHonoursPerTaskMaxRetries(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger(), RetryPolicies: cov1xFastPolicy(5, time.Millisecond)})
	var calls atomic.Int32
	s.RegisterHandler("bad", func(context.Context, *Task) error { calls.Add(1); return errors.New("x") })
	s.Start()
	defer s.Stop()
	info, err := s.Enqueue(context.Background(), &Task{Type: "bad"}, WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	if info.MaxRetries != 0 {
		t.Fatalf("max retries = %d", info.MaxRetries)
	}
	cov1xWaitFor(t, "dead letter", func() bool { d, _ := s.DLQTasks(); return len(d) == 1 })
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want exactly 1 with zero retries", calls.Load())
	}
}

func TestCov1xRunWorkerExitsWhenQueueChannelClosed(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	ch := make(chan *internalTask)
	close(ch)
	done := make(chan struct{})
	s.wg.Add(1)
	go func() { s.runWorker(s.ctx, QueueDefault, ch); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runWorker did not exit on closed channel")
	}
}

// When the task disappears from the archive between lookup and action (here:
// Redis starts failing script calls), the action's error is surfaced.
func TestCov1xDLQActionErrorsAreSurfaced(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	ctx := context.Background()
	task, _ := NewTask("t", map[string]string{})

	var evals atomic.Int32
	var failFrom atomic.Int32 // 0 = never fail
	mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
		if strings.HasPrefix(strings.ToLower(cmd), "eval") {
			n := evals.Add(1)
			if f := failFrom.Load(); f > 0 && n >= f {
				c.WriteError("ERR injected script failure")
				return true
			}
		}
		return false
	})

	archive := func() string {
		info, err := b.EnqueueIn(ctx, time.Hour, task)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.inspector.ArchiveTask(QueueDefault, info.ID); err != nil {
			t.Fatal(err)
		}
		return info.ID
	}
	idRetry, idDelete := archive(), archive()
	// Warm the lookup script so it is a single EVALSHA from here on.
	if _, err := b.inspector.GetTaskInfo(QueueDefault, idRetry); err != nil {
		t.Fatal(err)
	}

	// Each action performs one lookup script then one mutating script; fail the second.
	evals.Store(0)
	failFrom.Store(2)
	err := b.RetryDLQTask(ctx, idRetry)
	if err == nil || errors.Is(err, ErrTaskNotFound) || !strings.Contains(err.Error(), "injected script failure") {
		t.Fatalf("RetryDLQTask err = %v", err)
	}
	evals.Store(0)
	err = b.DeleteDLQTask(idDelete)
	if err == nil || errors.Is(err, ErrTaskNotFound) || !strings.Contains(err.Error(), "injected script failure") {
		t.Fatalf("DeleteDLQTask err = %v", err)
	}

	// Both tasks are untouched and still archived once Redis recovers.
	failFrom.Store(0)
	dlq, err := b.DLQTasks()
	if err != nil || len(dlq) != 2 {
		t.Fatalf("dlq = %+v err %v", dlq, err)
	}
}
