package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric instruments for the DARE platform.
// Register is called once at startup; all handlers share the same instance.
type Metrics struct {
	registry *prometheus.Registry

	// HTTP layer
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec

	// Background jobs
	JobExecutionsTotal *prometheus.CounterVec
	JobDurationSeconds *prometheus.HistogramVec

	// Import pipeline
	ImportFilesTotal *prometheus.CounterVec

	// Webhooks
	WebhookProcessingLag *prometheus.HistogramVec
}

// NewMetrics creates and registers all platform metrics with a fresh registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dare",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests handled, labelled by method, route, and status code.",
		}, []string{"method", "route", "status"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dare",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Histogram of HTTP request latencies in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),

		JobExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dare",
			Subsystem: "job",
			Name:      "executions_total",
			Help:      "Total number of background job executions, labelled by type and status (success|failure|dlq).",
		}, []string{"type", "status"}),

		JobDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dare",
			Subsystem: "job",
			Name:      "duration_seconds",
			Help:      "Histogram of background job execution durations in seconds.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 30, 120, 600},
		}, []string{"type"}),

		ImportFilesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dare",
			Subsystem: "import",
			Name:      "files_total",
			Help:      "Total number of import files processed, labelled by status (success|failure|conflict).",
		}, []string{"status"}),

		WebhookProcessingLag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dare",
			Subsystem: "webhook",
			Name:      "processing_lag_seconds",
			Help:      "Lag between Stripe webhook receipt and processing completion in seconds.",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		}, []string{"event_type"}),
	}

	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.JobExecutionsTotal,
		m.JobDurationSeconds,
		m.ImportFilesTotal,
		m.WebhookProcessingLag,
	)

	return m
}

// Handler returns an http.Handler that serves Prometheus metrics in text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
