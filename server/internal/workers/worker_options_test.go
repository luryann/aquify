package workers

import (
	"context"
	"testing"
	"time"
)

func TestEnqueueHonorsDelay(t *testing.T) {
	s := NewServer(ServerConfig{})
	defer s.Stop()
	s.RegisterHandler("test", func(context.Context, *Task) error { return nil })
	before := time.Now()
	info, err := s.Enqueue(context.Background(), &Task{Type: "test"}, WithDelay(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if info.ProcessAt.Before(before.Add(time.Hour)) {
		t.Fatalf("delay ignored: process_at=%s", info.ProcessAt)
	}
	if len(s.queues[QueueDefault]) != 0 {
		t.Fatal("delayed task dispatched immediately")
	}
}

func TestEnqueueInOverridesDelayOption(t *testing.T) {
	s := NewServer(ServerConfig{})
	defer s.Stop()
	s.RegisterHandler("test", func(context.Context, *Task) error { return nil })
	before := time.Now()
	info, err := s.EnqueueIn(context.Background(), time.Hour, &Task{Type: "test"}, WithDelay(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if info.ProcessAt.Before(before.Add(time.Hour)) || info.ProcessAt.After(before.Add(90*time.Minute)) {
		t.Fatalf("explicit delay lost: %s", info.ProcessAt)
	}
}

func TestEnqueueRejectsNilTask(t *testing.T) {
	s := NewServer(ServerConfig{})
	defer s.Stop()
	if _, err := s.Enqueue(context.Background(), nil); err == nil {
		t.Fatal("expected nil task error")
	}
}
