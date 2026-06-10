package gatewaymeta

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RouteMeta struct {
	ID       string          `json:"id"`
	Enabled  bool            `json:"enabled"`
	HTTP     HTTPMeta        `json:"http"`
	GRPC     GRPCMeta        `json:"grpc,omitempty"`
	Backend  *BackendMeta    `json:"backend,omitempty"`
	Binding  Binding         `json:"binding"`
	Timeout  string          `json:"timeout,omitempty"`
	Auth     *AuthPolicy     `json:"auth,omitempty"`
	Response *ResponsePolicy `json:"response,omitempty"`
}

type HTTPMeta struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

const HTTPMethodAny = "ANY"

type GRPCMeta struct {
	Service      string `json:"service"`
	FullMethod   string `json:"full_method"`
	RequestType  string `json:"request_type"`
	ResponseType string `json:"response_type"`
	DescriptorID string `json:"descriptor_id"`
}

type BackendType string

const (
	BackendTypeGRPC      BackendType = "grpc"
	BackendTypeHTTP      BackendType = "http"
	BackendTypeWebSocket BackendType = "websocket"
)

type BackendMeta struct {
	Type      BackendType           `json:"type,omitempty"`
	GRPC      *GRPCMeta             `json:"grpc,omitempty"`
	HTTP      *HTTPBackendMeta      `json:"http,omitempty"`
	WebSocket *WebSocketBackendMeta `json:"websocket,omitempty"`
}

const HTTPStreamModeSSE = "sse"

type HTTPBackendMeta struct {
	Service string          `json:"service,omitempty"`
	Path    string          `json:"path,omitempty"`
	Stream  *HTTPStreamMeta `json:"stream,omitempty"`
}

type HTTPStreamMeta struct {
	Mode string `json:"mode,omitempty"`
}

