package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type Metrics struct {
	service      string
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpLatency  *prometheus.HistogramVec
	grpcRequests *prometheus.CounterVec
	grpcLatency  *prometheus.HistogramVec
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
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		m.httpRequests,
		m.httpLatency,
		m.grpcRequests,
		m.grpcLatency,
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
		resp, err := handler(ctx, req)
		code := status.Code(err).String()

		m.grpcRequests.WithLabelValues(m.service, info.FullMethod, code).Inc()
		m.grpcLatency.WithLabelValues(m.service, info.FullMethod).Observe(time.Since(start).Seconds())
		return resp, err
	}
}
