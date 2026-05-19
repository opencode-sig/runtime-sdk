package logger

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	apperror "github.com/opencode-sig/runtime-sdk/apperror"
)

const (
	eventField      = "event"
	moduleField     = "module"
	operationField  = "operation"
	summaryField    = "summary"
	errorField      = "error"
	errorTypeField  = "error_type"
	errorFileField  = "error_file"
	errorLineField  = "error_line"
	errorCodeField  = "error_code"
	statusCodeField = "status_code"
)

// Event records a stable machine-readable event name.
func Event(value string) zap.Field {
	return zap.String(eventField, normalizeFieldValue(value))
}

// Module records the logical module that produced the log entry.
func Module(value string) zap.Field {
	return zap.String(moduleField, normalizeFieldValue(value))
}

// Operation records the business or platform operation name.
func Operation(value string) zap.Field {
	return zap.String(operationField, normalizeFieldValue(value))
}

// Summary records an optional human-readable description for the event.
//
// The log message should remain stable for aggregation. Summary is the place
// for formatted context that helps people read a single log entry.
func Summary(format string, args ...any) zap.Field {
	if len(args) == 0 {
		return zap.String(summaryField, format)
	}
	return zap.String(summaryField, fmt.Sprintf(format, args...))
}

// String records a normalized string field.
func String(key string, value string) zap.Field {
	return zap.String(normalizeFieldKey(key), value)
}

// Int records a normalized integer field.
func Int(key string, value int) zap.Field {
	return zap.Int(normalizeFieldKey(key), value)
}

// Int64 records a normalized int64 field.
func Int64(key string, value int64) zap.Field {
	return zap.Int64(normalizeFieldKey(key), value)
}

// Bool records a normalized boolean field.
func Bool(key string, value bool) zap.Field {
	return zap.Bool(normalizeFieldKey(key), value)
}

// Any records a normalized arbitrary field.
func Any(key string, value any) zap.Field {
	return zap.Any(normalizeFieldKey(key), value)
}

// Fields groups normalized zap fields for call sites that need to append slices.
func Fields(fields ...zap.Field) []zap.Field {
	return fields
}

// Duration records duration as milliseconds for Elasticsearch aggregation.
//
// key="duration" is written as duration_ms; key="upstream_duration" is written
// as upstream_duration_ms. Passing a key that already ends with _ms keeps it.
func Duration(key string, value time.Duration) zap.Field {
	key = normalizeFieldKey(key)
	if !strings.HasSuffix(key, "_ms") {
		key += "_ms"
	}
	return zap.Float64(key, float64(value)/float64(time.Millisecond))
}

// StatusCode records an HTTP status code.
func StatusCode(value int) zap.Field {
	return zap.Int(statusCodeField, value)
}

// ErrorCode records a platform or business error code.
func ErrorCode(value int) zap.Field {
	return zap.Int(errorCodeField, value)
}

// Error records the human-readable error string.
func Error(err error) zap.Field {
	if err == nil {
		return zap.Skip()
	}
	return zap.String(errorField, err.Error())
}

// ErrorFields returns normalized flat fields for an error.
func ErrorFields(err error) []zap.Field {
	if err == nil {
		return nil
	}
	fields := []zap.Field{
		Error(err),
		zap.String(errorTypeField, errorType(err)),
	}
	if frame, ok := apperror.FrameOf(err); ok {
		fields = append(fields, zap.String(errorFileField, frame.File), zap.Int(errorLineField, frame.Line))
	}
	return fields
}

func normalizeFieldKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "field"
	}
	runes := []rune(key)
	var b strings.Builder
	var previousUnderscore bool
	for i, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if i > 0 && startsFieldWord(runes, i) && !previousUnderscore {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			previousUnderscore = false
		default:
			if b.Len() > 0 && !previousUnderscore {
				b.WriteByte('_')
				previousUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "field"
	}
	return out
}

func startsFieldWord(runes []rune, index int) bool {
	r := runes[index]
	if !unicode.IsUpper(r) {
		return false
	}
	prev := runes[index-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	if unicode.IsUpper(prev) && index+1 < len(runes) {
		return unicode.IsLower(runes[index+1])
	}
	return false
}

func normalizeFieldValue(value string) string {
	return normalizeFieldKey(value)
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		if cause := errors.Unwrap(appErr); cause != nil {
			return errorType(cause)
		}
	}
	t := reflect.TypeOf(err)
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return fmt.Sprintf("%T", err)
	}
	if t.PkgPath() == "" {
		return t.Name()
	}
	return t.PkgPath() + "." + t.Name()
}
