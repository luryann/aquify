package workers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type cov1xSafeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *cov1xSafeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *cov1xSafeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func cov1xNewBackend(t *testing.T, mr *miniredis.Miniredis, logger *slog.Logger) *RedisBackend {
	t.Helper()
	b, err := NewRedisBackend(RedisBackendConfig{RedisURL: " redis://" + mr.Addr() + " ", Logger: logger, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestCov1xNewRedisBackendValidation(t *testing.T) {
	if _, err := NewRedisBackend(RedisBackendConfig{RedisURL: "  "}); err == nil || !strings.Contains(err.Error(), "redis url is required") {
		t.Fatalf("empty url err = %v", err)
	}
	if _, err := NewRedisBackend(RedisBackendConfig{RedisURL: "http://localhost:6379"}); err == nil || !strings.Contains(err.Error(), "parse redis url") {
		t.Fatalf("bad scheme err = %v", err)
	}
	// Accepted by asynq's parser but not by go-redis'.
	if _, err := NewRedisBackend(RedisBackendConfig{RedisURL: "redis-socket:///tmp/cov1x.sock"}); err == nil || !strings.Contains(err.Error(), "parse redis client url") {
		t.Fatalf("socket scheme err = %v", err)
	}
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()
	if _, err := NewRedisBackend(RedisBackendConfig{RedisURL: "redis://" + addr}); err == nil || !strings.Contains(err.Error(), "ping") {
		t.Fatalf("unreachable redis err = %v", err)
	}
}

func TestCov1xBackendAccessorsAndReady(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	if b.Metrics() == nil {
		t.Fatal("metrics missing")
	}
	if err := b.Ready(context.Background()); err != nil {
		t.Fatalf("ready: %v", err)
	}
	b.Register(ScheduledJob{Name: "n", TaskType: "t"})
	if len(b.scheduled) != 1 || b.scheduled[0].Name != "n" {
		t.Fatalf("scheduled = %+v", b.scheduled)
	}
	if err := (&RedisBackend{}).Ready(context.Background()); err == nil || err.Error() != "redis client unavailable" {
		t.Fatalf("nil client err = %v", err)
	}
	if err := (&RedisBackend{}).Close(); err != nil {
		t.Fatalf("close of empty backend: %v", err)
	}
	mr.Close()
	if err := b.Ready(context.Background()); err == nil {
		t.Fatal("ready should fail when redis is down")
	}
}

func TestCov1xEnqueueOptionsAndErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	ctx := context.Background()
	if _, err := b.Enqueue(ctx, nil); err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("nil task err = %v", err)
	}
	task, _ := NewTask("t", map[string]string{"org_id": "o1"})
	info, err := b.Enqueue(ctx, task, WithQueue(QueueEmail), WithMaxRetries(9))
	if err != nil {
		t.Fatal(err)
	}
	if info.Queue != QueueEmail || info.MaxRetries != 9 || info.OrgID != "o1" || info.Status != StatusPending {
		t.Fatalf("info = %+v", info)
	}
	def, err := b.Enqueue(ctx, task, WithQueue("  "))
	if err != nil {
		t.Fatal(err)
	}
	if def.Queue != QueueDefault || def.MaxRetries != DefaultRetryPolicies()[QueueDefault].MaxRetries {
		t.Fatalf("default info = %+v", def)
	}
	billing, _ := b.Enqueue(ctx, task, WithQueue(QueueBilling))
	if billing.MaxRetries != DefaultRetryPolicies()[QueueBilling].MaxRetries {
		t.Fatalf("billing retries = %d", billing.MaxRetries)
	}
	if got := b.maxRetriesForQueue("no-such-queue"); got != DefaultRetryPolicies()[QueueDefault].MaxRetries {
		t.Fatalf("unknown queue retries = %d", got)
	}

	if _, err := b.Enqueue(ctx, task, WithUniqueKey("uk")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Enqueue(ctx, task, WithUniqueKey("uk")); !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("duplicate err = %v", err)
	}
	delayed, err := b.EnqueueIn(ctx, time.Hour, task, WithQueue(QueueReconcile))
	if err != nil {
		t.Fatal(err)
	}
	if delayed.ProcessAt.Before(time.Now().Add(50 * time.Minute)) {
		t.Fatalf("process_at not delayed: %v", delayed.ProcessAt)
	}
	if snap := b.Metrics().TypeSnapshot()["t"]; snap.Enqueued != 5 {
		t.Fatalf("enqueued metric = %+v", snap)
	}

	// Any other enqueue failure is returned untouched.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := b.Enqueue(cctx, task); err == nil || errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("canceled enqueue err = %v", err)
	}
}

