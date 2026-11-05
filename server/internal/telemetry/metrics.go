package telemetry

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric instruments for the aquify platform.
// Register is called once at startup; all handlers share the same instance.
type Metrics struct {
	registry *prometheus.Registry

	// HTTP layer
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Background jobs
	JobExecutionsTotal *prometheus.CounterVec
	JobDurationSeconds *prometheus.HistogramVec

	// Import pipeline
	ImportFilesTotal *prometheus.CounterVec

	// Webhooks
	WebhookProcessingLag *prometheus.HistogramVec

	// Per-tenant usage. The org label is bounded (see BoundedLabel): beyond
	// MaxOrgLabels distinct orgs the rest share the value "other", so a growing
	// tenant count can never explode Prometheus cardinality.
	OrgHTTPRequestsTotal  *prometheus.CounterVec
	OrgJobExecutionsTotal *prometheus.CounterVec
	orgLabels             *BoundedLabel
}

// MaxOrgLabels is how many distinct orgs get their own metric series.
const MaxOrgLabels = 200

// BoundedLabel hands out label values for an unbounded key space: the first Max
// distinct keys keep their own value and every later key maps to "other".
type BoundedLabel struct {
	mu   sync.Mutex
	max  int
	seen map[string]struct{}
}

func NewBoundedLabel(max int) *BoundedLabel {
	return &BoundedLabel{max: max, seen: map[string]struct{}{}}
}

// Value returns key itself while there is room (or it was already admitted), else "other".
func (b *BoundedLabel) Value(key string) string {
	if key == "" {
		return "none"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.seen[key]; ok {
		return key
	}
	if len(b.seen) >= b.max {
		return "other"
	}
	b.seen[key] = struct{}{}
	return key
}

// OrgLabel returns the bounded metric label for an org id.
func (m *Metrics) OrgLabel(orgID string) string { return m.orgLabels.Value(orgID) }

// NewMetrics creates and registers all platform metrics with a fresh registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aquify",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests handled, labelled by method, route, and status code.",
		}, []string{"method", "route", "status"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aquify",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Histogram of HTTP request latencies in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),

		JobExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aquify",
			Subsystem: "job",
			Name:      "executions_total",
			Help:      "Total number of background job executions, labelled by type and status (success|failure|dlq).",
		}, []string{"type", "status"}),

		JobDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aquify",
			Subsystem: "job",
			Name:      "duration_seconds",
			Help:      "Histogram of background job execution durations in seconds.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 30, 120, 600},
		}, []string{"type"}),

		ImportFilesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aquify",
			Subsystem: "import",
			Name:      "files_total",
			Help:      "Total number of import files processed, labelled by status (success|failure|conflict).",
		}, []string{"status"}),

		OrgHTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aquify",
			Subsystem: "http",
			Name:      "org_requests_total",
			Help:      "Authenticated HTTP requests per org (bounded label) and status class (2xx|3xx|4xx|5xx).",
		}, []string{"org", "class"}),

		OrgJobExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aquify",
			Subsystem: "job",
			Name:      "org_executions_total",
			Help:      "Org-scoped background job executions per org (bounded label) and status (success|failure).",
		}, []string{"org", "status"}),

		orgLabels: NewBoundedLabel(MaxOrgLabels),

		WebhookProcessingLag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aquify",
			Subsystem: "webhook",
			Name:      "processing_lag_seconds",
			Help:      "Lag between Stripe webhook receipt and processing completion in seconds.",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		}, []string{"event_type"}),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.JobExecutionsTotal,
		m.JobDurationSeconds,
		m.ImportFilesTotal,
		m.WebhookProcessingLag,
		m.OrgHTTPRequestsTotal,
		m.OrgJobExecutionsTotal,
	)

	return m
}

// Handler returns an http.Handler that serves Prometheus metrics in text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
