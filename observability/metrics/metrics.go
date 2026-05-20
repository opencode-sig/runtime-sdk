package metrics

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Metrics struct {
	service            string
	registry           *prometheus.Registry
	serviceInfo        *prometheus.GaugeVec
	httpRequests       *prometheus.CounterVec
	httpLatency        *prometheus.HistogramVec
	grpcRequests       *prometheus.CounterVec
	grpcLatency        *prometheus.HistogramVec
	grpcStarted        *prometheus.CounterVec
	grpcHandled        *prometheus.CounterVec
	grpcHandling       *prometheus.HistogramVec
	grpcMsgReceived    *prometheus.CounterVec
	grpcMsgSent        *prometheus.CounterVec
	grpcServerRequests *prometheus.CounterVec
	grpcServerLatency  *prometheus.HistogramVec
	grpcInflight       *prometheus.GaugeVec
	grpcPanics         *prometheus.CounterVec
	grpcDeadline       *prometheus.CounterVec
	grpcRequestBytes   *prometheus.HistogramVec
	grpcResponseBytes  *prometheus.HistogramVec
}

// New creates a Prometheus metrics collection for one service.
//
// Each runtime component uses an isolated registry to avoid global collector
// conflicts in tests and multi-service processes.
func New(service string) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		service:  service,
		registry: reg,
		serviceInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "runtime_service_info",
				Help: "Runtime service information. Value is always 1 for a running service metrics registry.",
			},
			[]string{"service"},
		),
		httpRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "runtime_http_requests_total",
				Help: "Total HTTP requests handled by the runtime.",
			},
			[]string{"service", "method", "route", "status"},
		),
		httpLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "runtime_http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "method", "route"},
		),
		grpcRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "runtime_grpc_requests_total",
				Help: "Total gRPC requests handled by the runtime.",
			},
			[]string{"service", "method", "code"},
		),
		grpcLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "runtime_grpc_request_duration_seconds",
				Help:    "gRPC request latency in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "method"},
		),
		grpcStarted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_started_total",
				Help: "Total number of gRPC server RPCs started.",
			},
			[]string{"service", "grpc_type", "grpc_service", "grpc_method"},
		),
		grpcHandled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_handled_total",
				Help: "Total number of gRPC server RPCs completed, labeled by status code.",
			},
			[]string{"service", "grpc_type", "grpc_service", "grpc_method", "grpc_code"},
		),
		grpcHandling: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_handling_seconds",
				Help:    "Histogram of gRPC server RPC handling latency in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "grpc_type", "grpc_service", "grpc_method", "grpc_code"},
		),
		grpcMsgReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_msg_received_total",
				Help: "Total number of gRPC server request messages received.",
			},
			[]string{"service", "grpc_type", "grpc_service", "grpc_method"},
		),
		grpcMsgSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_msg_sent_total",
				Help: "Total number of gRPC server response messages sent.",
			},
			[]string{"service", "grpc_type", "grpc_service", "grpc_method"},
		),
		grpcServerRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_requests_total",
				Help: "Total gRPC server requests handled by the runtime.",
			},
			[]string{"service", "method", "code"},
		),
		grpcServerLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_request_duration_seconds",
				Help:    "gRPC server request latency in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "method", "code"},
		),
		grpcInflight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "grpc_server_inflight_requests",
				Help: "Current in-flight gRPC server requests.",
			},
			[]string{"service", "method"},
		),
		grpcPanics: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_panics_total",
				Help: "Total gRPC server handler panics.",
			},
			[]string{"service", "method"},
		),
		grpcDeadline: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_deadline_exceeded_total",
				Help: "Total gRPC server requests that finished with DeadlineExceeded.",
			},
			[]string{"service", "method"},
		),
		grpcRequestBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_request_message_bytes",
				Help:    "gRPC server request protobuf message size in bytes.",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16),
			},
			[]string{"service", "method"},
		),
		grpcResponseBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_response_message_bytes",
				Help:    "gRPC server response protobuf message size in bytes.",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16),
			},
			[]string{"service", "method", "code"},
		),
	}
	m.serviceInfo.WithLabelValues(m.service).Set(1)

	reg.MustRegister(
		collectors.NewGoCollector(),
		m.serviceInfo,
		m.httpRequests,
		m.httpLatency,
		m.grpcRequests,
		m.grpcLatency,
		m.grpcStarted,
		m.grpcHandled,
		m.grpcHandling,
		m.grpcMsgReceived,
		m.grpcMsgSent,
		m.grpcServerRequests,
		m.grpcServerLatency,
		m.grpcInflight,
		m.grpcPanics,
		m.grpcDeadline,
		m.grpcRequestBytes,
		m.grpcResponseBytes,
	)
	return m
}