func TestCov1xListingStatsAndPagination(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	ctx := context.Background()

	// Empty Redis: stats report every known queue with zero counts.
	stats, err := b.QueueStats()
	if err != nil || len(stats) != 5 {
		t.Fatalf("empty stats %v err %v", stats, err)
	}
	if d, err := b.DLQTasks(); err != nil || len(d) != 0 {
		t.Fatalf("empty dlq %v err %v", d, err)
	}

	task, _ := NewTask("t", map[string]string{})
	const n = 101
	for i := 0; i < n; i++ {
		if _, err := b.Enqueue(ctx, task, WithQueue(QueueImports)); err != nil {
			t.Fatal(err)
		}
	}
	sched, err := b.EnqueueIn(ctx, time.Hour, task, WithQueue(QueueEmail))
	if err != nil {
		t.Fatal(err)
	}
	// Second page is only fetched because the first one was full.
	got, err := b.ListTasks(QueueImports, StatusPending)
	if err != nil || len(got) != n {
		t.Fatalf("pending listing = %d err %v", len(got), err)
	}
	all, err := b.ListTasks("", "")
	if err != nil || len(all) != n+1 {
		t.Fatalf("all listing = %d err %v", len(all), err)
	}
	for _, status := range []TaskStatus{StatusActive, StatusRetrying, StatusFailed, StatusCompleted, StatusDeadLetter} {
		if l, err := b.ListTasks("", status); err != nil || len(l) != 0 {
			t.Fatalf("status %s: %v err %v", status, l, err)
		}
	}

	stats, err = b.QueueStats()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]QueueStats{}
	for _, s := range stats {
		byName[s.Name] = s
	}
	if len(stats) != 2 || byName[QueueImports].Pending != n || byName[QueueEmail].Pending != 1 {
		t.Fatalf("stats = %+v", stats)
	}

	// Archive the scheduled task, so it shows up in the DLQ and its stats.
	if err := b.inspector.ArchiveTask(QueueEmail, sched.ID); err != nil {
		t.Fatal(err)
	}
	dlq, err := b.DLQTasks()
	if err != nil || len(dlq) != 1 || dlq[0].ID != sched.ID || dlq[0].Status != StatusDeadLetter {
		t.Fatalf("dlq = %+v err %v", dlq, err)
	}
	stats, _ = b.QueueStats()
	for _, s := range stats {
		if s.Name == QueueEmail && (s.DeadLetter != 1 || s.Pending != 0) {
			t.Fatalf("email stats = %+v", s)
		}
	}

	if err := b.RetryDLQTask(ctx, "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("retry missing err = %v", err)
	}
	if err := b.DeleteDLQTask("missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("delete missing err = %v", err)
	}
	if err := b.RetryDLQTask(ctx, sched.ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := b.DLQTasks(); len(d) != 0 {
		t.Fatalf("task should have left the DLQ: %+v", d)
	}
	if l, _ := b.ListTasks(QueueEmail, StatusPending); len(l) != 1 || l[0].ID != sched.ID {
		t.Fatalf("retried task should be pending: %+v", l)
	}
}

