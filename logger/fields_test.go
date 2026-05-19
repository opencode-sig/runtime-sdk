package logger

import (
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	apperror "github.com/opencode-sig/runtime-sdk/apperror"
)

func TestNormalizeFieldKey(t *testing.T) {
	tests := map[string]string{
		"HTTP Status":       "http_status",
		"grpc-service":      "grpc_service",
		"duration_ms":       "duration_ms",
		" upstream latency": "upstream_latency",
		"":                  "field",
	}
	for input, want := range tests {
		if got := normalizeFieldKey(input); got != want {
			t.Fatalf("normalizeFieldKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDurationUsesMilliseconds(t *testing.T) {
	field := Duration("duration", 1500*time.Millisecond)
	if field.Key != "duration_ms" {
		t.Fatalf("key = %q", field.Key)
	}
	if field.Type != zapcore.Float64Type {
		t.Fatalf("type = %v", field.Type)
	}
	if got := math.Float64frombits(uint64(field.Integer)); got != 1500 {
		t.Fatalf("duration = %v", got)
	}
}

func TestSummarySupportsFormattedAndPlainValues(t *testing.T) {
	formatted := Summary("order %s created by %s", "order-1", "user-1")
	if formatted.Key != summaryField || formatted.String != "order order-1 created by user-1" {
		t.Fatalf("formatted summary = %#v", formatted)
	}

	plain := Summary("order created")
	if plain.Key != summaryField || plain.String != "order created" {
		t.Fatalf("plain summary = %#v", plain)
	}
}

func TestErrorFieldsIncludeAppErrorFrame(t *testing.T) {
	err := apperror.Wrap(os.ErrNotExist, "read file")
	fields := ErrorFields(err)
	got := map[string]zapcore.FieldType{}
	values := map[string]any{}
	for _, field := range fields {
		got[field.Key] = field.Type
		switch field.Type {
		case zapcore.StringType:
			values[field.Key] = field.String
		case zapcore.Int64Type:
			values[field.Key] = int(field.Integer)
		}
	}
	if got[errorField] != zapcore.StringType {
		t.Fatalf("missing error field: %#v", got)
	}
	if values[errorTypeField] == "" {
		t.Fatalf("missing error type: %#v", values)
	}
	if values[errorFileField] != "fields_test.go" {
		t.Fatalf("error file = %#v", values[errorFileField])
	}
	if values[errorLineField] == 0 {
		t.Fatalf("missing error line: %#v", values)
	}
}

func TestErrorFieldsAllowsPlainError(t *testing.T) {
	fields := ErrorFields(errors.New("plain"))
	if len(fields) != 2 {
		t.Fatalf("field count = %d", len(fields))
	}
}
