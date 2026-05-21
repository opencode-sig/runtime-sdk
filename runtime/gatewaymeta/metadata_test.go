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

func testRoute(descriptorID string) RouteMeta {
	return RouteMeta{
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
}
