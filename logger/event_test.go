package logger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestEventBuilderEmit(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	log := Wrap(zap.New(core)).WithModule("gateway")
	ctx := WithRequestID(context.Background(), "req-1")

	log.WarnEvent(ctx, "gateway_upstream_call_failed", "dynamic upstream call failed").
		WithSummary("route %s failed with %s", "order.create", "Unavailable").
		WithString("route", "order.create").
		WithString("grpc_service", "order").
		WithString("grpc_method", "/api.order.v1.OrderService/CreateOrder").
		WithString("grpc_code", "Unavailable").
		WithDuration(1500 * time.Millisecond).
		WithError(errors.New("upstream unavailable")).
		Emit()

	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Level != zap.WarnLevel {
		t.Fatalf("level = %s", entry.Level)
	}
	if entry.Message != "dynamic upstream call failed" {
		t.Fatalf("message = %q", entry.Message)
	}
	fields := entry.ContextMap()
	assertLogField(t, fields, requestIDField, "req-1")
	assertLogField(t, fields, eventField, "gateway_upstream_call_failed")
	assertLogField(t, fields, moduleField, "gateway")
	assertLogField(t, fields, summaryField, "route order.create failed with Unavailable")
	assertLogField(t, fields, "route", "order.create")
	assertLogField(t, fields, "grpc_service", "order")
	assertLogField(t, fields, "grpc_method", "/api.order.v1.OrderService/CreateOrder")
	assertLogField(t, fields, "grpc_code", "Unavailable")
	assertLogField(t, fields, "duration_ms", float64(1500))
	assertLogField(t, fields, errorField, "upstream unavailable")
	if fields[errorTypeField] == "" {
		t.Fatalf("missing error type: %#v", fields)
	}
	for _, key := range []string{"caller_file", "caller_func", "caller_line"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("unexpected custom caller field %s in %#v", key, fields)
		}
	}
}

func TestEventBuilderFields(t *testing.T) {
	fields := Wrap(zap.NewNop()).
		InfoEvent(context.Background(), "hello_world", "hello").
		WithSummary("hello %s", "world").
		WithString("OrderID", "order-1").
		Fields()

	if len(fields) != 3 {
		t.Fatalf("field count = %d", len(fields))
	}
	got := map[string]zap.Field{}
	for _, field := range fields {
		got[field.Key] = field
	}
	if got[eventField].String != "hello_world" {
		t.Fatalf("event = %q", got[eventField].String)
	}
	if got[summaryField].String != "hello world" {
		t.Fatalf("summary = %q", got[summaryField].String)
	}
	if got["order_id"].String != "order-1" {
		t.Fatalf("order_id = %q", got["order_id"].String)
	}
}

func TestContextLoggerCallerUsesCallSite(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core, zap.AddCaller()))

	log.Info(context.Background(), "hello")

	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}
	caller := logs.All()[0].Caller
	if got := filepath.Base(caller.File); got != "event_test.go" {
		t.Fatalf("caller file = %q, want event_test.go", got)
	}
}

func TestEventBuilderCallerUsesCallSite(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core, zap.AddCaller()))

	log.InfoEvent(context.Background(), "test_event", "hello").Emit()

	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}
	caller := logs.All()[0].Caller
	if got := filepath.Base(caller.File); got != "event_test.go" {
		t.Fatalf("caller file = %q, want event_test.go", got)
	}
}

func TestNilEventBuilderEmitIsNoop(t *testing.T) {
	var builder *EventBuilder
	builder.Emit()
	if fields := builder.Fields(); fields != nil {
		t.Fatalf("fields = %#v", fields)
	}
}

func assertLogField(t *testing.T, fields map[string]any, key string, want any) {
	t.Helper()
	got, ok := fields[key]
	if !ok {
		t.Fatalf("missing field %s in %#v", key, fields)
	}
	switch want := want.(type) {
	case float64:
		gotFloat, ok := got.(float64)
		if !ok || gotFloat != want {
			t.Fatalf("%s = %#v, want %v", key, got, want)
		}
	default:
		if got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}