// Handler returns the HTTP handler used by /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// MustRegister registers custom collectors.
func (m *Metrics) MustRegister(collectors ...prometheus.Collector) {
	m.registry.MustRegister(collectors...)
}

// Gather returns the metric families currently registered.
func (m *Metrics) Gather() ([]*dto.MetricFamily, error) {
	return m.registry.Gather()
}

// RecordHTTPRequest records one generic HTTP request.
func (m *Metrics) RecordHTTPRequest(method string, route string, status string, duration time.Duration) {
	if route == "" {
		route = "unknown"
	}
	m.httpRequests.WithLabelValues(m.service, method, route, status).Inc()
	m.httpLatency.WithLabelValues(m.service, method, route).Observe(duration.Seconds())
}

// UnaryServerInterceptor records gRPC server request count, duration, and status code.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		method := grpcMethod(info)
		grpcService, grpcMethodName := grpcMethodParts(method)
		m.grpcStarted.WithLabelValues(m.service, "unary", grpcService, grpcMethodName).Inc()
		if req != nil {
			m.grpcMsgReceived.WithLabelValues(m.service, "unary", grpcService, grpcMethodName).Inc()
		}
		if size, ok := protoMessageSize(req); ok {
			m.grpcRequestBytes.WithLabelValues(m.service, method).Observe(size)
		}
		m.grpcInflight.WithLabelValues(m.service, method).Inc()
		defer m.grpcInflight.WithLabelValues(m.service, method).Dec()
		defer func() {
			if recovered := recover(); recovered != nil {
				m.grpcPanics.WithLabelValues(m.service, method).Inc()
				m.recordGRPCServerRequest(method, codes.Internal.String(), time.Since(start), nil)
				panic(recovered)
			}
		}()

		resp, err := handler(ctx, req)
		code := status.Code(err).String()

		m.grpcRequests.WithLabelValues(m.service, method, code).Inc()
		m.grpcLatency.WithLabelValues(m.service, method).Observe(time.Since(start).Seconds())
		m.recordGRPCServerRequest(method, code, time.Since(start), resp)
		return resp, err
	}
}

func (m *Metrics) recordGRPCServerRequest(method string, code string, duration time.Duration, resp any) {
	grpcService, grpcMethodName := grpcMethodParts(method)
	m.grpcHandled.WithLabelValues(m.service, "unary", grpcService, grpcMethodName, code).Inc()
	m.grpcHandling.WithLabelValues(m.service, "unary", grpcService, grpcMethodName, code).Observe(duration.Seconds())
	m.grpcServerRequests.WithLabelValues(m.service, method, code).Inc()
	m.grpcServerLatency.WithLabelValues(m.service, method, code).Observe(duration.Seconds())
	if code == codes.DeadlineExceeded.String() {
		m.grpcDeadline.WithLabelValues(m.service, method).Inc()
	}
	if resp != nil {
		m.grpcMsgSent.WithLabelValues(m.service, "unary", grpcService, grpcMethodName).Inc()
	}
	if size, ok := protoMessageSize(resp); ok {
		m.grpcResponseBytes.WithLabelValues(m.service, method, code).Observe(size)
	}
}

func grpcMethod(info *grpc.UnaryServerInfo) string {
	if info == nil || info.FullMethod == "" {
		return "unknown"
	}
	return info.FullMethod
}

func grpcMethodParts(fullMethod string) (string, string) {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	parts := strings.SplitN(fullMethod, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "unknown", "unknown"
	}
	return parts[0], parts[1]
}

func protoMessageSize(value any) (float64, bool) {
	msg, ok := value.(proto.Message)
	if !ok || msg == nil {
		return 0, false
	}
	return float64(proto.Size(msg)), true
}
