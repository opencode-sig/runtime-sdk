package component

import (
	"context"
	"errors"
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
	// AdminAddr is the service admin HTTP address for /healthz, /metrics, and optional pprof.
	AdminAddr    string
	EnablePprof  bool
	Register     func(server *grpc.Server)
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
// The component installs tracing, metrics, RPC health, and optionally an Admin
// HTTP endpoint for /healthz, /metrics, and pprof.
func NewGRPCService(cfg GRPCConfig, logger *applogger.Logger) *GRPCService {
	return &GRPCService{
		cfg:     cfg,
		logger:  logger,
		metrics: metrics.New(cfg.Name),
	}
}

// Start starts the RPC server and optional Admin HTTP endpoint.
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

	if s.cfg.AdminAddr != "" {
		if err := s.startAdminHTTP(); err != nil {
			s.grpcServer.Stop()
			return err
		}
	}
	return nil
}

// Stop gracefully stops the service.
//
// It first marks gRPC health as NOT_SERVING, then shuts down Admin HTTP, then
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

// startAdminHTTP starts the service Admin HTTP endpoint.
//
// This endpoint only serves /healthz, /metrics, and optional pprof; it does not
// serve business HTTP routes.
func (s *GRPCService) startAdminHTTP() error {
	mux := http.NewServeMux()
	checker := health.New()
	checker.Add("self", func(ctx context.Context) error { return s.Health(ctx) })
	for name, check := range s.cfg.HealthChecks {
		checker.Add(name, check)
	}

	mux.Handle("/metrics", s.metrics.Handler())
	mux.HandleFunc("/healthz", checker.Handler())
	if s.cfg.EnablePprof {
		mountPprof(mux)
	}

	s.httpServer = &http.Server{
		Addr:              s.cfg.AdminAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", s.cfg.AdminAddr)
	if err != nil {
		return err
	}
	go func() {
		if s.logger != nil {
			s.logger.Warn(context.Background(), "service admin server started",
				applogger.Event("service_admin_server_started"),
				applogger.Module(s.cfg.Name),
				applogger.String("addr", s.cfg.AdminAddr),
			)
		}
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if s.logger != nil {
				s.logger.Error(context.Background(), "service admin server stopped unexpectedly", applogger.ErrorFields(err)...)
			}
		}
	}()
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
