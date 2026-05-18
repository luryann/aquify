// Package telemetry initialises OpenTelemetry tracing for the DARE platform.
// It is designed to be wired once in main() and then used throughout the
// application via the global OTel tracer.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is not set, a no-op tracer is registered
// so tests and local dev runs produce no external side-effects.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds observability configuration loaded from environment variables.
type Config struct {
	// ServiceName is the OTel service.name attribute (default: "dare-api").
	ServiceName string
	// ServiceVersion is the deployed artifact version (default: "dev").
	ServiceVersion string
	// OTLPEndpoint is the OTLP HTTP endpoint (e.g. "http://otel-collector:4318").
	// When empty, a no-op tracer is used.
	OTLPEndpoint string
	// SampleRate controls the fraction of traces that are sampled (0.0–1.0).
	// Default 1.0 (always sample).
	SampleRate float64
}

// LoadConfig reads observability settings from environment variables.
func LoadConfig() Config {
	sampleRate := 1.0
	if v := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG")); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil && f >= 0 && f <= 1 {
			sampleRate = f
		}
	}
	return Config{
		ServiceName:    envOrDefault("OTEL_SERVICE_NAME", "dare-api"),
		ServiceVersion: envOrDefault("OTEL_SERVICE_VERSION", "dev"),
		OTLPEndpoint:   strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		SampleRate:     sampleRate,
	}
}

// Setup initialises the global OTel tracer and propagator. It returns a
// shutdown function that must be called on process exit to flush spans.
// When cfg.OTLPEndpoint is empty a no-op TracerProvider is kept and the
// returned shutdown function is a safe no-op.
func Setup(ctx context.Context, cfg Config, logger *slog.Logger) (shutdown func(context.Context) error, err error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.OTLPEndpoint == "" {
		logger.Info("telemetry: OTLP endpoint not configured — using no-op tracer")
		// Default global provider is a no-op; keep it.
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)
		return func(context.Context) error { return nil }, nil
	}

	// Build the OTLP HTTP exporter.
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: create OTLP exporter: %w", err)
	}

	// Build the resource (service metadata).
	res, resErr := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
		resource.WithHost(),
		resource.WithOS(),
	)
	if resErr != nil {
		// Non-fatal: fall back to a minimal resource.
		res = resource.NewSchemaless(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		)
		logger.Warn("telemetry: resource detection partial", "err", resErr)
	}

	// Select sampler.
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Install as global.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	logger.Info("telemetry: OTel tracer initialised",
		"service", cfg.ServiceName,
		"version", cfg.ServiceVersion,
		"endpoint", cfg.OTLPEndpoint,
		"sample_rate", cfg.SampleRate,
	)

	return tp.Shutdown, nil
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
