package logger

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggerSkipLevels(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	log := Wrap(zap.New(core, zap.AddCaller()))
	ctx := context.Background()

	log.DebugSkip(ctx, 0, "debug")
	log.InfoSkip(ctx, 0, "info")
	log.WarnSkip(ctx, 0, "warn")
	log.ErrorSkip(ctx, 0, "error")

	entries := logs.All()
	if len(entries) != 4 {
		t.Fatalf("log count = %d, want 4", len(entries))
	}
	wantLevels := []zapcore.Level{
		zap.DebugLevel,
		zap.InfoLevel,
		zap.WarnLevel,
		zap.ErrorLevel,
	}
	for i, want := range wantLevels {
		if entries[i].Level != want {
			t.Errorf("entry %d level = %s, want %s", i, entries[i].Level, want)
		}
		if !strings.HasSuffix(entries[i].Caller.Function, ".TestLoggerSkipLevels") {
			t.Errorf("entry %d caller function = %q, want TestLoggerSkipLevels", i, entries[i].Caller.Function)
		}
	}
}

func TestLoggerInfoSkipAdditionalCallerFrames(t *testing.T) {
	tests := []struct {
		name       string
		skip       int
		wantCaller string
	}{
		{name: "negative", skip: -1, wantCaller: "loggerSkipLevelTwo"},
		{name: "zero", skip: 0, wantCaller: "loggerSkipLevelTwo"},
		{name: "one", skip: 1, wantCaller: "loggerSkipLevelOne"},
		{name: "two", skip: 2, wantCaller: "TestLoggerInfoSkipAdditionalCallerFrames"},
	}

	for _, tt := range tests {
		core, logs := observer.New(zap.InfoLevel)
		log := Wrap(zap.New(core, zap.AddCaller()))

		loggerSkipLevelOne(log, tt.skip)

		if logs.Len() != 1 {
			t.Fatalf("%s: log count = %d, want 1", tt.name, logs.Len())
		}
		caller := logs.All()[0].Caller.Function
		if !strings.HasSuffix(caller, "."+tt.wantCaller) {
			t.Fatalf("%s: caller function = %q, want %s", tt.name, caller, tt.wantCaller)
		}
	}
}

//go:noinline
func loggerSkipLevelOne(log *Logger, skip int) {
	loggerSkipLevelTwo(log, skip)
}

//go:noinline
func loggerSkipLevelTwo(log *Logger, skip int) {
	log.InfoSkip(context.Background(), skip, "hello")
}

func TestLoggerSkipPreservesContextAndLoggerFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core, zap.AddCaller()).With(zap.String("service", "payment"))).WithModule("handler")
	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithClientIP(ctx, "127.0.0.1")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	}))

	log.InfoSkip(ctx, 1, "hello", zap.String("operation", "GetUsers"))

	if logs.Len() != 1 {
		t.Fatalf("log count = %d, want 1", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	assertLogField(t, fields, "service", "payment")
	assertLogField(t, fields, moduleField, "handler")
	assertLogField(t, fields, "operation", "GetUsers")
	assertLogField(t, fields, requestIDField, "req-1")
	assertLogField(t, fields, clientIPField, "127.0.0.1")
	assertLogField(t, fields, traceIDField, "01000000000000000000000000000000")
	assertLogField(t, fields, spanIDField, "0200000000000000")
}

func TestLoggerSkipWithoutCallerStillLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := wrapWithCallerSkip(zap.New(core), false)

	log.InfoSkip(context.Background(), 2, "hello")

	if logs.Len() != 1 {
		t.Fatalf("log count = %d, want 1", logs.Len())
	}
	if logs.All()[0].Caller.Defined {
		t.Fatalf("caller = %#v, want undefined", logs.All()[0].Caller)
	}
}

func TestNilLoggerSkipMethodsAreNoop(t *testing.T) {
	var log *Logger
	ctx := context.Background()

	log.DebugSkip(ctx, 1, "debug")
	log.InfoSkip(ctx, 1, "info")
	log.WarnSkip(ctx, 1, "warn")
	log.ErrorSkip(ctx, 1, "error")
	log.WithCallerSkip(1).Info(ctx, "child")

	log = &Logger{}
	log.DebugSkip(ctx, 1, "debug")
	log.InfoSkip(ctx, 1, "info")
	log.WarnSkip(ctx, 1, "warn")
	log.ErrorSkip(ctx, 1, "error")
}

func TestLoggerWithCallerSkip(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := Wrap(zap.New(core, zap.AddCaller())).WithCallerSkip(1)

	loggerWithCallerSkipHelper(log)

	if logs.Len() != 1 {
		t.Fatalf("log count = %d, want 1", logs.Len())
	}
	caller := logs.All()[0].Caller.Function
	if !strings.HasSuffix(caller, ".TestLoggerWithCallerSkip") {
		t.Fatalf("caller function = %q, want TestLoggerWithCallerSkip", caller)
	}
}

//go:noinline
func loggerWithCallerSkipHelper(log *Logger) {
	log.Info(context.Background(), "hello")
}
