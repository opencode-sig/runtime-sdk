package logger

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/metadata"
)

func TestContextFieldsExtractsRequestAndTrace(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0011223344556677")
	if err != nil {
		t.Fatalf("span id: %v", err)
	}

	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithClientIP(ctx, "127.0.0.1")
	ctx = WithUserID(ctx, "user-1")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))

	fields := fieldMap(ContextFields(ctx))
	if fields[requestIDField] != "req-1" {
		t.Fatalf("request id = %q", fields[requestIDField])
	}
	if fields[traceIDField] != traceID.String() {
		t.Fatalf("trace id = %q", fields[traceIDField])
	}
	if fields[spanIDField] != spanID.String() {
		t.Fatalf("span id = %q", fields[spanIDField])
	}
	if fields[clientIPField] != "127.0.0.1" {
		t.Fatalf("client ip = %q", fields[clientIPField])
	}
	if fields[userIDField] != "user-1" {
		t.Fatalf("user id = %q", fields[userIDField])
	}
}

func TestContextFieldsExtractsMetadataAndTraceparent(t *testing.T) {
	traceparent := "00-00112233445566778899aabbccddeeff-0011223344556677-01"
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		requestIDMetadataKey, "req-md",
		userIDMetadataKey, "user-md",
		traceparentMetadataKey, traceparent,
		xForwardedForMetadataKey, "10.0.0.1, 10.0.0.2",
	))

	fields := fieldMap(ContextFields(ctx))
	if fields[requestIDField] != "req-md" {
		t.Fatalf("request id = %q", fields[requestIDField])
	}
	if fields[traceIDField] != "00112233445566778899aabbccddeeff" {
		t.Fatalf("trace id = %q", fields[traceIDField])
	}
	if fields[spanIDField] != "0011223344556677" {
		t.Fatalf("span id = %q", fields[spanIDField])
	}
	if fields[traceparentField] != traceparent {
		t.Fatalf("traceparent = %q", fields[traceparentField])
	}
	if fields[clientIPField] != "10.0.0.1" {
		t.Fatalf("client ip = %q", fields[clientIPField])
	}
	if fields[userIDField] != "user-md" {
		t.Fatalf("user id = %q", fields[userIDField])
	}
}

func TestContextLoggerAddsFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core))
	ctx := WithRequestID(context.Background(), "req-1")

	log.Info(ctx, "hello", zap.String("component", "test"))

	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["component"] != "test" {
		t.Fatalf("component = %v", fields["component"])
	}
	if fields[requestIDField] != "req-1" {
		t.Fatalf("request id = %v", fields[requestIDField])
	}
	for _, key := range []string{"caller_file", "caller_func", "caller_line"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("unexpected custom caller field %s in %#v", key, fields)
		}
	}
}

func TestExplicitFieldOverridesContextField(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core))
	ctx := WithRequestID(context.Background(), "req-context")
	ctx = WithUserID(ctx, "user-context")

	log.Info(ctx, "hello", zap.String(requestIDField, "req-explicit"), zap.String(userIDField, "user-explicit"))

	fields := logs.All()[0].ContextMap()
	if fields[requestIDField] != "req-explicit" {
		t.Fatalf("request id = %v", fields[requestIDField])
	}
	if fields[userIDField] != "user-explicit" {
		t.Fatalf("user id = %v", fields[userIDField])
	}
}

func fieldMap(fields []zap.Field) map[string]string {
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		out[field.Key] = field.String
	}
	return out
}
