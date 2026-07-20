package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a context-aware wrapper around zap.Logger.
//
// It keeps zap as the underlying logging engine while adding standard
// request_id, trace_id, span_id and metadata fields from context.
type Logger struct {
	base *zap.Logger
}

// NewContext creates a context-aware logger with the default production config.
func NewContext(service string) (*Logger, error) {
	base, err := New(service)
	if err != nil {
		return nil, err
	}
	return wrapWithCallerSkip(base, true), nil
}

// NewContextWithConfig creates a context-aware logger from Config.
func NewContextWithConfig(config Config) (*Logger, error) {
	base, err := NewWithConfig(config)
	if err != nil {
		return nil, err
	}
	return wrapWithCallerSkip(base, config.Caller), nil
}

// Wrap turns a zap logger into a context-aware Logger.
func Wrap(base *zap.Logger) *Logger {
	return wrapWithCallerSkip(base, true)
}

func wrapWithCallerSkip(base *zap.Logger, enabled bool) *Logger {
	if base == nil {
		base = zap.NewNop()
	}
	if enabled {
		base = base.WithOptions(zap.AddCallerSkip(2))
	}
	return &Logger{base: base}
}

// Zap returns the underlying zap logger for APIs that still accept *zap.Logger.
func (l *Logger) Zap() *zap.Logger {
	if l == nil || l.base == nil {
		return zap.NewNop()
	}
	return l.base
}

// With returns a child logger with fields attached to every entry.
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{base: l.Zap().With(fields...)}
}

// WithModule returns a child logger that attaches module to every log entry.
func (l *Logger) WithModule(module string) *Logger {
	return l.With(Module(module))
}

// Named returns a child logger with a name segment.
func (l *Logger) Named(name string) *Logger {
	return &Logger{base: l.Zap().Named(name)}
}

// WithCallerSkip returns a child logger that skips additional caller frames.
// Non-positive values leave caller behavior unchanged.
func (l *Logger) WithCallerSkip(skip int) *Logger {
	base := l.Zap()
	if skip > 0 {
		base = base.WithOptions(zap.AddCallerSkip(skip))
	}
	return &Logger{base: base}
}

// Sync flushes buffered log entries.
func (l *Logger) Sync() error {
	return l.Zap().Sync()
}

// Debug logs a debug message with context correlation fields.
func (l *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.DebugLevel, msg, fields...)
}

// DebugSkip logs a debug message while skipping additional caller frames.
func (l *Logger) DebugSkip(ctx context.Context, skip int, msg string, fields ...zap.Field) {
	l.logWithCallerSkip(ctx, zapcore.DebugLevel, skip, msg, fields...)
}

// Info logs an info message with context correlation fields.
func (l *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.InfoLevel, msg, fields...)
}

// InfoSkip logs an info message while skipping additional caller frames.
func (l *Logger) InfoSkip(ctx context.Context, skip int, msg string, fields ...zap.Field) {
	l.logWithCallerSkip(ctx, zapcore.InfoLevel, skip, msg, fields...)
}

// Warn logs a warning message with context correlation fields.
func (l *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.WarnLevel, msg, fields...)
}

// WarnSkip logs a warning message while skipping additional caller frames.
func (l *Logger) WarnSkip(ctx context.Context, skip int, msg string, fields ...zap.Field) {
	l.logWithCallerSkip(ctx, zapcore.WarnLevel, skip, msg, fields...)
}

// Error logs an error message with context correlation fields.
func (l *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.ErrorLevel, msg, fields...)
}

// ErrorSkip logs an error message while skipping additional caller frames.
func (l *Logger) ErrorSkip(ctx context.Context, skip int, msg string, fields ...zap.Field) {
	l.logWithCallerSkip(ctx, zapcore.ErrorLevel, skip, msg, fields...)
}

// DPanic logs a development panic message with context correlation fields.
func (l *Logger) DPanic(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.DPanicLevel, msg, fields...)
}

// Panic logs a panic message with context correlation fields, then panics.
func (l *Logger) Panic(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.PanicLevel, msg, fields...)
}

// Fatal logs a fatal message with context correlation fields, then exits.
func (l *Logger) Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	l.log(ctx, zapcore.FatalLevel, msg, fields...)
}

func (l *Logger) log(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	base := l.Zap()
	if checked := base.Check(level, msg); checked != nil {
		checked.Write(appendContextFields(ctx, fields)...)
	}
}

func (l *Logger) logWithCallerSkip(ctx context.Context, level zapcore.Level, skip int, msg string, fields ...zap.Field) {
	base := l.Zap()
	if skip > 0 {
		base = base.WithOptions(zap.AddCallerSkip(skip))
	}
	if checked := base.Check(level, msg); checked != nil {
		checked.Write(appendContextFields(ctx, fields)...)
	}
}
