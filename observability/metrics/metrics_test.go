package metrics

import (
	"context"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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
		t.Fatal("missing legacy grpc request metric")
	}
	if !hasMetric(m, "grpc_server_requests_total", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
		"code":    "Unavailable",
	}) {
		t.Fatal("missing grpc server request metric")
	}
	if !hasMetric(m, "grpc_server_started_total", map[string]string{
		"service":      "user",
		"grpc_type":    "unary",
		"grpc_service": "api.user.v1.UserService",
		"grpc_method":  "GetUser",
	}) {
		t.Fatal("missing compatible grpc started metric")
	}
	if !hasMetric(m, "grpc_server_handled_total", map[string]string{
		"service":      "user",
		"grpc_type":    "unary",
		"grpc_service": "api.user.v1.UserService",
		"grpc_method":  "GetUser",
		"grpc_code":    "Unavailable",
	}) {
		t.Fatal("missing compatible grpc handled metric")
	}
	if !hasMetric(m, "grpc_server_handling_seconds", map[string]string{
		"service":      "user",
		"grpc_type":    "unary",
		"grpc_service": "api.user.v1.UserService",
		"grpc_method":  "GetUser",
		"grpc_code":    "Unavailable",
	}) {
		t.Fatal("missing compatible grpc handling latency metric")
	}
	if !hasMetric(m, "grpc_server_request_duration_seconds", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
		"code":    "Unavailable",
	}) {
		t.Fatal("missing grpc server latency metric")
	}
	if got := metricValue(t, m, "runtime_service_info", map[string]string{"service": "user"}); got != 1 {
		t.Fatalf("runtime_service_info = %v, want 1", got)
	}
	if got := metricValue(t, m, "grpc_server_inflight_requests", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
	}); got != 0 {
		t.Fatalf("grpc_server_inflight_requests = %v, want 0 after completion", got)
	}
}

func TestUnaryServerInterceptorRecordsMessageSizes(t *testing.T) {
	m := New("user")
	interceptor := m.UnaryServerInterceptor()
	_, err := interceptor(
		context.Background(),
		&emptypb.Empty{},
		&grpc.UnaryServerInfo{FullMethod: "/api.user.v1.UserService/GetUser"},
		func(ctx context.Context, req any) (any, error) {
			return &emptypb.Empty{}, nil
		},
	)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if !hasMetric(m, "grpc_server_request_message_bytes", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
	}) {
		t.Fatal("missing grpc request message size metric")
	}
	if !hasMetric(m, "grpc_server_msg_received_total", map[string]string{
		"service":      "user",
		"grpc_type":    "unary",
		"grpc_service": "api.user.v1.UserService",
		"grpc_method":  "GetUser",
	}) {
		t.Fatal("missing compatible grpc message received metric")
	}
	if !hasMetric(m, "grpc_server_msg_sent_total", map[string]string{
		"service":      "user",
		"grpc_type":    "unary",
		"grpc_service": "api.user.v1.UserService",
		"grpc_method":  "GetUser",
	}) {
		t.Fatal("missing compatible grpc message sent metric")
	}
	if !hasMetric(m, "grpc_server_response_message_bytes", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
		"code":    "OK",
	}) {
		t.Fatal("missing grpc response message size metric")
	}
}

func TestUnaryServerInterceptorRecordsDeadlineExceeded(t *testing.T) {
	m := New("user")
	interceptor := m.UnaryServerInterceptor()
	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/api.user.v1.UserService/GetUser"},
		func(ctx context.Context, req any) (any, error) {
			return nil, status.Error(codes.DeadlineExceeded, "deadline")
		},
	)
	if err == nil {
		t.Fatal("expected handler error")
	}

	if !hasMetric(m, "grpc_server_deadline_exceeded_total", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
	}) {
		t.Fatal("missing grpc deadline exceeded metric")
	}
}

func TestUnaryServerInterceptorRecordsPanic(t *testing.T) {
	m := New("user")
	interceptor := m.UnaryServerInterceptor()

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("expected handler panic")
			}
		}()
		_, _ = interceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/api.user.v1.UserService/GetUser"},
			func(ctx context.Context, req any) (any, error) {
				panic("boom")
			},
		)
	}()

	if !hasMetric(m, "grpc_server_panics_total", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
	}) {
		t.Fatal("missing grpc panic metric")
	}
	if !hasMetric(m, "grpc_server_requests_total", map[string]string{
		"service": "user",
		"method":  "/api.user.v1.UserService/GetUser",
		"code":    "Internal",
	}) {
		t.Fatal("missing grpc internal request metric for panic")
	}
}

func TestControlPlaneMetricsRecordsStatusErrorsAndRecoveries(t *testing.T) {
	m := New("payment")
	controlPlane := NewControlPlaneMetrics("payment")
	m.MustRegister(controlPlane.Collectors()...)

	controlPlane.SetStatus("registry", false)
	controlPlane.RecordError("registry", "renew")
	controlPlane.RecordRecovery("registry", "renew")

	if got := metricValue(t, m, "runtime_control_plane_status", map[string]string{
		"service":   "payment",
		"component": "registry",
	}); got != 0 {
		t.Fatalf("control plane status = %v, want 0", got)
	}
	if !hasMetric(m, "runtime_control_plane_errors_total", map[string]string{
		"service":   "payment",
		"component": "registry",
		"operation": "renew",
	}) {
		t.Fatal("missing control-plane error metric")
	}
	if !hasMetric(m, "runtime_control_plane_recoveries_total", map[string]string{
		"service":   "payment",
		"component": "registry",
		"operation": "renew",
	}) {
		t.Fatal("missing control-plane recovery metric")
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
