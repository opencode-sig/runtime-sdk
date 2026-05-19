package gatewaymeta

import (
	"net/http"
	"testing"
)

func TestNewDescriptorRoute(t *testing.T) {
	route, err := NewDescriptorRoute(DescriptorRouteSpec{
		ID:         "user.get",
		Enabled:    true,
		HTTPMethod: http.MethodGet,
		HTTPPath:   "/v1/users/{id}",
		Service:    "user",
		File:       testUserFile(t),
		Method:     "GetUser",
		Binding: Binding{
			Path: map[string]string{"id": "id"},
		},
		Timeout: "3s",
	})
	if err != nil {
		t.Fatalf("descriptor route: %v", err)
	}
	if route.GRPC.FullMethod != "/api.user.v1.UserService/GetUser" {
		t.Fatalf("full method = %q", route.GRPC.FullMethod)
	}
	if route.GRPC.RequestType != "api.user.v1.GetUserRequest" {
		t.Fatalf("request type = %q", route.GRPC.RequestType)
	}
	if route.GRPC.ResponseType != "api.user.v1.UserResponse" {
		t.Fatalf("response type = %q", route.GRPC.ResponseType)
	}
	if route.GRPC.DescriptorID != "api.user.v1" {
		t.Fatalf("descriptor id = %q", route.GRPC.DescriptorID)
	}
}

func TestNewDescriptorRouteRejectsUnknownMethod(t *testing.T) {
	_, err := NewDescriptorRoute(DescriptorRouteSpec{
		ID:         "user.get",
		Enabled:    true,
		HTTPMethod: http.MethodGet,
		HTTPPath:   "/v1/users/{id}",
		Service:    "user",
		File:       testUserFile(t),
		Method:     "Missing",
	})
	if err == nil {
		t.Fatal("expected descriptor route error")
	}
}
