package gatewaymeta

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type DescriptorRouteSpec struct {
	ID         string
	Enabled    bool
	HTTPMethod string
	HTTPPath   string
	Service    string
	File       protoreflect.FileDescriptor
	Method     string
	Binding    Binding
	Timeout    string
	Response   *ResponsePolicy
}

// NewDescriptorRoute completes Gateway RouteMeta gRPC fields from protobuf descriptors.
//
// Service modules only declare HTTP method/path, RPC method, and parameter
// bindings. FullMethod, request type, response type, and descriptor id are
// inferred from the protobuf descriptor.
func NewDescriptorRoute(spec DescriptorRouteSpec) (RouteMeta, error) {
	service, err := serviceDescriptor(spec.File)
	if err != nil {
		return RouteMeta{}, err
	}
	descriptorID, err := DescriptorID(spec.File)
	if err != nil {
		return RouteMeta{}, err
	}
	methodName := strings.TrimSpace(spec.Method)
	if methodName == "" {
		return RouteMeta{}, fmt.Errorf("proto method is required")
	}
	method := service.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return RouteMeta{}, fmt.Errorf("proto method %s not found in service %s", methodName, service.FullName())
	}

	route := RouteMeta{
		ID:      spec.ID,
		Enabled: spec.Enabled,
		HTTP: HTTPMeta{
			Method: spec.HTTPMethod,
			Path:   spec.HTTPPath,
		},
		GRPC: GRPCMeta{
			Service:      spec.Service,
			FullMethod:   fullMethod(string(service.FullName()), string(method.Name())),
			RequestType:  string(method.Input().FullName()),
			ResponseType: string(method.Output().FullName()),
			DescriptorID: descriptorID,
		},
		Binding:  spec.Binding,
		Timeout:  spec.Timeout,
		Response: cloneResponsePolicy(spec.Response),
	}
	if err := route.Validate(); err != nil {
		return RouteMeta{}, err
	}
	return route, nil
}

// serviceDescriptor returns the single service descriptor declared by a proto file.
//
// The SDK convention is one gRPC service per service proto file. Enforcing the
// rule keeps route ownership unambiguous for Gateway metadata generation.
func serviceDescriptor(file protoreflect.FileDescriptor) (protoreflect.ServiceDescriptor, error) {
	if file == nil {
		return nil, fmt.Errorf("proto file descriptor is required")
	}
	services := file.Services()
	if services.Len() != 1 {
		return nil, fmt.Errorf("proto file %s must contain exactly one service, got %d", file.Path(), services.Len())
	}
	return services.Get(0), nil
}

// DescriptorID uses the proto package as the descriptor id.
//
// The id is published in route metadata and used by Gateway descriptor
// registries to find the corresponding FileDescriptorSet. Proto packages are
// stable across processes and deployments and do not depend on local paths.
func DescriptorID(file protoreflect.FileDescriptor) (string, error) {
	if file == nil {
		return "", fmt.Errorf("proto file descriptor is required")
	}
	id := strings.TrimSpace(string(file.Package()))
	if id == "" {
		return "", fmt.Errorf("proto file %s package is required for descriptor id", file.Path())
	}
	return id, nil
}

// fullMethod builds the standard gRPC FullMethod value: /package.Service/Method.
//
// Generic invokers pass this string directly to grpc.ClientConn.Invoke.
func fullMethod(service string, method string) string {
	service = strings.Trim(strings.TrimSpace(service), "/")
	method = strings.Trim(strings.TrimSpace(method), "/")
	if service == "" || method == "" {
		return ""
	}
	return "/" + service + "/" + method
}
