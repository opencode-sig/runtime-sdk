package gatewaymeta

import (
	"encoding/json"
	"net/http"
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
	if routes[0].Backend == nil || routes[0].Backend.Type != BackendTypeGRPC || routes[0].Backend.GRPC == nil {
		t.Fatalf("backend = %#v, want grpc backend", routes[0].Backend)
	}
	if routes[0].Backend.GRPC.FullMethod != routes[0].GRPC.FullMethod {
		t.Fatalf("backend full method = %q, want %q", routes[0].Backend.GRPC.FullMethod, routes[0].GRPC.FullMethod)
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

func TestNewGatewayPublicationHTTPProxy(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", "legacy-api").
				UpstreamPath("/orders/search").
				Timeout("3s"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(descriptors) != 0 {
		t.Fatalf("descriptors = %v, want empty", descriptors)
	}
	if len(routes) != 1 {
		t.Fatalf("routes length = %d", len(routes))
	}
	route := routes[0]
	if route.ID != "legacy.orders_search" {
		t.Fatalf("route id = %q", route.ID)
	}
	if route.Backend == nil || route.Backend.Type != BackendTypeHTTP || route.Backend.HTTP == nil {
		t.Fatalf("backend = %#v, want http backend", route.Backend)
	}
	if route.Backend.HTTP.Service != "legacy-api" {
		t.Fatalf("backend service = %q", route.Backend.HTTP.Service)
	}
	if route.Backend.HTTP.Path != "/orders/search" {
		t.Fatalf("backend path = %q", route.Backend.HTTP.Path)
	}
	if route.GRPC.FullMethod != "" {
		t.Fatalf("grpc full method = %q, want empty", route.GRPC.FullMethod)
	}
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("marshal route: %v", err)
	}
	if containsJSONField(data, "grpc") {
		t.Fatalf("http proxy route should omit grpc field: %s", string(data))
	}
}

func TestNewGatewayPublicationHTTPSSEProxy(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			HTTPProxy("legacy.events_sse", "GET", "/v1/legacy/events/stream", "legacy-api").
				UpstreamPath("/events/stream").
				SSE().
				Timeout("5s"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(descriptors) != 0 {
		t.Fatalf("descriptors = %v, want empty", descriptors)
	}
	if len(routes) != 1 {
		t.Fatalf("routes length = %d", len(routes))
	}
	route := routes[0]
	if route.Backend == nil || route.Backend.Type != BackendTypeHTTP || route.Backend.HTTP == nil {
		t.Fatalf("backend = %#v, want http backend", route.Backend)
	}
	if route.Backend.HTTP.Stream == nil || route.Backend.HTTP.Stream.Mode != HTTPStreamModeSSE {
		t.Fatalf("http stream = %#v", route.Backend.HTTP.Stream)
	}
}

func TestNewGatewayPublicationWSProxy(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api").
				UpstreamWSPath("/events/stream").
				Timeout("3s"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(descriptors) != 0 {
		t.Fatalf("descriptors = %v, want empty", descriptors)
	}
	if len(routes) != 1 {
		t.Fatalf("routes length = %d", len(routes))
	}
	route := routes[0]
	if route.Backend == nil || route.Backend.Type != BackendTypeWebSocket || route.Backend.WebSocket == nil {
		t.Fatalf("backend = %#v, want websocket backend", route.Backend)
	}
	if route.Backend.WebSocket.Service != "legacy-api" {
		t.Fatalf("backend service = %q", route.Backend.WebSocket.Service)
	}
	if route.Backend.WebSocket.Path != "/events/stream" {
		t.Fatalf("backend path = %q", route.Backend.WebSocket.Path)
	}
	if route.HTTP.Method != http.MethodGet {
		t.Fatalf("http method = %q, want GET", route.HTTP.Method)
	}
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("marshal route: %v", err)
	}
	if containsJSONField(data, "grpc") {
		t.Fatalf("websocket proxy route should omit grpc field: %s", string(data))
	}
}

func TestNewGatewayPublicationHTTPProxyAnyMethod(t *testing.T) {
	routes, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			HTTPProxy("legacy.any", "ANY", "/v1/legacy/{path}", "legacy-api"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if routes[0].HTTP.Method != HTTPMethodAny {
		t.Fatalf("http method = %q, want ANY", routes[0].HTTP.Method)
	}

	routes, _, err = NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			HTTPProxy("legacy.star", "*", "/v1/legacy-star/{path}", "legacy-api"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication with star method: %v", err)
	}
	if routes[0].HTTP.Method != HTTPMethodAny {
		t.Fatalf("star http method = %q, want ANY", routes[0].HTTP.Method)
	}
}

func TestNewGatewayPublicationMixedGRPCAndHTTPProxy(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}").Path("id", "id"),
			HTTPProxy("user.legacy_profile", "GET", "/v1/users/{id}/legacy-profile", "legacy-user").
				UpstreamPath("/users/profile"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("routes length = %d", len(routes))
	}
	if len(descriptors["api.user.v1"]) == 0 {
		t.Fatal("descriptor set is empty")
	}
	if routes[0].Backend.Type != BackendTypeGRPC {
		t.Fatalf("first backend = %q, want grpc", routes[0].Backend.Type)
	}
	if routes[1].Backend.Type != BackendTypeHTTP {
		t.Fatalf("second backend = %q, want http", routes[1].Backend.Type)
	}
}

func TestNewGatewayPublicationMixedGRPCHTTPAndWSProxy(t *testing.T) {
	routes, descriptors, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}").Path("id", "id"),
			HTTPProxy("user.legacy_profile", "GET", "/v1/users/{id}/legacy-profile", "legacy-user").
				UpstreamPath("/users/profile"),
			WSProxy("user.events_stream", "/v1/users/events/stream", "legacy-user").
				UpstreamWSPath("/events/stream"),
		},
	})
	if err != nil {
		t.Fatalf("gateway publication: %v", err)
	}
	if len(routes) != 3 {
		t.Fatalf("routes length = %d", len(routes))
	}
	if len(descriptors["api.user.v1"]) == 0 {
		t.Fatal("descriptor set is empty")
	}
	if routes[0].Backend.Type != BackendTypeGRPC {
		t.Fatalf("first backend = %q, want grpc", routes[0].Backend.Type)
	}
	if routes[1].Backend.Type != BackendTypeHTTP {
		t.Fatalf("second backend = %q, want http", routes[1].Backend.Type)
	}
	if routes[2].Backend.Type != BackendTypeWebSocket {
		t.Fatalf("third backend = %q, want websocket", routes[2].Backend.Type)
	}
}