func TestCov1xFailuresWhenRedisIsDown(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	task, _ := NewTask("t", map[string]string{})
	if _, err := b.Enqueue(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	mr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := b.Enqueue(ctx, task); err == nil || errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("enqueue err = %v", err)
	}
	if _, err := b.ListTasks("", ""); err == nil {
		t.Fatal("ListTasks without queue name should fail")
	}
	if _, err := b.ListTasks(QueueDefault, ""); err == nil {
		t.Fatal("ListTasks on a named queue should fail")
	}
	if _, err := b.QueueStats(); err == nil {
		t.Fatal("QueueStats should fail")
	}
	if _, err := b.DLQTasks(); err == nil {
		t.Fatal("DLQTasks should fail")
	}
	if err := b.RetryDLQTask(ctx, "x"); err == nil || errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("RetryDLQTask err = %v", err)
	}
	if err := b.DeleteDLQTask("x"); err == nil || errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("DeleteDLQTask err = %v", err)
	}
}

func TestCov1xQueueStatsFailsWhenAStateListingFails(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	task, _ := NewTask("t", map[string]string{})
	if _, err := b.Enqueue(context.Background(), task, WithQueue(QueueEmail)); err != nil {
		t.Fatal(err)
	}
	// Corrupt the archived set so only the archived-state listing fails.
	if err := mr.Set("asynq:{email}:archived", "not-a-zset"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.QueueStats(); err == nil {
		t.Fatal("QueueStats should surface a listing failure")
	}
	if _, err := b.DLQTasks(); err == nil {
		t.Fatal("DLQTasks should surface a listing failure")
	}
	if _, err := b.ListTasks(QueueEmail, StatusDeadLetter); err == nil {
		t.Fatal("ListTasks should surface a listing failure")
	}
	// A task lookup skips the broken queue entirely instead of failing.
	if err := b.DeleteDLQTask("nothing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xNormalizeAsynqTaskError(t *testing.T) {
	if normalizeAsynqTaskError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	for _, e := range []error{asynq.ErrTaskNotFound, asynq.ErrQueueNotFound, fmt.Errorf("wrap: %w", asynq.ErrTaskNotFound), errors.New("Thing Not Found here")} {
		if got := normalizeAsynqTaskError(e); !errors.Is(got, ErrTaskNotFound) {
			t.Errorf("%v normalised to %v", e, got)
		}
	}
	other := errors.New("redis exploded")
	if got := normalizeAsynqTaskError(other); got != other {
		t.Fatalf("other errors pass through, got %v", got)
	}
}

func TestCov1xConvertAsynqTaskInfo(t *testing.T) {
	if convertAsynqTaskInfo(nil) != nil {
		t.Fatal("nil in, nil out")
	}
	failed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("x", 3600))
	cases := map[asynq.TaskState]TaskStatus{
		asynq.TaskStateActive:      StatusActive,
		asynq.TaskStatePending:     StatusPending,
		asynq.TaskStateScheduled:   StatusPending,
		asynq.TaskStateRetry:       StatusRetrying,
		asynq.TaskStateArchived:    StatusDeadLetter,
		asynq.TaskStateCompleted:   StatusCompleted,
		asynq.TaskStateAggregating: StatusPending,
	}
	for state, want := range cases {
		got := convertAsynqTaskInfo(&asynq.TaskInfo{ID: "i", State: state, LastFailedAt: failed, CompletedAt: failed, NextProcessAt: failed, Payload: []byte(`{"org_id":"o"}`), MaxRetry: 3, Retried: 1, LastErr: "e"})
		if got.Status != want {
			t.Errorf("state %v -> %v want %v", state, got.Status, want)
		}
		if got.OrgID != "o" || got.MaxRetries != 3 || got.Retried != 1 || got.LastErr != "e" {
			t.Errorf("fields lost: %+v", got)
		}
		if got.LastFailedAt == nil || got.LastFailedAt.Location() != time.UTC || !got.ProcessAt.Equal(failed) || got.CompletedAt == nil {
			t.Errorf("times not normalised: %+v", got)
		}
	}
	bare := convertAsynqTaskInfo(&asynq.TaskInfo{ID: "b"})
	if bare.LastFailedAt != nil || bare.CompletedAt != nil || !bare.ProcessAt.IsZero() {
		t.Fatalf("zero times should stay unset: %+v", bare)
	}
}

func TestCov1xRetryDelayAndQueueError(t *testing.T) {
	qe := &queueError{queue: QueueBilling, err: errors.New("inner")}
	if qe.Error() != "inner" || !errors.Is(qe, qe.err) && qe.Unwrap() != qe.err {
		t.Fatalf("queueError = %v / %v", qe.Error(), qe.Unwrap())
	}
	billing := DefaultRetryPolicies()[QueueBilling].Backoff
	if d := retryDelay(1, fmt.Errorf("w: %w", qe), nil); d != billing[1] {
		t.Fatalf("billing delay = %v", d)
	}
	if d := retryDelay(99, qe, nil); d != billing[len(billing)-1] {
		t.Fatalf("overflow delay = %v", d)
	}
	def := DefaultRetryPolicies()[QueueDefault].Backoff
	if d := retryDelay(-5, errors.New("plain"), nil); d != def[0] {
		t.Fatalf("negative n = %v", d)
	}
	other := &queueError{queue: "weird", err: errors.New("x")}
	if d := retryDelay(2, other, nil); d != def[2] {
		t.Fatalf("unknown queue should use default policy: %v", d)
	}
}

// End-to-end: a started backend runs handlers, records metrics and moves
// tasks across active / retry / completed / archived states.
func TestCov1xStartRunsHandlersAndReportsEveryState(t *testing.T) {
	mr := miniredis.RunT(t)
	logs := &cov1xSafeBuf{}
	b := cov1xNewBackend(t, mr, slog.New(slog.NewTextHandler(logs, nil)))
	ctx := context.Background()

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	b.RegisterHandler("blocker", func(ctx context.Context, _ *Task) error {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	b.RegisterHandler("okay", func(context.Context, *Task) error { return nil })
	b.RegisterHandler("flaky", func(context.Context, *Task) error { return errors.New("transient") })
	b.RegisterHandler("fatal", func(context.Context, *Task) error { return errors.New("permanent") })
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil { // idempotent
		t.Fatalf("second Start: %v", err)
	}

	payload := map[string]string{"org_id": "org-e2e"}
	enq := func(typ string, opts ...Option) *TaskInfo {
		task, _ := NewTask(typ, payload)
		info, err := b.Enqueue(ctx, task, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	blocker := enq("blocker")
	<-started
	okInfo := enq("okay")
	flaky := enq("flaky", WithMaxRetries(3))
	fatal := enq("fatal", WithMaxRetries(0))

	status := func(id string, want TaskStatus) func() bool {
		return func() bool {
			l, err := b.ListTasks("", want)
			if err != nil {
				return false
			}
			for _, ti := range l {
				if ti.ID == id {
					return true
				}
			}
			return false
		}
	}
	cov1xWaitFor(t, "active", status(blocker.ID, StatusActive))
	cov1xWaitFor(t, "completed", status(okInfo.ID, StatusCompleted))
	cov1xWaitFor(t, "retrying", status(flaky.ID, StatusRetrying))
	cov1xWaitFor(t, "archived", status(fatal.ID, StatusDeadLetter))

	retrying, _ := b.ListTasks(QueueDefault, StatusRetrying)
	if len(retrying) != 1 || retrying[0].LastErr != "transient" || retrying[0].LastFailedAt == nil || retrying[0].Retried != 1 || retrying[0].OrgID != "org-e2e" {
		t.Fatalf("retry listing = %+v", retrying)
	}
	done, _ := b.ListTasks(QueueDefault, StatusCompleted)
	if len(done) != 1 || done[0].CompletedAt == nil {
		t.Fatalf("completed listing = %+v", done)
	}
	stats, _ := b.QueueStats()
	if len(stats) != 1 || stats[0].Active != 1 || stats[0].Completed != 1 || stats[0].Failed != 1 || stats[0].DeadLetter != 1 || stats[0].Pending != 1 {
		t.Fatalf("stats = %+v", stats)
	}

	releaseOnce.Do(func() { close(release) })
	cov1xWaitFor(t, "blocker done", status(blocker.ID, StatusCompleted))

	snap := b.Metrics().TypeSnapshot()
	if snap["okay"].Succeeded != 1 || snap["flaky"].Failed < 1 || snap["fatal"].Failed != 1 || snap["fatal"].DLQ != 1 || snap["flaky"].DLQ != 0 {
		t.Fatalf("metrics = %+v", snap)
	}
	if !strings.Contains(logs.String(), "job handler failed") {
		t.Fatalf("failure not logged: %s", logs.String())
	}
	hist := b.Metrics().RecentExecutions(0)
	var sawFatal bool
	for _, e := range hist {
		if e.Type == "fatal" && e.Status == StatusFailed && e.Err == "permanent" && e.Queue == QueueDefault && e.TaskID == fatal.ID {
			sawFatal = true
		}
	}
	if !sawFatal {
		t.Fatalf("history missing fatal execution: %+v", hist)
	}

	if err := b.DeleteDLQTask(fatal.ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := b.DLQTasks(); len(d) != 0 {
		t.Fatalf("dlq = %+v", d)
	}
	if err := b.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestCov1xStartFailuresAreCleanedUp(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	b.RegisterHandler("t", func(context.Context, *Task) error { return nil })

	b.RegisterScheduledJob(ScheduledJob{Name: "bad", TaskType: "t", Interval: time.Hour, Payload: make(chan int)})
	if err := b.Start(); err == nil {
		t.Fatal("unmarshalable payload must fail Start")
	}
	if b.started || b.server != nil {
		t.Fatal("failed Start must not leave a server running")
	}

	b.scheduled = []ScheduledJob{{Name: "fan", TaskType: "t", Interval: time.Hour, FanOut: func(context.Context) ([]FanOutItem, error) {
		return nil, errors.New("registry down")
	}}}
	err := b.Start()
	if err == nil || !strings.Contains(err.Error(), `enqueue initial scheduled job "fan"`) || !strings.Contains(err.Error(), "fan-out: registry down") {
		t.Fatalf("err = %v", err)
	}
	if b.started || b.server != nil {
		t.Fatal("failed Start must not leave a server running")
	}
}

func TestCov1xSchedulerTicksAndLogsFanOutFailures(t *testing.T) {
	mr := miniredis.RunT(t)
	logs := &cov1xSafeBuf{}
	b := cov1xNewBackend(t, mr, slog.New(slog.NewTextHandler(logs, nil)))
	b.RegisterHandler("plain", func(context.Context, *Task) error { return nil })
	b.RegisterHandler("fan", func(context.Context, *Task) error { return nil })

	var mu sync.Mutex
	calls := 0
	b.RegisterScheduledJob(ScheduledJob{Name: "off", TaskType: "plain", Interval: 0})
	b.RegisterScheduledJob(ScheduledJob{Name: "p", TaskType: "plain", Queue: QueueEmail, Interval: 50 * time.Millisecond, Payload: map[string]string{"k": "v"}})
	b.RegisterScheduledJob(ScheduledJob{Name: "f", TaskType: "fan", Queue: QueueBilling, Interval: 50 * time.Millisecond,
		FanOut: func(context.Context) ([]FanOutItem, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return []FanOutItem{{Key: "a", Payload: map[string]string{"org_id": "a"}}}, nil
			}
			return nil, errors.New("registry flaked")
		}})
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })

	cov1xWaitFor(t, "tick task", func() bool {
		l, err := b.inspector.ListPendingTasks(QueueEmail)
		if err != nil {
			return false
		}
		for _, ti := range l {
			if strings.Contains(ti.ID, ":tick:") {
				return true
			}
		}
		// the worker may already have run it
		c, _ := b.inspector.ListCompletedTasks(QueueEmail)
		for _, ti := range c {
			if strings.Contains(ti.ID, ":tick:") {
				return true
			}
		}
		return false
	})
	cov1xWaitFor(t, "flake log", func() bool { return strings.Contains(logs.String(), "scheduled job enqueue failed") })
	if !strings.Contains(logs.String(), "registry flaked") {
		t.Fatalf("logs: %s", logs.String())
	}
}

func TestCov1xEnqueueScheduledDedupesAndAggregatesErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	b := cov1xNewBackend(t, mr, cov1xQuietLogger())
	ctx := context.Background()
	task, _ := NewTask("t", map[string]string{})
	plain := ScheduledJob{Name: "p", TaskType: "t", Queue: QueueDefault}
	for i := 0; i < 2; i++ { // second firing in the same bucket is a silent duplicate
		if err := b.enqueueScheduled(ctx, plain, task, "tick", 7); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if l, _ := b.ListTasks(QueueDefault, ""); len(l) != 1 || l[0].ID != scheduledTaskID(plain, "tick", 7) {
		t.Fatalf("tasks = %+v", l)
	}

	fan := ScheduledJob{Name: "f", TaskType: "t", Queue: QueueReconcile, FanOut: func(context.Context) ([]FanOutItem, error) {
		return []FanOutItem{
			{Key: "bad", Payload: make(chan int)},
			{Key: "a", Payload: map[string]string{"org_id": "a"}},
			{Key: "b", Payload: map[string]string{"org_id": "b"}},
		}, nil
	}}
	err := b.enqueueScheduled(ctx, fan, nil, "tick", 1)
	if err == nil {
		t.Fatal("unmarshalable item must be reported")
	}
	l, _ := b.ListTasks(QueueReconcile, "")
	if len(l) != 2 {
		t.Fatalf("good items should still be enqueued: %+v", l)
	}
	// Re-running the same bucket dedupes the good items and reports only the bad one.
	if err2 := b.enqueueScheduled(ctx, fan, nil, "tick", 1); err2 == nil || err2.Error() != err.Error() {
		t.Fatalf("second err = %v", err2)
	}
	if l, _ := b.ListTasks(QueueReconcile, ""); len(l) != 2 {
		t.Fatalf("duplicates must collapse: %+v", l)
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := b.enqueueScheduled(cctx, plain, task, "tick", 99); err == nil {
		t.Fatal("non-duplicate enqueue failure must be returned")
	}
	ok := ScheduledJob{Name: "f2", TaskType: "t", FanOut: func(context.Context) ([]FanOutItem, error) {
		return []FanOutItem{{Key: "x", Payload: 1}}, nil
	}}
	if err := b.enqueueScheduled(cctx, ok, nil, "tick", 5); err == nil {
		t.Fatal("fan-out enqueue failure must be returned")
	}
}

func TestCov1xRedisMetricsRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	m := NewRedisJobMetrics(client, "")
	if m.prefix != "aquify:jobs:metrics" {
		t.Fatalf("default prefix = %q", m.prefix)
	}
	m = NewRedisJobMetrics(client, "cov1x:m")

	if got := m.TypeSnapshot(); len(got) != 0 {
		t.Fatalf("empty snapshot = %+v", got)
	}
	if got := m.RecentExecutions(5); len(got) != 0 {
		t.Fatalf("empty history = %+v", got)
	}

	m.RecordEnqueue("a")
	m.RecordEnqueue("a")
	m.RecordSuccess(JobExecution{Type: "a", TaskID: "1", Status: StatusCompleted})
	m.RecordFailure(JobExecution{Type: "a", TaskID: "2", Status: StatusFailed, Err: "bad"})
	m.RecordDLQ("a")
	m.RecordSuccess(JobExecution{Type: "b", TaskID: "3"})
	m.RecordEnqueue("") // ignored
	snap := m.TypeSnapshot()
	if len(snap) != 2 || snap["a"] != (TypeMetricSnapshot{Enqueued: 2, Succeeded: 1, Failed: 1, DLQ: 1}) || snap["b"].Succeeded != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}

	// Newest first, limited, clamped; malformed rows are skipped.
	if err := client.LPush(context.Background(), m.historyKey(), "{not json").Err(); err != nil {
		t.Fatal(err)
	}
	recent := m.RecentExecutions(0)
	if len(recent) != 3 || recent[0].TaskID != "3" || recent[2].TaskID != "1" || recent[1].Err != "bad" {
		t.Fatalf("recent = %+v", recent)
	}
	if got := m.RecentExecutions(2); len(got) != 1 { // two rows fetched, the newest is malformed
		t.Fatalf("limited = %+v", got)
	}
	if got := m.RecentExecutions(100000); len(got) != 3 {
		t.Fatalf("clamped = %+v", got)
	}

	// A counter hash that cannot be read is dropped but others survive.
	mr.Del(m.countersKey("b"))
	if err := mr.Set(m.countersKey("b"), "oops"); err != nil {
		t.Fatal(err)
	}
	logs := &cov1xSafeBuf{}
	m.logger = slog.New(slog.NewTextHandler(logs, nil))
	snap = m.TypeSnapshot()
	if _, ok := snap["b"]; ok || snap["a"].Enqueued != 2 {
		t.Fatalf("snapshot with corrupt counter = %+v", snap)
	}
	if !strings.Contains(logs.String(), "op=type_snapshot_counters") {
		t.Fatalf("logs: %s", logs.String())
	}
}

func TestCov1xRedisMetricsNilReceiversAndLogging(t *testing.T) {
	var nilM *RedisJobMetrics
	nilM.RecordEnqueue("x")
	nilM.RecordSuccess(JobExecution{Type: "x"})
	nilM.appendHistory(context.Background(), JobExecution{})
	if got := nilM.TypeSnapshot(); got == nil || len(got) != 0 {
		t.Fatalf("nil snapshot = %#v", got)
	}
	if nilM.RecentExecutions(3) != nil {
		t.Fatal("nil receiver history must be nil")
	}
	noClient := &RedisJobMetrics{prefix: "p"}
	noClient.RecordDLQ("x")
	noClient.appendHistory(context.Background(), JobExecution{})
	if len(noClient.TypeSnapshot()) != 0 || noClient.RecentExecutions(1) != nil {
		t.Fatal("client-less metrics must be inert")
	}

	// Outage with no logger set falls back to the default logger and reads fail soft.
	var buf cov1xSafeBuf
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Writer(&buf), nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = dead.Close() })
	m := &RedisJobMetrics{client: dead, prefix: "dead"}
	if got := m.RecentExecutions(3); got != nil {
		t.Fatalf("history during outage = %+v", got)
	}
	if !strings.Contains(buf.String(), "op=recent_executions") {
		t.Fatalf("outage not logged via default logger: %s", buf.String())
	}
}

func TestCov1xParseMetricInt(t *testing.T) {
	for in, want := range map[string]int64{"": 0, "12": 12, "-3": -3, "x": 0, "1.5": 0} {
		if got := parseMetricInt(in); got != want {
			t.Errorf("parseMetricInt(%q)=%d want %d", in, got, want)
		}
	}
	if normalizeTimestamp(time.Time{}) != nil {
		t.Fatal("zero time must normalise to nil")
	}
}
