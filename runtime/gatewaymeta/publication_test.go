package gatewaymeta

import (
	"testing"
)

func TestNewGatewayPublication(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}").Path("id", "id").Timeout("3s"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes length = %d", len(routes))
	}
	if routes[0].GRPC.FullMethod != "/api.user.v1.UserService/GetUser" {
		t.Fatalf("full method = %q", routes[0].GRPC.FullMethod)
	}
	if routes[0].ID != "user.get" {
		t.Fatalf("route id = %q", routes[0].ID)
	}
	if routes[0].HTTP.Path != "/user/v1/users/{id}" {
		t.Fatalf("http path = %q", routes[0].HTTP.Path)
	}
	if routes[0].GRPC.DescriptorID != "api.user.v1" {
		t.Fatalf("descriptor id = %q", routes[0].GRPC.DescriptorID)
	}
	if len(descriptors["api.user.v1"]) == 0 {
		t.Fatal("descriptor set is empty")
	}
}

func TestNewGatewayPublicationRejectsUnknownMethod(t *testing.T) {
	_, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("Missing", "/v1/users/{id}"),
		},
	})
	if err == nil {
		t.Fatal("expected gateway publication error")
	}
}

func TestServiceRoutePath(t *testing.T) {
	tests := []struct {
		name    string
		service string
		path    string
		want    string
	}{
		{name: "local absolute path", service: "user", path: "/v1/users/{id}", want: "/user/v1/users/{id}"},
		{name: "local relative path", service: "user", path: "v1/users/{id}", want: "/user/v1/users/{id}"},
		{name: "already prefixed", service: "user", path: "/user/v1/users/{id}", want: "/user/v1/users/{id}"},
		{name: "extra slashes", service: "/payment/", path: "/v1/payments/", want: "/payment/v1/payments"},
		{name: "root path", service: "health", path: "/", want: "/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceRoutePath(tt.service, tt.path); got != tt.want {
				t.Fatalf("service route path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultGatewayRouteID(t *testing.T) {
	tests := []struct {
		service string
		method  string
		want    string
	}{
		{service: "user", method: "GetUser", want: "user.get"},
		{service: "order", method: "CreateOrder", want: "order.create"},
		{service: "order", method: "ListUserOrders", want: "order.list_user_orders"},
		{service: "billing_account", method: "GetBillingAccount", want: "billing_account.get"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := defaultGatewayRouteID(tt.service, tt.method); got != tt.want {
				t.Fatalf("route id = %q", got)
			}
		})
	}
}