func TestNewGatewayPublicationHTTPProxyRejectsInvalidSpec(t *testing.T) {
	tests := []struct {
		name string
		spec GatewayRouteSpec
	}{
		{
			name: "missing id",
			spec: HTTPProxy("", "POST", "/v1/legacy/orders/search", "legacy-api"),
		},
		{
			name: "missing service",
			spec: HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", ""),
		},
		{
			name: "binding",
			spec: HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", "legacy-api").Body("*"),
		},
		{
			name: "raw response",
			spec: HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", "legacy-api").RawResponse("text/plain"),
		},
		{
			name: "sse non get",
			spec: HTTPProxy("legacy.events_sse", "POST", "/v1/legacy/events/stream", "legacy-api").SSE(),
		},
		{
			name: "url upstream path",
			spec: HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", "legacy-api").UpstreamPath("http://legacy/orders"),
		},
		{
			name: "query upstream path",
			spec: HTTPProxy("legacy.orders_search", "POST", "/v1/legacy/orders/search", "legacy-api").UpstreamPath("/orders?debug=1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := NewGatewayPublication(GatewayPublicationSpec{
				Service: "legacy",
				Routes:  []GatewayRouteSpec{tt.spec},
			})
			if err == nil {
				t.Fatal("expected gateway publication error")
			}
		})
	}
}

func TestNewGatewayPublicationWSProxyRejectsInvalidSpec(t *testing.T) {
	tests := []struct {
		name string
		spec GatewayRouteSpec
	}{
		{
			name: "missing id",
			spec: WSProxy("", "/v1/legacy/events/stream", "legacy-api"),
		},
		{
			name: "missing service",
			spec: WSProxy("legacy.events_stream", "/v1/legacy/events/stream", ""),
		},
		{
			name: "binding",
			spec: WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api").Body("*"),
		},
		{
			name: "raw response",
			spec: WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api").RawResponse("text/plain"),
		},
		{
			name: "invalid method",
			spec: func() GatewayRouteSpec {
				route := WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api")
				route.HTTPMethod = http.MethodPost
				return route
			}(),
		},
		{
			name: "url upstream path",
			spec: WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api").UpstreamWSPath("http://legacy/events"),
		},
		{
			name: "query upstream path",
			spec: WSProxy("legacy.events_stream", "/v1/legacy/events/stream", "legacy-api").UpstreamWSPath("/events?debug=1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := NewGatewayPublication(GatewayPublicationSpec{
				Service: "legacy",
				Routes:  []GatewayRouteSpec{tt.spec},
			})
			if err == nil {
				t.Fatal("expected gateway publication error")
			}
		})
	}
}

func TestNewGatewayPublicationRejectsDuplicateHTTPRoute(t *testing.T) {
	_, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "user",
		File:    testUserFile(t),
		Routes: []GatewayRouteSpec{
			GET("GetUser", "/v1/users/{id}"),
			HTTPProxy("user.legacy_get", "GET", "/v1/users/{id}", "legacy-user"),
		},
	})
	if err == nil {
		t.Fatal("expected duplicate route error")
	}
}

func TestNewGatewayPublicationWSProxyConflictsWithHTTPGetRoute(t *testing.T) {
	_, _, err := NewGatewayPublication(GatewayPublicationSpec{
		Service: "legacy",
		Routes: []GatewayRouteSpec{
			HTTPProxy("legacy.http_get", "GET", "/v1/legacy/events/stream", "legacy-api"),
			WSProxy("legacy.ws_get", "/v1/legacy/events/stream", "legacy-api"),
		},
	})
	if err == nil {
		t.Fatal("expected duplicate route error")
	}
}

func TestNewGatewayPublicationRejectsAnyHTTPRouteConflict(t *testing.T) {
	tests := []struct {
		name   string
		routes []GatewayRouteSpec
	}{
		{
			name: "any before concrete",
			routes: []GatewayRouteSpec{
				HTTPProxy("legacy.any", "ANY", "/v1/legacy/orders", "legacy-api"),
				HTTPProxy("legacy.get", "GET", "/v1/legacy/orders", "legacy-api"),
			},
		},
		{
			name: "concrete before any",
			routes: []GatewayRouteSpec{
				HTTPProxy("legacy.post", "POST", "/v1/legacy/orders", "legacy-api"),
				HTTPProxy("legacy.any", "ANY", "/v1/legacy/orders", "legacy-api"),
			},
		},
		{
			name: "star conflicts as any",
			routes: []GatewayRouteSpec{
				HTTPProxy("legacy.get", "GET", "/v1/legacy/orders", "legacy-api"),
				HTTPProxy("legacy.star", "*", "/v1/legacy/orders", "legacy-api"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := NewGatewayPublication(GatewayPublicationSpec{
				Service: "legacy",
				Routes:  tt.routes,
			})
			if err == nil {
				t.Fatal("expected conflict error")
			}
		})
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
