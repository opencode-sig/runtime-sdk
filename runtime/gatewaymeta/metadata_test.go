package gatewaymeta

import (
	"net/http"
	"testing"
)

func TestRouteMetaValidate(t *testing.T) {
	route := testRoute("api.test.v1")
	if err := route.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRouteMetaValidateRejectsMissingDescriptor(t *testing.T) {
	route := testRoute("")
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsRawResponseWithoutBody(t *testing.T) {
	route := testRoute("api.test.v1")
	route.Response = &ResponsePolicy{Raw: &RawResponsePolicy{ContentType: "text/html"}}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaAllowsPublicAuthPolicy(t *testing.T) {
	route := testRoute("api.test.v1")
	route.Auth = &AuthPolicy{Public: true}
	if err := route.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRouteMetaValidateHTTPBackend(t *testing.T) {
	route := testHTTPBackendRoute()
	if err := route.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRouteMetaValidateHTTPBackendSSE(t *testing.T) {
	route := testHTTPBackendRoute()
	route.HTTP.Method = http.MethodGet
	route.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: HTTPStreamModeSSE}
	if err := route.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRouteMetaValidateWebSocketBackend(t *testing.T) {
	route := testWebSocketBackendRoute()
	if err := route.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRouteMetaValidateRejectsHTTPBackendBinding(t *testing.T) {
	route := testHTTPBackendRoute()
	route.Binding.Body = "*"
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendRawResponse(t *testing.T) {
	route := testHTTPBackendRoute()
	route.Response = &ResponsePolicy{Raw: defaultRawResponsePolicy("text/plain")}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendSSEBinding(t *testing.T) {
	route := testHTTPBackendRoute()
	route.HTTP.Method = http.MethodGet
	route.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: HTTPStreamModeSSE}
	route.Binding.Body = "*"
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendSSERawResponse(t *testing.T) {
	route := testHTTPBackendRoute()
	route.HTTP.Method = http.MethodGet
	route.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: HTTPStreamModeSSE}
	route.Response = &ResponsePolicy{Raw: defaultRawResponsePolicy("text/plain")}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendSSENonGET(t *testing.T) {
	route := testHTTPBackendRoute()
	route.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: HTTPStreamModeSSE}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendInvalidStreamMode(t *testing.T) {
	route := testHTTPBackendRoute()
	route.HTTP.Method = http.MethodGet
	route.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: "streaming"}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsHTTPBackendGRPCMetadata(t *testing.T) {
	route := testHTTPBackendRoute()
	route.GRPC = GRPCMeta{Service: "legacy"}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsWebSocketBackendBinding(t *testing.T) {
	route := testWebSocketBackendRoute()
	route.Binding.Body = "*"
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsWebSocketBackendRawResponse(t *testing.T) {
	route := testWebSocketBackendRoute()
	route.Response = &ResponsePolicy{Raw: defaultRawResponsePolicy("text/plain")}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsWebSocketBackendNonGET(t *testing.T) {
	route := testWebSocketBackendRoute()
	route.HTTP.Method = http.MethodPost
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsWebSocketBackendGRPCMetadata(t *testing.T) {
	route := testWebSocketBackendRoute()
	route.GRPC = GRPCMeta{Service: "legacy"}
	if err := route.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRouteMetaValidateRejectsInvalidHTTPBackendPath(t *testing.T) {
	tests := []string{
		"http://legacy/orders",
		"/orders?debug=1",
		"/orders//search",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			route := testHTTPBackendRoute()
			route.Backend.HTTP.Path = path
			if err := route.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func testRoute(descriptorID string) RouteMeta {
	route := RouteMeta{
		ID:      "test.get",
		Enabled: true,
		HTTP: HTTPMeta{
			Method: http.MethodGet,
			Path:   "/v1/tests/{id}",
		},
		GRPC: GRPCMeta{
			Service:      "test",
			FullMethod:   "/api.test.v1.TestService/GetTest",
			RequestType:  "api.test.v1.GetTestRequest",
			ResponseType: "api.test.v1.TestResponse",
			DescriptorID: descriptorID,
		},
		Binding: Binding{
			Path: map[string]string{"id": "id"},
		},
		Timeout: "3s",
	}
	route.Backend = &BackendMeta{Type: BackendTypeGRPC, GRPC: &route.GRPC}
	return route
}

func testHTTPBackendRoute() RouteMeta {
	return RouteMeta{
		ID:      "legacy.orders_search",
		Enabled: true,
		HTTP: HTTPMeta{
			Method: http.MethodPost,
			Path:   "/v1/legacy/orders/search",
		},
		Backend: &BackendMeta{
			Type: BackendTypeHTTP,
			HTTP: &HTTPBackendMeta{
				Service: "legacy-api",
				Path:    "/orders/search",
			},
		},
		Timeout: "3s",
	}
}

func testWebSocketBackendRoute() RouteMeta {
	return RouteMeta{
		ID:      "legacy.events_stream",
		Enabled: true,
		HTTP: HTTPMeta{
			Method: http.MethodGet,
			Path:   "/v1/legacy/events/stream",
		},
		Backend: &BackendMeta{
			Type: BackendTypeWebSocket,
			WebSocket: &WebSocketBackendMeta{
				Service: "legacy-api",
				Path:    "/events/stream",
			},
		},
		Timeout: "3s",
	}
}
