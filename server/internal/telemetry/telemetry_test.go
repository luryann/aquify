package telemetry

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestSetupAcceptsEndpointWithScheme(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); otel.SetTextMapPropagator(prevProp) })

	for _, endpoint := range []string{srv.URL, strings.TrimPrefix(srv.URL, "http://")} {
		hits.Store(0)
		shutdown, err := Setup(context.Background(), Config{ServiceName: "t", OTLPEndpoint: endpoint, SampleRate: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		_, span := otel.Tracer("t").Start(context.Background(), "op")
		span.End()
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("endpoint %q: flush failed: %v", endpoint, err)
		}
		if hits.Load() == 0 {
			t.Fatalf("endpoint %q: collector received no export", endpoint)
		}
	}
}

func TestBoundedLabelCapsDistinctOrgs(t *testing.T) {
	b := NewBoundedLabel(2)
	if b.Value("a") != "a" || b.Value("b") != "b" || b.Value("a") != "a" {
		t.Fatal("admitted keys keep their own label")
	}
	if b.Value("c") != "other" || b.Value("d") != "other" {
		t.Fatal("keys beyond the cap share one label")
	}
	if b.Value("b") != "b" {
		t.Fatal("an admitted key stays admitted after the cap is hit")
	}
	if b.Value("") != "none" {
		t.Fatal("empty key is labelled none")
	}
}
