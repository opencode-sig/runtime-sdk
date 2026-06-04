package component

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	applogger "github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/observability/health"
	"github.com/opencode-sig/runtime-sdk/observability/metrics"
	"github.com/opencode-sig/runtime-sdk/observability/tracing"
)

type GRPCConfig struct {
	Name     string
	GRPCAddr string
	// HTTPAddr is the service HTTP listener for /healthz, /readyz, /metrics, and optional pprof.
	HTTPAddr        string
	EnablePprof     bool
	Register        func(server *grpc.Server)
	RegisterHTTP    func(mux *http.ServeMux)
	ReadinessChecks map[string]func(context.Context) error
	ControlPlane    *metrics.ControlPlaneMetrics
	// HealthChecks is kept for compatibility and is evaluated by /readyz.
	HealthChecks map[string]func(context.Context) error
}

type GRPCService struct {
	cfg          GRPCConfig
	logger       *applogger.Logger
	metrics      *metrics.Metrics
	grpcServer   *grpc.Server
	healthServer *grpchealth.Server
	httpServer   *http.Server
}

// NewGRPCService creates a lifecycle-managed gRPC service component.
//
// The component installs tracing, metrics, RPC health, and optionally a service
// HTTP listener for /healthz, /readyz, /metrics, and pprof.
func NewGRPCService(cfg GRPCConfig, logger *applogger.Logger) *GRPCService {
	serviceMetrics := metrics.New(cfg.Name)
	serviceMetrics.MustRegister(cfg.ControlPlane.Collectors()...)
	return &GRPCService{
		cfg:     cfg,
		logger:  logger,
		metrics: serviceMetrics,
	}
}

// Start starts the RPC server and optional service HTTP listener.
//
// Register lets service modules register generated protobuf servers. The
// runtime layer only provides the gRPC server container.
func (s *GRPCService) Start(ctx context.Context) error {
	grpcListener, err := net.Listen("tcp", s.cfg.GRPCAddr)
	if err != nil {
		return err
	}

	s.healthServer = grpchealth.NewServer()
	s.grpcServer = grpc.NewServer(grpc.ChainUnaryInterceptor(
		tracing.UnaryServerInterceptor(s.cfg.Name),
		s.metrics.UnaryServerInterceptor(),
	))
	healthpb.RegisterHealthServer(s.grpcServer, s.healthServer)
	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	if s.cfg.Register != nil {
		s.cfg.Register(s.grpcServer)
	}

	go func() {
		if s.logger != nil {
			s.logger.Warn(context.Background(), "grpc server started",
				applogger.Event("grpc_server_started"),
				applogger.Module(s.cfg.Name),
				applogger.String("addr", s.cfg.GRPCAddr),
			)
		}
		if err := s.grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			if s.logger != nil {
				s.logger.Error(context.Background(), "grpc server stopped unexpectedly", applogger.ErrorFields(err)...)
			}
		}
	}()

	if s.cfg.HTTPAddr != "" {
		if err := s.startHTTP(); err != nil {
			s.grpcServer.Stop()
			return err
		}
	}
	return nil
}

// Stop gracefully stops the service.
//
// It first marks gRPC health as NOT_SERVING, then shuts down service HTTP, then
// waits for in-flight RPCs with GracefulStop before falling back to Stop.
func (s *GRPCService) Stop(ctx context.Context) error {
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	}

	var httpErr error
	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
	}

	if s.grpcServer != nil {
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			s.grpcServer.Stop()
			return errors.Join(httpErr, ctx.Err())
		}
	}
	return httpErr
}

// Health checks whether the gRPC server has started.
func (s *GRPCService) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if s.grpcServer == nil {
		return errors.New("grpc server is not started")
	}
	return nil
}

// startHTTP starts the service HTTP listener.
//
// Runtime reserves /healthz, /readyz, /metrics, and optional pprof paths on this
// listener. Business HTTP traffic must still be declared through Gateway
// metadata before a Gateway proxies to this address.
func (s *GRPCService) startHTTP() error {
	mux := http.NewServeMux()
	liveness := health.New()
	liveness.Add("self", func(ctx context.Context) error { return s.Health(ctx) })
	readiness := health.New()
	readiness.Add("self", func(ctx context.Context) error { return s.Health(ctx) })
	for name, check := range s.cfg.HealthChecks {
		readiness.Add(name, check)
	}
	for name, check := range s.cfg.ReadinessChecks {
		readiness.Add(name, check)
	}

	mux.Handle("/metrics", s.metrics.Handler())
	mux.HandleFunc("/healthz", liveness.Handler())
	mux.HandleFunc("/readyz", readiness.Handler())
	if s.cfg.EnablePprof {
		mountPprof(mux)
	}
	if err := s.registerServiceHTTP(mux); err != nil {
		return err
	}

	s.httpServer = &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", s.cfg.HTTPAddr)
	if err != nil {
		return err
	}
	go func() {
		if s.logger != nil {
			s.logger.Warn(context.Background(), "service http server started",
				applogger.Event("service_http_server_started"),
				applogger.Module(s.cfg.Name),
				applogger.String("addr", s.cfg.HTTPAddr),
			)
		}
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if s.logger != nil {
				s.logger.Error(context.Background(), "service http server stopped unexpectedly", applogger.ErrorFields(err)...)
			}
		}
	}()
	return nil
}

func (s *GRPCService) registerServiceHTTP(mux *http.ServeMux) (err error) {
	if s.cfg.RegisterHTTP == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register service http handlers: %v", recovered)
		}
	}()
	s.cfg.RegisterHTTP(mux)
	return nil
}

// mountPprof mounts standard library pprof handlers on the service HTTP mux.
func mountPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
