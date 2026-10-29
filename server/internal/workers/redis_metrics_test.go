package workers

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRedisJobMetricsLogsWhenRedisIsUnavailable(t *testing.T) {
	// Nothing listens on this port, and retries are disabled so the test is fast.
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	var logs bytes.Buffer
	m := NewRedisJobMetrics(client, "test:metrics")
	m.logger = slog.New(slog.NewTextHandler(&logs, nil))

	m.RecordEnqueue("email:send")
	m.RecordFailure(JobExecution{Type: "email:send"})
	m.TypeSnapshot()

	out := logs.String()
	for _, op := range []string{"increment_counter", "append_history", "type_snapshot_types"} {
		if !strings.Contains(out, "op="+op) {
			t.Errorf("expected a warn log for %s, got:\n%s", op, out)
		}
	}
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("metric loss must be logged at warn level:\n%s", out)
	}
}
