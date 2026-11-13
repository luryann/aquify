package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app"
	"github.com/luryann/aquify/server/internal/workers"
)

// A failing telemetry flush must not turn a clean drain into a failure: the
// worker logs a warning and returns normally.
func TestCov1xRunWorkerWarnsWhenTelemetryFlushFails(t *testing.T) {
	cov1xBaseEnv(t)
	cov1xUseFakeRedis(t)
	cfg := app.LoadConfig()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	backend, err := workers.NewRedisBackend(workers.RedisBackendConfig{RedisURL: cfg.Worker.RedisURL, Logger: logger, Concurrency: 2})
	if err != nil {
		t.Fatalf("redis backend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	origExit := osExit
	osExit = func(c int) { t.Errorf("runWorker must not exit, got code %d", c) }
	t.Cleanup(func() { osExit = origExit })

	guard := make(chan os.Signal, 64)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)
	stop := make(chan struct{})
	var sender sync.WaitGroup
	sender.Add(1)
	go func() {
		defer sender.Done()
		tick := time.NewTicker(25 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			}
		}
	}()

	flushErr := errors.New("collector unreachable")
	var flushed bool
	runWorker(cfg, logger, backend, func(context.Context) error { flushed = true; return flushErr })
	close(stop)
	sender.Wait()

	if !flushed {
		t.Fatal("telemetry must be flushed during shutdown")
	}
	out := logs.String()
	if !strings.Contains(out, "telemetry shutdown error") || !strings.Contains(out, "collector unreachable") {
		t.Fatalf("expected the flush failure to be logged as a warning:\n%s", out)
	}
	if strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("a flush failure must not be logged as an error:\n%s", out)
	}
}
