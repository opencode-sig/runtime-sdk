package gatewaymeta

import (
	"encoding/json"
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
	if routes[0].HTTP.Path != "/v1/users/{id}" {
		t.Fatalf("http path = %q", routes[0].HTTP.Path)
	}
	if routes[0].GRPC.DescriptorID != "api.user.v1" {
		t.Fatalf("descriptor id = %q", routes[0].GRPC.DescriptorID)
	}
	if len(descriptors["api.user.v1"]) == 0 {
		t.Fatal("descriptor set is empty")
	}
	if routes[0].Response != nil {
		t.Fatalf("default route response policy = %#v, want nil", routes[0].Response)
	}
	data, err := json.Marshal(routes[0])
	if err != nil {
		t.Fatalf("marshal route: %v", err)
	}
	if json.Valid(data) && string(data) != "" && containsJSONField(data, "response") {
		t.Fatalf("default route should omit response field: %s", string(data))
	}
}

func TestNewGatewayPublicationRawResponse(t *testing.T) {
	routes, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}").
				Path("id", "id").
				RawResponse("text/html; charset=utf-8").
				RawBody("html").
				RawStatus("http_status").
				RawHeaders("response_headers"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	raw := routes[0].Response.Raw
	if raw == nil {
		t.Fatal("raw response policy is required")
	}
	if raw.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", raw.ContentType)
	}
	if raw.Body != "html" {
		t.Fatalf("body = %q", raw.Body)
	}
	if raw.Status != "http_status" {
		t.Fatalf("status = %q", raw.Status)
	}
	if raw.Headers != "response_headers" {
		t.Fatalf("headers = %q", raw.Headers)
	}
}

func TestNewGatewayPublicationPublicRoute(t *testing.T) {
	routes, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}").Path("id", "id").Public(),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if routes[0].Auth == nil || !routes[0].Auth.Public {
		t.Fatalf("auth policy = %#v", routes[0].Auth)
	}
}

func containsJSONField(data []byte, field string) bool {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return false
	}
	_, ok := object[field]
	return ok
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

func TestGatewayRoutePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "absolute path", path: "/v1/users/{id}", want: "/v1/users/{id}"},
		{name: "relative path", path: "v1/users/{id}", want: "/v1/users/{id}"},
		{name: "extra slashes", path: "/v1/payments/", want: "/v1/payments"},
		{name: "root path", path: "/", want: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gatewayRoutePath(tt.path); got != tt.want {
				t.Fatalf("gateway route path = %q, want %q", got, tt.want)
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
