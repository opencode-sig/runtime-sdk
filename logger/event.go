package logger

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// EventBuilder builds and emits one structured log event.
type EventBuilder struct {
	logger *Logger
	ctx    context.Context
	level  zapcore.Level
	msg    string
	fields []zap.Field
}

// DebugEvent starts building a debug log event.
func (l *Logger) DebugEvent(ctx context.Context, event string, msg string) *EventBuilder {
	return l.newEvent(ctx, zapcore.DebugLevel, event, msg)
}

// InfoEvent starts building an info log event.
func (l *Logger) InfoEvent(ctx context.Context, event string, msg string) *EventBuilder {
	return l.newEvent(ctx, zapcore.InfoLevel, event, msg)
}

// WarnEvent starts building a warning log event.
func (l *Logger) WarnEvent(ctx context.Context, event string, msg string) *EventBuilder {
	return l.newEvent(ctx, zapcore.WarnLevel, event, msg)
}

// ErrorEvent starts building an error log event.
func (l *Logger) ErrorEvent(ctx context.Context, event string, msg string) *EventBuilder {
	return l.newEvent(ctx, zapcore.ErrorLevel, event, msg)
}

func (l *Logger) newEvent(ctx context.Context, level zapcore.Level, event string, msg string) *EventBuilder {
	return &EventBuilder{
		logger: l,
		ctx:    ctx,
		level:  level,
		msg:    msg,
		fields: []zap.Field{Event(event)},
	}
}

// WithOperation records the operation name.
func (b *EventBuilder) WithOperation(value string) *EventBuilder {
	return b.append(Operation(value))
}

// WithSummary records an optional formatted human-readable event summary.
func (b *EventBuilder) WithSummary(format string, args ...any) *EventBuilder {
	return b.append(Summary(format, args...))
}

// WithDuration records duration_ms.
func (b *EventBuilder) WithDuration(value time.Duration) *EventBuilder {
	return b.append(Duration("duration", value))
}

// WithStatusCode records HTTP status code.
func (b *EventBuilder) WithStatusCode(value int) *EventBuilder {
	return b.append(StatusCode(value))
}

// WithErrorCode records platform or business error code.
func (b *EventBuilder) WithErrorCode(value int) *EventBuilder {
	return b.append(ErrorCode(value))
}

// WithError records normalized error fields.
func (b *EventBuilder) WithError(err error) *EventBuilder {
	return b.append(ErrorFields(err)...)
}

// WithString records a normalized string field.
func (b *EventBuilder) WithString(key string, value string) *EventBuilder {
	return b.append(String(key, value))
}

// WithInt records a normalized int field.
func (b *EventBuilder) WithInt(key string, value int) *EventBuilder {
	return b.append(Int(key, value))
}

// WithBool records a normalized bool field.
func (b *EventBuilder) WithBool(key string, value bool) *EventBuilder {
	return b.append(Bool(key, value))
}

// WithAny records a normalized arbitrary field.
func (b *EventBuilder) WithAny(key string, value any) *EventBuilder {
	return b.append(Any(key, value))
}

// WithFields appends raw zap fields for integration points that already have zap fields.
func (b *EventBuilder) WithFields(fields ...zap.Field) *EventBuilder {
	return b.append(fields...)
}

// Fields returns the built fields without emitting a log entry.
func (b *EventBuilder) Fields() []zap.Field {
	if b == nil || len(b.fields) == 0 {
		return nil
	}
	return append([]zap.Field(nil), b.fields...)
}

// Emit writes the built event.
func (b *EventBuilder) Emit() {
	if b == nil {
		return
	}
	b.logger.log(b.ctx, b.level, b.msg, b.fields...)
}

func (b *EventBuilder) append(fields ...zap.Field) *EventBuilder {
	if b == nil || len(fields) == 0 {
		return b
	}
	b.fields = append(b.fields, fields...)
	return b
}