type WebSocketBackendMeta struct {
	Service string `json:"service,omitempty"`
	Path    string `json:"path,omitempty"`
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

func cloneBackendMeta(backend *BackendMeta) *BackendMeta {
	if backend == nil {
		return nil
	}
	clone := *backend
	if backend.GRPC != nil {
		grpc := *backend.GRPC
		clone.GRPC = &grpc
	}
	if backend.HTTP != nil {
		httpBackend := *backend.HTTP
		if backend.HTTP.Stream != nil {
			stream := *backend.HTTP.Stream
			httpBackend.Stream = &stream
		}
		clone.HTTP = &httpBackend
	}
	if backend.WebSocket != nil {
		websocketBackend := *backend.WebSocket
		clone.WebSocket = &websocketBackend
	}
	return &clone
}

// MarshalJSON omits the legacy top-level grpc field for non-gRPC backend routes.
func (r RouteMeta) MarshalJSON() ([]byte, error) {
	type routeMeta RouteMeta
	out := struct {
		routeMeta
		GRPC *GRPCMeta `json:"grpc,omitempty"`
	}{
		routeMeta: routeMeta(r),
	}
	out.routeMeta.GRPC = GRPCMeta{}
	if r.BackendType() == BackendTypeGRPC {
		grpc := r.GRPC
		out.GRPC = &grpc
	}
	return json.Marshal(out)
}

// BackendType returns the explicit backend type, or the legacy gRPC default.
func (r RouteMeta) BackendType() BackendType {
	if r.Backend == nil || strings.TrimSpace(string(r.Backend.Type)) == "" {
		return BackendTypeGRPC
	}
	return r.Backend.Type
}

// Validate checks whether a Gateway dynamic route has complete HTTP and backend mapping metadata.
//
// Gateway validates etcd metadata and monolith local metadata before serving
// traffic so route metadata mistakes fail during startup or reload instead of
// during a later upstream invocation.
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
	if r.Timeout != "" {
		if _, err := r.TimeoutDuration(0); err != nil {
			return err
		}
	}

	switch r.BackendType() {
	case BackendTypeGRPC:
		grpc := r.GRPC
		if r.Backend != nil && r.Backend.GRPC != nil {
			grpc = *r.Backend.GRPC
		}
		if err := validateGRPCMeta(r.ID, grpc); err != nil {
			return err
		}
	case BackendTypeHTTP:
		if err := r.validateHTTPBackend(); err != nil {
			return err
		}
	case BackendTypeWebSocket:
		if err := r.validateWebSocketBackend(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("route %s backend type %q is not supported", r.ID, r.Backend.Type)
	}

	if r.Response != nil && r.Response.Raw != nil {
		if strings.TrimSpace(r.Response.Raw.Body) == "" {
			return fmt.Errorf("route %s raw response body is required", r.ID)
		}
	}
	return nil
}

func validateGRPCMeta(routeID string, grpc GRPCMeta) error {
	if strings.TrimSpace(grpc.Service) == "" {
		return fmt.Errorf("route %s grpc service is required", routeID)
	}
	if strings.TrimSpace(grpc.FullMethod) == "" || !strings.HasPrefix(grpc.FullMethod, "/") {
		return fmt.Errorf("route %s grpc full_method must start with /", routeID)
	}
	if strings.TrimSpace(grpc.RequestType) == "" {
		return fmt.Errorf("route %s grpc request_type is required", routeID)
	}
	if strings.TrimSpace(grpc.ResponseType) == "" {
		return fmt.Errorf("route %s grpc response_type is required", routeID)
	}
	if strings.TrimSpace(grpc.DescriptorID) == "" {
		return fmt.Errorf("route %s grpc descriptor_id is required", routeID)
	}
	return nil
}

func (r RouteMeta) validateHTTPBackend() error {
	if r.Backend == nil || r.Backend.HTTP == nil {
		return fmt.Errorf("route %s http backend is required", r.ID)
	}
	if !isZeroGRPCMeta(r.GRPC) || r.Backend.GRPC != nil {
		return fmt.Errorf("route %s http backend must not set grpc metadata", r.ID)
	}
	if strings.TrimSpace(r.Backend.HTTP.Service) == "" {
		return fmt.Errorf("route %s http backend service is required", r.ID)
	}
	if strings.TrimSpace(r.Backend.HTTP.Path) != "" {
		if err := validateHTTPBackendPath(r.ID, r.Backend.HTTP.Path); err != nil {
			return err
		}
	}
	streamMode := normalizeHTTPStreamMode("")
	if r.Backend.HTTP.Stream != nil {
		streamMode = normalizeHTTPStreamMode(r.Backend.HTTP.Stream.Mode)
		if streamMode == "" {
			return fmt.Errorf("route %s http backend stream mode is required", r.ID)
		}
		if streamMode != HTTPStreamModeSSE {
			return fmt.Errorf("route %s http backend stream mode %q is not supported", r.ID, r.Backend.HTTP.Stream.Mode)
		}
	}
	if streamMode == HTTPStreamModeSSE && r.HTTP.NormalizedMethod() != http.MethodGet {
		return fmt.Errorf("route %s sse http backend method must be GET", r.ID)
	}
	if len(r.Binding.Path) > 0 || len(r.Binding.Query) > 0 || strings.TrimSpace(r.Binding.Body) != "" {
		return fmt.Errorf("route %s http backend must not set protobuf binding", r.ID)
	}
	if r.Response != nil && r.Response.Raw != nil {
		return fmt.Errorf("route %s http backend must not set raw response policy", r.ID)
	}
	return nil
}

func (r RouteMeta) validateWebSocketBackend() error {
	if r.Backend == nil || r.Backend.WebSocket == nil {
		return fmt.Errorf("route %s websocket backend is required", r.ID)
	}
	if !isZeroGRPCMeta(r.GRPC) || r.Backend.GRPC != nil {
		return fmt.Errorf("route %s websocket backend must not set grpc metadata", r.ID)
	}
	if r.HTTP.NormalizedMethod() != http.MethodGet {
		return fmt.Errorf("route %s websocket backend http method must be GET", r.ID)
	}
	if strings.TrimSpace(r.Backend.WebSocket.Service) == "" {
		return fmt.Errorf("route %s websocket backend service is required", r.ID)
	}
	if strings.TrimSpace(r.Backend.WebSocket.Path) != "" {
		if err := validateHTTPBackendPath(r.ID, r.Backend.WebSocket.Path); err != nil {
			return err
		}
	}
	if len(r.Binding.Path) > 0 || len(r.Binding.Query) > 0 || strings.TrimSpace(r.Binding.Body) != "" {
		return fmt.Errorf("route %s websocket backend must not set protobuf binding", r.ID)
	}
	if r.Response != nil && r.Response.Raw != nil {
		return fmt.Errorf("route %s websocket backend must not set raw response policy", r.ID)
	}
	return nil
}

func isZeroGRPCMeta(grpc GRPCMeta) bool {
	return strings.TrimSpace(grpc.Service) == "" &&
		strings.TrimSpace(grpc.FullMethod) == "" &&
		strings.TrimSpace(grpc.RequestType) == "" &&
		strings.TrimSpace(grpc.ResponseType) == "" &&
		strings.TrimSpace(grpc.DescriptorID) == ""
}

func validateHTTPBackendPath(routeID string, path string) error {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("route %s http backend path must start with /", routeID)
	}
	if strings.Contains(path, "://") || strings.Contains(path, "?") || strings.Contains(path, "#") {
		return fmt.Errorf("route %s http backend path must not include scheme, host, query, or fragment", routeID)
	}
	if strings.Contains(path, "//") {
		return fmt.Errorf("route %s http backend path must not contain empty segments", routeID)
	}
	return nil
}

func normalizeHTTPStreamMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
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
	method := strings.ToUpper(strings.TrimSpace(h.Method))
	if method == "*" {
		return HTTPMethodAny
	}
	return method
}
