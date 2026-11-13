package workers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cov1xQuietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func cov1xWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func cov1xFastPolicy(max int, backoff ...time.Duration) map[string]RetryPolicy {
	return map[string]RetryPolicy{QueueDefault: {MaxRetries: max, Backoff: backoff}}
}

func TestCov1xNewTaskMarshalError(t *testing.T) {
	if _, err := NewTask("x", make(chan int)); err == nil {
		t.Fatal("expected marshal error for unsupported payload")
	}
	task, err := NewTask("x", map[string]string{"a": "b"})
	if err != nil || task.Type != "x" || string(task.Payload) != `{"a":"b"}` {
		t.Fatalf("unexpected task %+v err %v", task, err)
	}
}

func TestCov1xWithMaxRetriesOption(t *testing.T) {
	o := &EnqueueOptions{}
	WithMaxRetries(7)(o)
	if o.MaxRetries == nil || *o.MaxRetries != 7 {
		t.Fatalf("max retries not set: %+v", o.MaxRetries)
	}
}

func TestCov1xTaskInfoMarshalJSON(t *testing.T) {
	raw, err := json.Marshal(TaskInfo{ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "enqueued_at") || strings.Contains(string(raw), "process_at") {
		t.Fatalf("zero times must be omitted: %s", raw)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	raw, err = json.Marshal(TaskInfo{ID: "a", EnqueuedAt: now, ProcessAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"enqueued_at":"2026-01-02T03:04:05Z"`) || !strings.Contains(string(raw), `"process_at":"2026-01-02T03:04:05Z"`) {
		t.Fatalf("set times must be emitted: %s", raw)
	}
}

func TestCov1xTaskOrgID(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"not json":                 "",
		`[1,2]`:                    "",
		`{"x":1}`:                  "",
		`{"org_id":"  org-1 "}`:    "org-1",
		`{"org_id":"org-2","a":1}`: "org-2",
	}
	for in, want := range cases {
		if got := TaskOrgID([]byte(in)); got != want {
			t.Errorf("TaskOrgID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCov1xJobMetricsHistoryTrimmedAndLimited(t *testing.T) {
	m := NewJobMetrics()
	for i := 0; i < 505; i++ {
		m.RecordSuccess(JobExecution{TaskID: string(rune('a' + i%26)), Type: "t", Retried: i})
	}
	m.RecordFailure(JobExecution{Type: "t", Retried: 1000})
	m.RecordDLQ("t")
	m.RecordEnqueue("t")
	all := m.RecentExecutions(0)
	if len(all) != 500 {
		t.Fatalf("history len = %d, want 500", len(all))
	}
	if all[len(all)-1].Retried != 1000 {
		t.Fatalf("most recent entry should be last: %+v", all[len(all)-1])
	}
	if all[0].Retried != 6 { // 506 total recorded, oldest 6 dropped
		t.Fatalf("oldest retained = %d, want 6", all[0].Retried)
	}
	if got := m.RecentExecutions(2); len(got) != 2 || got[1].Retried != 1000 {
		t.Fatalf("limit not honoured: %+v", got)
	}
	if got := m.RecentExecutions(9999); len(got) != 500 {
		t.Fatalf("oversized limit should clamp: %d", len(got))
	}
	snap := m.TypeSnapshot()["t"]
	if snap.Succeeded != 505 || snap.Failed != 1 || snap.DLQ != 1 || snap.Enqueued != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestCov1xNewServerDefaultsAndPolicyOverride(t *testing.T) {
	s := NewServer(ServerConfig{Concurrency: -1, RetryPolicies: map[string]RetryPolicy{QueueEmail: {MaxRetries: 9}}})
	if s.concurrency != 2 {
		t.Fatalf("concurrency default = %d", s.concurrency)
	}
	if s.logger == nil {
		t.Fatal("logger default missing")
	}
	if got := s.policyFor(QueueEmail).MaxRetries; got != 9 {
		t.Fatalf("override ignored: %d", got)
	}
	if got := s.policyFor("unknown-queue").MaxRetries; got != DefaultRetryPolicies()[QueueDefault].MaxRetries {
		t.Fatalf("unknown queue should use default policy: %d", got)
	}
	if s.channelFor("unknown-queue") != s.queues[QueueDefault] {
		t.Fatal("unknown queue should map to default channel")
	}
	if s.Metrics() == nil {
		t.Fatal("metrics reader missing")
	}
}

func TestCov1xBackoffFor(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	if d := s.backoffFor(RetryPolicy{}, 3); d != 30*time.Second {
		t.Fatalf("empty backoff = %v", d)
	}
	p := RetryPolicy{Backoff: []time.Duration{time.Second, time.Minute}}
	if d := s.backoffFor(p, 0); d != time.Second {
		t.Fatalf("attempt 0 = %v", d)
	}
	if d := s.backoffFor(p, 5); d != time.Minute {
		t.Fatalf("overflow attempt should reuse last: %v", d)
	}
}

func TestCov1xProcessesTaskSuccessfully(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	var got atomic.Value
	s.RegisterHandler("ok", func(_ context.Context, task *Task) error {
		got.Store(string(task.Payload))
		return nil
	})
	s.Start()
	defer s.Stop()
	task, _ := NewTask("ok", map[string]string{"org_id": "org-9"})
	info, err := s.Enqueue(context.Background(), task, WithUniqueKey("u1"))
	if err != nil {
		t.Fatal(err)
	}
	if info.OrgID != "org-9" || info.Status != StatusPending || info.UniqueKey != "u1" {
		t.Fatalf("unexpected info %+v", info)
	}
	cov1xWaitFor(t, "completion", func() bool {
		ti, err := s.GetTask(info.ID)
		return err == nil && ti.Status == StatusCompleted && ti.CompletedAt != nil
	})
	if got.Load() != `{"org_id":"org-9"}` {
		t.Fatalf("handler payload = %v", got.Load())
	}
	// The unique key is released on completion, so the same key can be enqueued again.
	if _, err := s.Enqueue(context.Background(), task, WithUniqueKey("u1")); err != nil {
		t.Fatalf("unique key not released after completion: %v", err)
	}
	snap := s.Metrics().TypeSnapshot()["ok"]
	if snap.Enqueued < 1 {
		t.Fatalf("enqueue not recorded: %+v", snap)
	}
	stats, err := s.QueueStats()
	if err != nil || len(stats) != 5 {
		t.Fatalf("stats %v err %v", stats, err)
	}
}

func TestCov1xQueueStatsCountsByStatus(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	s.RegisterHandler("t", func(context.Context, *Task) error { return nil })
	// Not started: the tasks sit pending.
	for i := 0; i < 2; i++ {
		if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}, WithQueue(QueueEmail)); err != nil {
			t.Fatal(err)
		}
	}
	s.allTasks["a"] = &internalTask{TaskInfo: TaskInfo{ID: "a", Queue: QueueEmail, Status: StatusActive}}
	s.allTasks["c"] = &internalTask{TaskInfo: TaskInfo{ID: "c", Queue: QueueEmail, Status: StatusCompleted}}
	s.allTasks["f"] = &internalTask{TaskInfo: TaskInfo{ID: "f", Queue: QueueEmail, Status: StatusFailed}}
	s.allTasks["r"] = &internalTask{TaskInfo: TaskInfo{ID: "r", Queue: QueueEmail, Status: StatusRetrying}}
	s.allTasks["d"] = &internalTask{TaskInfo: TaskInfo{ID: "d", Queue: QueueEmail, Status: StatusDeadLetter}}
	stats, _ := s.QueueStats()
	var email QueueStats
	for _, q := range stats {
		if q.Name == QueueEmail {
			email = q
		}
	}
	want := QueueStats{Name: QueueEmail, Pending: 3, Active: 1, Completed: 1, Failed: 1, DeadLetter: 1}
	if email != want {
		t.Fatalf("email stats = %+v, want %+v", email, want)
	}
	if got, _ := s.ListTasks(QueueEmail, StatusActive); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("status filter: %+v", got)
	}
	if got, _ := s.ListTasks(QueueBilling, ""); len(got) != 0 {
		t.Fatalf("queue filter: %+v", got)
	}
}

func TestCov1xGetTaskNotFound(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	if _, err := s.GetTask("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xEnqueueRejections(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	s.RegisterHandler("t", func(context.Context, *Task) error { return nil })
	if _, err := s.Enqueue(context.Background(), &Task{Type: "missing"}); err == nil || !strings.Contains(err.Error(), `no handler registered for task type "missing"`) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}, WithUniqueKey("k")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}, WithUniqueKey("k")); !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("duplicate err = %v", err)
	}
	s.Stop()
	if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}); !errors.Is(err, ErrWorkerClosed) {
		t.Fatalf("closed err = %v", err)
	}
}

func TestCov1xEnqueueCanceledContextReleasesUniqueKey(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	s.RegisterHandler("t", func(context.Context, *Task) error { return nil })
	ch := s.queues[QueueDefault]
	for len(ch) < cap(ch) {
		ch <- &internalTask{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Enqueue(ctx, &Task{Type: "t"}, WithUniqueKey("k"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if _, held := s.uniqueKeyIndex["k"]; held {
		t.Fatal("unique key must be released when dispatch fails")
	}
	if got, _ := s.ListTasks("", ""); len(got) != 0 {
		t.Fatalf("task must be removed when dispatch fails: %+v", got)
	}
}

func TestCov1xFailureRetriesThenDeadLettersAndCanBeRetriedAndDeleted(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger(), RetryPolicies: cov1xFastPolicy(2, time.Millisecond)})
	var fail atomic.Bool
	fail.Store(true)
	var calls atomic.Int32
	s.RegisterHandler("flaky", func(context.Context, *Task) error {
		calls.Add(1)
		if fail.Load() {
			return errors.New("boom")
		}
		return nil
	})
	s.Start()
	defer s.Stop()
	info, err := s.Enqueue(context.Background(), &Task{Type: "flaky"}, WithUniqueKey("fk"))
	if err != nil {
		t.Fatal(err)
	}
	cov1xWaitFor(t, "dead letter", func() bool { d, _ := s.DLQTasks(); return len(d) == 1 })
	if calls.Load() != 3 {
		t.Fatalf("attempts = %d, want 3 (1 + 2 retries)", calls.Load())
	}
	dlq, _ := s.DLQTasks()
	if dlq[0].ID != info.ID || dlq[0].LastErr != "boom" || dlq[0].Status != StatusDeadLetter || dlq[0].Retried != 3 {
		t.Fatalf("dlq entry %+v", dlq[0])
	}
	snap := s.Metrics().TypeSnapshot()["flaky"]
	if snap.Failed != 3 || snap.DLQ != 1 {
		t.Fatalf("metrics %+v", snap)
	}
	history := s.Metrics().RecentExecutions(10)
	if len(history) != 3 || history[0].Err != "boom" || history[0].Status != StatusFailed {
		t.Fatalf("history %+v", history)
	}
	// Unique key is freed once dead-lettered.
	if _, held := func() (string, bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		v, ok := s.uniqueKeyIndex["fk"]
		return v, ok
	}(); held {
		t.Fatal("unique key should be released in DLQ")
	}

	fail.Store(false)
	if err := s.RetryDLQTask(context.Background(), "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("retry missing err = %v", err)
	}
	if err := s.RetryDLQTask(context.Background(), info.ID); err != nil {
		t.Fatal(err)
	}
	cov1xWaitFor(t, "completion after retry", func() bool {
		ti, err := s.GetTask(info.ID)
		return err == nil && ti.Status == StatusCompleted
	})
	if d, _ := s.DLQTasks(); len(d) != 0 {
		t.Fatalf("DLQ should be empty: %+v", d)
	}
}

func TestCov1xDeleteDLQTask(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	keep := &internalTask{TaskInfo: TaskInfo{ID: "keep", Status: StatusDeadLetter}}
	gone := &internalTask{TaskInfo: TaskInfo{ID: "gone", Status: StatusDeadLetter, UniqueKey: "gk"}}
	s.allTasks["keep"], s.allTasks["gone"] = keep, gone
	s.uniqueKeyIndex["gk"] = "gone"
	s.dlq = []*internalTask{keep, gone}
	if err := s.DeleteDLQTask("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := s.DeleteDLQTask("gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTask("gone"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatal("deleted task still known")
	}
	if _, held := s.uniqueKeyIndex["gk"]; held {
		t.Fatal("unique key not released")
	}
	if d, _ := s.DLQTasks(); len(d) != 1 || d[0].ID != "keep" {
		t.Fatalf("dlq = %+v", d)
	}
}

func TestCov1xRetryDLQReRegistersUniqueKey(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	defer s.Stop()
	s.RegisterHandler("t", func(context.Context, *Task) error { return nil })
	it := &internalTask{TaskInfo: TaskInfo{ID: "x", Queue: QueueDefault, Type: "t", Status: StatusDeadLetter, UniqueKey: "uk", LastErr: "e", Retried: 4}}
	s.allTasks["x"] = it
	s.dlq = []*internalTask{it}
	if err := s.RetryDLQTask(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if s.uniqueKeyIndex["uk"] != "x" {
		t.Fatal("unique key should be re-registered on retry")
	}
	if it.Status != StatusPending || it.Retried != 0 || it.LastErr != "" {
		t.Fatalf("task not reset: %+v", it.TaskInfo)
	}
	// Canceled dispatch after a unique-key retry releases the key again.
	it2 := &internalTask{TaskInfo: TaskInfo{ID: "y", Queue: QueueBilling, Status: StatusDeadLetter, UniqueKey: "uk2"}}
	s.dlq = []*internalTask{it2}
	ch := s.queues[QueueBilling]
	for len(ch) < cap(ch) {
		ch <- &internalTask{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.RetryDLQTask(ctx, "y"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if _, held := s.uniqueKeyIndex["uk2"]; held {
		t.Fatal("unique key should be released after failed dispatch")
	}
}

func TestCov1xDelayedEnqueueRunsAndDeadLettersOnShutdown(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	var ran atomic.Int32
	s.RegisterHandler("t", func(context.Context, *Task) error { ran.Add(1); return nil })
	s.Start()
	if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}, WithDelay(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	cov1xWaitFor(t, "delayed run", func() bool { return ran.Load() == 1 })
	info, err := s.EnqueueIn(context.Background(), time.Hour, &Task{Type: "t"}, WithUniqueKey("late"))
	if err != nil {
		t.Fatal(err)
	}
	s.Stop()
	cov1xWaitFor(t, "shutdown dead-letter", func() bool { d, _ := s.DLQTasks(); return len(d) == 1 })
	d, _ := s.DLQTasks()
	if d[0].ID != info.ID || d[0].LastErr != "server_shutdown" || d[0].Status != StatusDeadLetter {
		t.Fatalf("dlq entry %+v", d[0])
	}
}

func TestCov1xDispatchOrDeadLetterOnShutdownWithFullQueue(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	ch := s.queues[QueueDefault]
	for len(ch) < cap(ch) {
		ch <- &internalTask{}
	}
	s.cancel()
	it := &internalTask{TaskInfo: TaskInfo{ID: "z", Queue: QueueDefault}}
	s.dispatchOrDeadLetter(ch, it)
	if d, _ := s.DLQTasks(); len(d) != 1 || d[0].ID != "z" || d[0].LastErr != "server_shutdown" {
		t.Fatalf("dlq = %+v", d)
	}
}

func TestCov1xRetryWaitInterruptedByShutdown(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger(), RetryPolicies: cov1xFastPolicy(5, time.Hour)})
	s.RegisterHandler("bad", func(context.Context, *Task) error { return errors.New("nope") })
	s.Start()
	info, err := s.Enqueue(context.Background(), &Task{Type: "bad"})
	if err != nil {
		t.Fatal(err)
	}
	cov1xWaitFor(t, "retrying", func() bool {
		ti, err := s.GetTask(info.ID)
		return err == nil && ti.Status == StatusRetrying
	})
	s.Stop()
	cov1xWaitFor(t, "parked", func() bool { d, _ := s.DLQTasks(); return len(d) == 1 })
	if d, _ := s.DLQTasks(); d[0].LastErr != "server_shutdown" {
		t.Fatalf("last err = %q", d[0].LastErr)
	}
}

func TestCov1xCompletedHistoryIsBounded(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger(), Concurrency: 4})
	var done atomic.Int32
	s.RegisterHandler("t", func(context.Context, *Task) error { done.Add(1); return nil })
	s.Start()
	defer s.Stop()
	total := maxCompletedHistory + 20
	for i := 0; i < total; i++ {
		if _, err := s.Enqueue(context.Background(), &Task{Type: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	cov1xWaitFor(t, "all done", func() bool { return int(done.Load()) == total })
	cov1xWaitFor(t, "history bounded", func() bool {
		all, _ := s.ListTasks("", StatusCompleted)
		return len(all) == maxCompletedHistory
	})
}

func TestCov1xGenerateIDIsUniqueHex(t *testing.T) {
	a, err := generateID()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := generateID()
	if len(a) != 16 || a == b {
		t.Fatalf("ids %q %q", a, b)
	}
}

func TestCov1xSchedulerFiresPlainAndFanOutJobs(t *testing.T) {
	s := NewServer(ServerConfig{Logger: cov1xQuietLogger()})
	var plain, fan atomic.Int32
	s.RegisterHandler("plain", func(context.Context, *Task) error { plain.Add(1); return nil })
	s.RegisterHandler("fan", func(_ context.Context, task *Task) error {
		if TaskOrgID(task.Payload) == "" {
			t.Errorf("fan-out payload lost org: %s", task.Payload)
		}
		fan.Add(1)
		return nil
	})
	s.Start()
	defer s.Stop()

	sch := NewScheduler(s, nil)
	if sch.logger == nil {
		t.Fatal("logger default missing")
	}
	sch.logger = cov1xQuietLogger()
	sch.Register(ScheduledJob{Name: "p", TaskType: "plain", Queue: QueueDefault, Interval: 5 * time.Millisecond, Payload: map[string]string{"k": "v"}})
	sch.Register(ScheduledJob{Name: "off", TaskType: "plain", Interval: 0})
	sch.Register(ScheduledJob{Name: "f", TaskType: "fan", Queue: QueueDefault, Interval: 5 * time.Millisecond,
		FanOut: func(context.Context) ([]FanOutItem, error) {
			return []FanOutItem{{Key: "a", Payload: map[string]string{"org_id": "a"}}, {Key: "b", Payload: map[string]string{"org_id": "b"}}}, nil
		}})
	sch.Start()
	cov1xWaitFor(t, "scheduled runs", func() bool { return plain.Load() >= 1 && fan.Load() >= 2 })
	sch.Stop()
}

func TestCov1xSchedulerStopWithoutStart(t *testing.T) {
	sch := NewScheduler(NewServer(ServerConfig{Logger: cov1xQuietLogger()}), cov1xQuietLogger())
	sch.Stop() // must not panic without a prior Start
}

type cov1xFailQueue struct{ calls atomic.Int32 }

func (q *cov1xFailQueue) Enqueue(context.Context, *Task, ...Option) (*TaskInfo, error) {
	q.calls.Add(1)
	return nil, errors.New("enqueue down")
}

func (q *cov1xFailQueue) EnqueueIn(context.Context, time.Duration, *Task, ...Option) (*TaskInfo, error) {
	return nil, errors.New("enqueue down")
}

func TestCov1xSchedulerFireErrorPaths(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	q := &cov1xFailQueue{}
	sch := NewScheduler(q, logger)

	// plain: payload cannot be marshalled
	sch.fire(context.Background(), ScheduledJob{Name: "badpayload", TaskType: "t", Payload: make(chan int)})
	if q.calls.Load() != 0 || !strings.Contains(logs.String(), "scheduler: build task") {
		t.Fatalf("expected build-task error, calls=%d logs=%s", q.calls.Load(), logs.String())
	}
	// plain: enqueue fails
	sch.fire(context.Background(), ScheduledJob{Name: "enq", TaskType: "t", Payload: 1})
	if q.calls.Load() != 1 || !strings.Contains(logs.String(), "scheduler: enqueue") {
		t.Fatalf("expected enqueue error log, calls=%d", q.calls.Load())
	}
	// fan-out: fan-out call fails
	logs.Reset()
	sch.fire(context.Background(), ScheduledJob{Name: "fo", TaskType: "t", FanOut: func(context.Context) ([]FanOutItem, error) { return nil, errors.New("registry down") }})
	if !strings.Contains(logs.String(), "scheduler: fan-out") || !strings.Contains(logs.String(), "registry down") {
		t.Fatalf("logs: %s", logs.String())
	}
	// fan-out: one bad payload, one enqueue failure
	logs.Reset()
	before := q.calls.Load()
	sch.fire(context.Background(), ScheduledJob{Name: "fo2", TaskType: "t", FanOut: func(context.Context) ([]FanOutItem, error) {
		return []FanOutItem{{Key: "bad", Payload: make(chan int)}, {Key: "good", Payload: 1}}, nil
	}})
	if q.calls.Load() != before+1 {
		t.Fatalf("only the buildable item should be enqueued")
	}
	if !strings.Contains(logs.String(), "scheduler: build task") || !strings.Contains(logs.String(), "key=bad") ||
		!strings.Contains(logs.String(), "scheduler: enqueue") || !strings.Contains(logs.String(), "key=good") {
		t.Fatalf("logs: %s", logs.String())
	}
}
