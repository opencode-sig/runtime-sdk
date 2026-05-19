package metrics

import (
	"context"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecordHTTPRequestRecordsRouteAndStatus(t *testing.T) {
	m := New("gateway")
	m.RecordHTTPRequest("GET", "/v1/resources/:id", "202", 10*time.Millisecond)

	if !hasMetric(m, "runtime_http_requests_total", map[string]string{
		"service": "gateway",
		"method":  "GET",
		"route":   "/v1/resources/:id",
		"status":  "202",
	}) {
		t.Fatal("missing http request metric")
	}
}

func TestUnaryServerInterceptorRecordsCode(t *testing.T) {
	m := New("user")
	interceptor := m.UnaryServerInterceptor()
	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/api.user.v1.UserService/GetUser"},
		func(ctx context.Context, req any) (any, error) {
			return nil, status.Error(codes.Unavailable, "upstream unavailable")
		},
	)
	if err == nil {
		t.Fatal("expected handler error")
	}

	if !hasMetric(m, "runtime_grpc_requests_total", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
		"code":    "Unavailable",
	}) {
		t.Fatal("missing grpc request metric")
	}
}

func hasMetric(m *Metrics, name string, labels map[string]string) bool {
	families, err := m.Gather()
	if err != nil {
		panic(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric, labels) {
				return true
			}
		}
	}
	return false
}

func metricValue(t *testing.T, m *Metrics, name string, labels map[string]string) float64 {
	t.Helper()

	families, err := m.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !labelsMatch(metric, labels) {
				continue
			}
			if metric.Gauge == nil {
				t.Fatalf("metric %s is not a gauge", name)
			}
			return metric.Gauge.GetValue()
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}

func labelsMatch(metric *dto.Metric, want map[string]string) bool {
	got := make(map[string]string, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		got[label.GetName()] = label.GetValue()
	}
	for name, value := range want {
		if got[name] != value {
			return false
		}
	}
	return true
}
