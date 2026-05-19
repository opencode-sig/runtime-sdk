package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// InitNoop initializes a noop OpenTelemetry tracer provider.
//
// The SDK keeps tracing boundaries and context propagation available without
// requiring an external collector. Replace this when a real exporter is used.
func InitNoop(_ string) func(context.Context) error {
	otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return func(ctx context.Context) error {
		return nil
	}
}

// UnaryClientInterceptor creates a span for gRPC client calls and injects trace context.
func UnaryClientInterceptor(service string) grpc.UnaryClientInterceptor {
	tracer := otel.Tracer(service + ".grpc.client")
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, method string, req any, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, span := tracer.Start(ctx, method)
		defer span.End()

		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		if md == nil {
			md = metadata.MD{}
		}
		propagator.Inject(ctx, metadataCarrier{md: md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerInterceptor extracts trace context and creates a span for gRPC server calls.
func UnaryServerInterceptor(service string) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(service + ".grpc.server")
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = propagator.Extract(ctx, metadataCarrier{md: md.Copy()})
		ctx, span := tracer.Start(ctx, info.FullMethod)
		defer span.End()

		return handler(ctx, req)
	}
}

type metadataCarrier struct {
	md metadata.MD
}

// Get implements propagation.TextMapCarrier by reading from gRPC metadata.
func (c metadataCarrier) Get(key string) string {
	values := c.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Set implements propagation.TextMapCarrier by writing to gRPC metadata.
func (c metadataCarrier) Set(key string, value string) {
	c.md.Set(key, value)
}

// Keys returns all keys currently present in metadata.
func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for key := range c.md {
		keys = append(keys, key)
	}
	return keys
}
