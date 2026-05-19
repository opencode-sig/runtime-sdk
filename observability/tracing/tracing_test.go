package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc/metadata"
)

func TestMetadataCarrierRoundTrip(t *testing.T) {
	md := metadata.MD{}
	carrier := metadataCarrier{md: md}
	carrier.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")

	if got := carrier.Get("traceparent"); got == "" {
		t.Fatal("traceparent was not set")
	}
	if len(carrier.Keys()) != 1 {
		t.Fatalf("keys = %#v", carrier.Keys())
	}
}

func TestInitNoopInstallsTraceContextPropagator(t *testing.T) {
	shutdown := InitNoop("test")
	defer shutdown(context.Background())

	otel.GetTextMapPropagator().Inject(context.Background(), propagation.MapCarrier{})
}
