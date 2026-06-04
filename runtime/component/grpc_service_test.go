package component

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
)

func TestGRPCServiceRegistersServiceHTTPHandlers(t *testing.T) {
	grpcAddr := freeLocalTCPAddr(t)
	httpAddr := freeLocalTCPAddr(t)
	service := NewGRPCService(GRPCConfig{
		Name:     "payment",
		GRPCAddr: grpcAddr,
		HTTPAddr: httpAddr,
		Register: func(server *grpc.Server) {},
		RegisterHTTP: func(mux *http.ServeMux) {
			mux.HandleFunc("/internal/ping", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("pong"))
			})
		},
	}, nil)

	if err := service.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = service.Stop(context.Background())
	})

	body := getEventually(t, "http://"+httpAddr+"/internal/ping")
	if body != "pong" {
		t.Fatalf("business http body = %q, want pong", body)
	}
	health := getEventually(t, "http://"+httpAddr+"/healthz")
	if health == "" {
		t.Fatal("healthz body is empty")
	}
	ready := getEventually(t, "http://"+httpAddr+"/readyz")
	if ready == "" {
		t.Fatal("readyz body is empty")
	}
}

func TestGRPCServiceReadinessDoesNotAffectLiveness(t *testing.T) {
	grpcAddr := freeLocalTCPAddr(t)
	httpAddr := freeLocalTCPAddr(t)
	service := NewGRPCService(GRPCConfig{
		Name:     "payment",
		GRPCAddr: grpcAddr,
		HTTPAddr: httpAddr,
		Register: func(server *grpc.Server) {},
		ReadinessChecks: map[string]func(context.Context) error{
			"mysql": func(context.Context) error {
				return errors.New("mysql unavailable")
			},
		},
	}, nil)

	if err := service.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = service.Stop(context.Background())
	})

	if got := getStatusEventually(t, "http://"+httpAddr+"/healthz", http.StatusOK); got != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", got)
	}
	if got := getStatusEventually(t, "http://"+httpAddr+"/readyz", http.StatusServiceUnavailable); got != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", got)
	}
}

func TestGRPCServiceExposesControlPlaneMetrics(t *testing.T) {
	grpcAddr := freeLocalTCPAddr(t)
	httpAddr := freeLocalTCPAddr(t)
	controlPlane := runtimemetrics.NewControlPlaneMetrics("payment")
	controlPlane.SetStatus("registry", false)
	controlPlane.RecordError("registry", "renew")
	service := NewGRPCService(GRPCConfig{
		Name:         "payment",
		GRPCAddr:     grpcAddr,
		HTTPAddr:     httpAddr,
		Register:     func(server *grpc.Server) {},
		ControlPlane: controlPlane,
	}, nil)

	if err := service.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = service.Stop(context.Background())
	})

	body := getEventually(t, "http://"+httpAddr+"/metrics")
	if !strings.Contains(body, "runtime_control_plane_status") {
		t.Fatal("missing control-plane status metric")
	}
	if !strings.Contains(body, "runtime_control_plane_errors_total") {
		t.Fatal("missing control-plane error metric")
	}
}

func freeLocalTCPAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close free port listener: %v", err)
	}
	return addr
}

func getEventually(t *testing.T, url string) string {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			data, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				t.Fatalf("read %s: %v", url, readErr)
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return string(data)
			}
			lastErr = &statusError{status: resp.Status}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("get %s: %v", url, lastErr)
	return ""
}

func getStatusEventually(t *testing.T, url string, want int) int {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	var lastStatus int
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastStatus = resp.StatusCode
			if resp.StatusCode == want {
				return resp.StatusCode
			}
			lastErr = &statusError{status: resp.Status}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	if lastStatus != 0 {
		return lastStatus
	}
	t.Fatalf("get %s: %v", url, lastErr)
	return 0
}

type statusError struct {
	status string
}

func (e *statusError) Error() string {
	return e.status
}
