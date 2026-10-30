package app

import (
	"context"
	"time"

	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

// instrumentedRegistrar wraps every registered job handler so executions and
// durations reach Prometheus (dare_job_executions_total and
// dare_job_duration_seconds); without it those series, the job dashboards and
// the billing-failure alert never receive data.
type instrumentedRegistrar struct {
	inner   workerHandlerRegistrar
	metrics *telemetry.Metrics
}

func (r instrumentedRegistrar) RegisterHandler(taskType string, h workers.Handler) {
	r.inner.RegisterHandler(taskType, instrumentHandler(r.metrics, taskType, h))
}

// withJobMetrics returns the registrar unchanged when there is nothing to record into.
func withJobMetrics(registrar workerHandlerRegistrar, metrics *telemetry.Metrics) workerHandlerRegistrar {
	if registrar == nil || metrics == nil {
		return registrar
	}
	return instrumentedRegistrar{inner: registrar, metrics: metrics}
}

func instrumentHandler(metrics *telemetry.Metrics, taskType string, h workers.Handler) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		start := time.Now()
		// Recorded in a defer so a panicking handler still counts as a failure
		// (the panic keeps propagating to the worker's recover).
		status := "failure"
		defer func() {
			metrics.JobExecutionsTotal.WithLabelValues(taskType, status).Inc()
			metrics.JobDurationSeconds.WithLabelValues(taskType).Observe(time.Since(start).Seconds())
		}()
		err := h(ctx, task)
		if err == nil {
			status = "success"
		}
		return err
	}
}
