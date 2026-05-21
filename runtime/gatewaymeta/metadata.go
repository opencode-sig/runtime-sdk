package gatewaymeta

import (
	"fmt"
	"strings"
	"time"
)

type RouteMeta struct {
	ID       string          `json:"id"`
	Enabled  bool            `json:"enabled"`
	HTTP     HTTPMeta        `json:"http"`
	GRPC     GRPCMeta        `json:"grpc"`
	Binding  Binding         `json:"binding"`
	Timeout  string          `json:"timeout,omitempty"`
	Auth     *AuthPolicy     `json:"auth,omitempty"`
	Response *ResponsePolicy `json:"response,omitempty"`
}

type HTTPMeta struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type GRPCMeta struct {
	Service      string `json:"service"`
	FullMethod   string `json:"full_method"`
	RequestType  string `json:"request_type"`
	ResponseType string `json:"response_type"`
	DescriptorID string `json:"descriptor_id"`
}

type Binding struct {
	Path  map[string]string `json:"path,omitempty"`
	Query map[string]string `json:"query,omitempty"`
	Body  string            `json:"body,omitempty"`
}

type ResponsePolicy struct {
	Raw *RawResponsePolicy `json:"raw,omitempty"`
}

type AuthPolicy struct {
	Public bool `json:"public,omitempty"`
}

type RawResponsePolicy struct {
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body,omitempty"`
	Status      string `json:"status,omitempty"`
	Headers     string `json:"headers,omitempty"`
}

func cloneResponsePolicy(policy *ResponsePolicy) *ResponsePolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	if policy.Raw != nil {
		raw := *policy.Raw
		clone.Raw = &raw
	}
	return &clone
}

func cloneAuthPolicy(policy *AuthPolicy) *AuthPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	return &clone
}

// Validate checks whether a Gateway dynamic route has complete HTTP and gRPC mapping metadata.
//
// Gateway validates etcd metadata and monolith local metadata before serving
// traffic so descriptor id, request type, and FullMethod mistakes fail during
// startup or reload instead of during a later generic gRPC invocation.
func (r RouteMeta) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("route id is required")
	}
	if strings.TrimSpace(r.HTTP.Method) == "" {
		return fmt.Errorf("route %s http method is required", r.ID)
	}
	if strings.TrimSpace(r.HTTP.Path) == "" || !strings.HasPrefix(r.HTTP.Path, "/") {
		return fmt.Errorf("route %s http path must start with /", r.ID)
	}
	if strings.TrimSpace(r.GRPC.Service) == "" {
		return fmt.Errorf("route %s grpc service is required", r.ID)
	}
	if strings.TrimSpace(r.GRPC.FullMethod) == "" || !strings.HasPrefix(r.GRPC.FullMethod, "/") {
		return fmt.Errorf("route %s grpc full_method must start with /", r.ID)
	}
	if strings.TrimSpace(r.GRPC.RequestType) == "" {
		return fmt.Errorf("route %s grpc request_type is required", r.ID)
	}
	if strings.TrimSpace(r.GRPC.ResponseType) == "" {
		return fmt.Errorf("route %s grpc response_type is required", r.ID)
	}
	if strings.TrimSpace(r.GRPC.DescriptorID) == "" {
		return fmt.Errorf("route %s grpc descriptor_id is required", r.ID)
	}
	if r.Timeout != "" {
		if _, err := r.TimeoutDuration(0); err != nil {
			return err
		}
	}
	if r.Response != nil && r.Response.Raw != nil {
		if strings.TrimSpace(r.Response.Raw.Body) == "" {
			return fmt.Errorf("route %s raw response body is required", r.ID)
		}
	}
	return nil
}

// TimeoutDuration returns the upstream call timeout for this route.
//
// Empty RouteMeta.Timeout uses the Gateway fallback. If the fallback is also
// empty, the SDK default is 3s. Centralized parsing keeps all dynamic routes on
// one timeout validation rule.
func (r RouteMeta) TimeoutDuration(fallback time.Duration) (time.Duration, error) {
	if fallback <= 0 {
		fallback = 3 * time.Second
	}
	if strings.TrimSpace(r.Timeout) == "" {
		return fallback, nil
	}
	timeout, err := time.ParseDuration(r.Timeout)
	if err != nil {
		return 0, fmt.Errorf("route %s timeout is invalid: %w", r.ID, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("route %s timeout must be positive", r.ID)
	}
	return timeout, nil
}

// NormalizedMethod returns the HTTP method in uppercase form.
//
// Route indexes should use the normalized method to avoid case-sensitive misses.
func (h HTTPMeta) NormalizedMethod() string {
	return strings.ToUpper(strings.TrimSpace(h.Method))
}
