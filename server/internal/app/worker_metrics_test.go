package app

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

type captureRegistrar map[string]workers.Handler

func (c captureRegistrar) RegisterHandler(taskType string, h workers.Handler) { c[taskType] = h }

func TestInstrumentedRegistrarRecordsJobExecutions(t *testing.T) {
	t.Parallel()
	m := telemetry.NewMetrics()
	reg := captureRegistrar{}
	r := withJobMetrics(reg, m)
	r.RegisterHandler("billing:reconcile", func(context.Context, *workers.Task) error { return nil })
	r.RegisterHandler("exports:generate", func(context.Context, *workers.Task) error { return errors.New("boom") })

	if err := reg["billing:reconcile"](context.Background(), &workers.Task{}); err != nil {
		t.Fatalf("success handler returned %v", err)
	}
	if err := reg["exports:generate"](context.Background(), &workers.Task{}); err == nil {
		t.Fatal("handler error must pass through the wrapper")
	}

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{
		`dare_job_executions_total{status="success",type="billing:reconcile"} 1`,
		`dare_job_executions_total{status="failure",type="exports:generate"} 1`,
		`dare_job_duration_seconds_count{type="billing:reconcile"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

func TestWithJobMetricsLeavesRegistrarAloneWithoutMetrics(t *testing.T) {
	t.Parallel()
	reg := captureRegistrar{}
	if got := withJobMetrics(reg, nil); got == nil {
		t.Fatal("registrar must be returned unchanged when metrics are nil")
	}
}

func TestInstrumentHandlerCountsPanicAsFailure(t *testing.T) {
	t.Parallel()
	m := telemetry.NewMetrics()
	h := instrumentHandler(m, "exports:panic", func(context.Context, *workers.Task) error { panic("boom") })
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic must keep propagating to the worker")
			}
		}()
		_ = h(context.Background(), &workers.Task{})
	}()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if want := `dare_job_executions_total{status="failure",type="exports:panic"} 1`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("metrics output missing %q", want)
	}
}
