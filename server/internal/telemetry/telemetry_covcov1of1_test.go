package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func cov1xRestoreOTel(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func TestCov1xLoadConfigDefaults(t *testing.T) {
	for _, k := range []string{"OTEL_TRACES_SAMPLER_ARG", "OTEL_SERVICE_NAME", "OTEL_SERVICE_VERSION", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		t.Setenv(k, "")
	}
	cfg := LoadConfig()
	if cfg.ServiceName != "aquify-api" || cfg.ServiceVersion != "dev" || cfg.OTLPEndpoint != "" || cfg.SampleRate != 1.0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestCov1xLoadConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "  svc-x ")
	t.Setenv("OTEL_SERVICE_VERSION", "1.2.3")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", " http://collector:4318 ")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	cfg := LoadConfig()
	if cfg.ServiceName != "svc-x" || cfg.ServiceVersion != "1.2.3" || cfg.OTLPEndpoint != "http://collector:4318" || cfg.SampleRate != 0.25 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestCov1xLoadConfigInvalidSampleRate(t *testing.T) {
	for _, v := range []string{"abc", "1.5", "-0.1"} {
		t.Setenv("OTEL_TRACES_SAMPLER_ARG", v)
		if got := LoadConfig().SampleRate; got != 1.0 {
			t.Fatalf("sample arg %q: want fallback 1.0, got %v", v, got)
		}
	}
}

func TestCov1xSetupNoEndpointNilLogger(t *testing.T) {
	cov1xRestoreOTel(t)
	shutdown, err := Setup(context.Background(), Config{}, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	fields := otel.GetTextMapPropagator().Fields()
	joined := strings.Join(fields, ",")
	if !strings.Contains(joined, "traceparent") || !strings.Contains(joined, "baggage") {
		t.Fatalf("propagator fields = %v", fields)
	}
}

func cov1xSetupWithCollector(t *testing.T, rate float64, endpointFn func(string) string) {
	t.Helper()
	cov1xRestoreOTel(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	shutdown, err := Setup(context.Background(), Config{
		ServiceName: "cov-svc", ServiceVersion: "9", OTLPEndpoint: endpointFn(srv.URL), SampleRate: rate,
	}, logger)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !strings.Contains(buf.String(), "OTel tracer initialised") {
		t.Fatalf("expected init log, got %q", buf.String())
	}
	_, span := otel.Tracer("cov").Start(context.Background(), "op")
	if rate >= 1 && !span.SpanContext().IsSampled() {
		t.Fatal("expected span to be sampled with rate 1")
	}
	if rate == 0 && span.SpanContext().IsSampled() {
		t.Fatal("expected span not to be sampled with rate 0")
	}
	span.End()
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatalf("global provider is %T, want SDK provider", otel.GetTracerProvider())
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if rate >= 1 && atomic.LoadInt32(&hits) == 0 {
		t.Fatal("expected flushed spans to reach the collector")
	}
	if rate == 0 && atomic.LoadInt32(&hits) != 0 {
		t.Fatal("unsampled spans must not be exported")
	}
}

func TestCov1xSetupFullURLAlwaysSample(t *testing.T) {
	cov1xSetupWithCollector(t, 1.0, func(u string) string { return u })
}

func TestCov1xSetupBareHostAlwaysSample(t *testing.T) {
	cov1xSetupWithCollector(t, 1.0, func(u string) string { return strings.TrimPrefix(u, "http://") })
}

func TestCov1xSetupZeroRatioNeverExports(t *testing.T) {
	cov1xSetupWithCollector(t, 0, func(u string) string { return u })
}

func TestCov1xSetupCancelledContext(t *testing.T) {
	cov1xRestoreOTel(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdown, err := Setup(ctx, Config{OTLPEndpoint: "http://collector.invalid:4318", SampleRate: 1}, nil)
	if err == nil {
		_ = shutdown(context.Background())
		t.Fatal("expected error from cancelled context")
	}
	if shutdown != nil || !strings.Contains(err.Error(), "telemetry: create OTLP exporter") {
		t.Fatalf("unexpected error shape: %v", err)
	}
}
