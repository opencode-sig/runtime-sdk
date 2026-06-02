package component

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
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

type statusError struct {
	status string
}

func (e *statusError) Error() string {
	return e.status
}
