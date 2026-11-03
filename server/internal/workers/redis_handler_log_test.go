package workers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func TestWrapHandlerLogsFailureAtError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	var logs bytes.Buffer
	b := &RedisBackend{
		logger:  slog.New(slog.NewTextHandler(&logs, nil)),
		metrics: NewRedisJobMetrics(client, "test:wrap"),
	}
	b.metrics.logger = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	wrapped := b.wrapHandler("compliance:data_export", func(context.Context, *Task) error {
		return errors.New("presign blew up")
	})
	if err := wrapped(context.Background(), asynq.NewTask("compliance:data_export", nil)); err == nil {
		t.Fatal("expected handler error to propagate")
	}
	out := logs.String()
	for _, want := range []string{"level=ERROR", "compliance:data_export", "presign blew up", "retry_count"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
}
