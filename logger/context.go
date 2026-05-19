package logger

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

const (
	requestIDField   = "request_id"
	traceIDField     = "trace_id"
	spanIDField      = "span_id"
	traceparentField = "traceparent"
	tracestateField  = "tracestate"
	xTraceIDField    = "x_trace_id"
	clientIPField    = "client_ip"
	userIDField      = "user_id"

	requestIDMetadataKey     = "x-request-id"
	userIDMetadataKey        = "x-user-id"
	traceparentMetadataKey   = "traceparent"
	tracestateMetadataKey    = "tracestate"
	xTraceIDMetadataKey      = "x-trace-id"
	xForwardedForMetadataKey = "x-forwarded-for"
	xRealIPMetadataKey       = "x-real-ip"
)

type contextKey string

const (
	requestIDContextKey contextKey = "request_id"
	clientIPContextKey  contextKey = "client_ip"
	userIDContextKey    contextKey = "user_id"
)

// WithRequestID returns a child context carrying request id for contextual logs.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

// RequestIDFromContext extracts request id from context values or gRPC metadata.
func RequestIDFromContext(ctx context.Context) string {
	return firstNonEmpty(contextString(ctx, requestIDContextKey), metadataValue(ctx, requestIDMetadataKey))
}

// WithUserID returns a child context carrying user id for contextual logs.
func WithUserID(ctx context.Context, userID string) context.Context {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext extracts user id from context values or gRPC metadata.
func UserIDFromContext(ctx context.Context) string {
	return firstNonEmpty(contextString(ctx, userIDContextKey), metadataValue(ctx, userIDMetadataKey))
}

// WithClientIP returns a child context carrying client IP for contextual logs.
func WithClientIP(ctx context.Context, clientIP string) context.Context {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return ctx
	}
	return context.WithValue(ctx, clientIPContextKey, clientIP)
}

// ContextFields extracts standard trace correlation fields from context.
//
// Supported sources:
// - values set by WithRequestID, WithUserID and WithClientIP
// - incoming or outgoing gRPC metadata
// - OpenTelemetry SpanContext
// - W3C traceparent header when no OpenTelemetry span is available
func ContextFields(ctx context.Context) []zap.Field {
	if ctx == nil {
		return nil
	}
	values := contextLogValues{
		RequestID:   RequestIDFromContext(ctx),
		Traceparent: metadataValue(ctx, traceparentMetadataKey),
		Tracestate:  metadataValue(ctx, tracestateMetadataKey),
		XTraceID:    metadataValue(ctx, xTraceIDMetadataKey),
		ClientIP:    firstNonEmpty(contextString(ctx, clientIPContextKey), metadataValue(ctx, xRealIPMetadataKey), firstForwardedFor(metadataValue(ctx, xForwardedForMetadataKey))),
		UserID:      UserIDFromContext(ctx),
	}

	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.HasTraceID() {
		values.TraceID = spanContext.TraceID().String()
	}
	if spanContext.HasSpanID() {
		values.SpanID = spanContext.SpanID().String()
	}
	if values.TraceID == "" || values.SpanID == "" {
		traceID, spanID := parseTraceparent(values.Traceparent)
		values.TraceID = firstNonEmpty(values.TraceID, traceID)
		values.SpanID = firstNonEmpty(values.SpanID, spanID)
	}

	return values.fields()
}

// appendContextFields appends context fields unless the caller already supplied the same key.
func appendContextFields(ctx context.Context, fields []zap.Field) []zap.Field {
	contextFields := ContextFields(ctx)
	if len(contextFields) == 0 {
		return fields
	}
	existing := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		existing[field.Key] = struct{}{}
	}
	out := append([]zap.Field(nil), fields...)
	for _, field := range contextFields {
		if _, ok := existing[field.Key]; ok {
			continue
		}
		out = append(out, field)
	}
	return out
}

type contextLogValues struct {
	RequestID   string
	TraceID     string
	SpanID      string
	Traceparent string
	Tracestate  string
	XTraceID    string
	ClientIP    string
	UserID      string
}

func (v contextLogValues) fields() []zap.Field {
	fields := make([]zap.Field, 0, 8)
	if v.RequestID != "" {
		fields = append(fields, zap.String(requestIDField, v.RequestID))
	}
	if v.UserID != "" {
		fields = append(fields, zap.String(userIDField, v.UserID))
	}
	if v.TraceID != "" {
		fields = append(fields, zap.String(traceIDField, v.TraceID))
	}
	if v.SpanID != "" {
		fields = append(fields, zap.String(spanIDField, v.SpanID))
	}
	if v.Traceparent != "" {
		fields = append(fields, zap.String(traceparentField, v.Traceparent))
	}
	if v.Tracestate != "" {
		fields = append(fields, zap.String(tracestateField, v.Tracestate))
	}
	if v.XTraceID != "" {
		fields = append(fields, zap.String(xTraceIDField, v.XTraceID))
	}
	if v.ClientIP != "" {
		fields = append(fields, zap.String(clientIPField, v.ClientIP))
	}
	return fields
}

func contextString(ctx context.Context, key contextKey) string {
	value, _ := ctx.Value(key).(string)
	return strings.TrimSpace(value)
}

func metadataValue(ctx context.Context, key string) string {
	if value := metadataValueFrom(ctx, key, metadata.FromIncomingContext); value != "" {
		return value
	}
	return metadataValueFrom(ctx, key, metadata.FromOutgoingContext)
}

func metadataValueFrom(ctx context.Context, key string, getter func(context.Context) (metadata.MD, bool)) string {
	md, ok := getter(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func firstForwardedFor(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func parseTraceparent(value string) (string, string) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) < 4 {
		return "", ""
	}
	traceID := strings.ToLower(parts[1])
	spanID := strings.ToLower(parts[2])
	if len(traceID) != 32 || len(spanID) != 16 || allZeros(traceID) || allZeros(spanID) {
		return "", ""
	}
	return traceID, spanID
}

func allZeros(value string) bool {
	for _, r := range value {
		if r != '0' {
			return false
		}
	}
	return true
}
