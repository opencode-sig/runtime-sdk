package bootstrap

import (
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

func TestGatewayPublicationIncludesBusinessHTTPProxyFixture(t *testing.T) {
	routes, descriptors, err := GatewayPublication()
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(descriptors) == 0 {
		t.Fatal("descriptor set is required for payment gRPC routes")
	}

	var found bool
	for _, route := range routes {
		if route.ID != "payment.http_report" {
			continue
		}
		found = true
		if route.Backend == nil || route.Backend.Type != gatewaymeta.BackendTypeHTTP || route.Backend.HTTP == nil {
			t.Fatalf("backend = %#v, want http backend", route.Backend)
		}
		if route.Backend.HTTP.Service != ServiceName {
			t.Fatalf("backend service = %q, want %q", route.Backend.HTTP.Service, ServiceName)
		}
		if route.Backend.HTTP.Path != "/internal/payments/report" {
			t.Fatalf("backend path = %q, want /internal/payments/report", route.Backend.HTTP.Path)
		}
	}
	if !found {
		t.Fatal("payment.http_report route was not published")
	}
}
