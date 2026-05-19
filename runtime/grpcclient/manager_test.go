package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	runtimediscovery "github.com/opencode-sig/runtime-sdk/runtime/discovery"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestManagerConnUsesRuntimeDiscovery(t *testing.T) {
	addr, stop := startHealthServer(t)
	defer stop()

	reg := registry.NewMemoryRegistry()
	if _, err := reg.Register(context.Background(), registry.NewServiceInstance("health", addr, nil)); err != nil {
		t.Fatalf("register instance: %v", err)
	}
	manager := NewManager(NewResolverBuilder(runtimediscovery.NewMemoryDiscovery(reg)))
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := manager.Conn(ctx, "health")
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	client := healthv1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &healthv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if resp.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %s", resp.GetStatus())
	}

	again, err := manager.Conn(ctx, "health")
	if err != nil {
		t.Fatalf("conn again: %v", err)
	}
	if again != conn {
		t.Fatal("manager did not reuse connection")
	}
}

func TestManagerRejectsEmptyService(t *testing.T) {
	manager := NewManager(NewResolverBuilder(runtimediscovery.NewMemoryDiscovery(registry.NewMemoryRegistry())))
	if _, err := manager.Conn(context.Background(), " "); err == nil {
		t.Fatal("expected empty service error")
	}
}

func TestManagerCloseRejectsNewConnections(t *testing.T) {
	manager := NewManager(NewResolverBuilder(runtimediscovery.NewMemoryDiscovery(registry.NewMemoryRegistry())))
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := manager.Health(context.Background()); err == nil {
		t.Fatal("expected closed health error")
	}
}

func startHealthServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(server, healthServer)
	go func() {
		_ = server.Serve(listener)
	}()
	return listener.Addr().String(), func() {
		server.Stop()
		_ = listener.Close()
	}
}
