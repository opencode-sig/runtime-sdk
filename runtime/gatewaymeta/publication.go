package gatewaymeta

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type GatewayPublicationSpec struct {
	Service string
	File    protoreflect.FileDescriptor
	Routes  []GatewayRouteSpec
}

type GatewayRouteSpec struct {
	ID            string
	Method        string
	HTTPMethod    string
	HTTPPath      string
	Binding       Binding
	Backend       *BackendMeta
	TimeoutString string
	Auth          *AuthPolicy
	Response      *ResponsePolicy
	Disabled      bool
}

// GET creates an HTTP GET route declaration for one gRPC method.
//
// Service modules use this helper to declare exposed HTTP entrypoints without
// hand-writing full RouteMeta values.
func GET(method string, path string) GatewayRouteSpec {
	return HTTP(http.MethodGet, method, path)
}

// POST creates an HTTP POST route declaration for one gRPC method.
func POST(method string, path string) GatewayRouteSpec {
	return HTTP(http.MethodPost, method, path)
}

// HTTP creates a GatewayRouteSpec for any HTTP method.
//
// It stores only the information the service explicitly knows. Descriptor id,
// FullMethod, and request/response types are inferred from proto descriptors.
func HTTP(httpMethod string, method string, path string) GatewayRouteSpec {
	return GatewayRouteSpec{
		Method:     method,
		HTTPMethod: normalizeHTTPMethod(httpMethod),
		HTTPPath:   path,
	}
}

// HTTPProxy creates an HTTP reverse proxy route declaration.
//
// HTTP proxy routes do not use protobuf descriptors or protobuf bindings.
// Gateways resolve service instances through registry metadata and use the
// selected instance's advertise_http_addr as the upstream address.
func HTTPProxy(id string, method string, path string, service string) GatewayRouteSpec {
	return GatewayRouteSpec{
		ID:         strings.TrimSpace(id),
		HTTPMethod: normalizeHTTPMethod(method),
		HTTPPath:   path,
		Backend: &BackendMeta{
			Type: BackendTypeHTTP,
			HTTP: &HTTPBackendMeta{
				Service: strings.TrimSpace(service),
			},
		},
	}
}

// WSProxy creates a WebSocket reverse proxy route declaration.
//
// WebSocket proxy routes keep Gateway in a pure proxy role while making the
// upgraded protocol explicit in route metadata and validation.
func WSProxy(id string, path string, service string) GatewayRouteSpec {
	return GatewayRouteSpec{
		ID:         strings.TrimSpace(id),
		HTTPMethod: http.MethodGet,
		HTTPPath:   path,
		Backend: &BackendMeta{
			Type: BackendTypeWebSocket,
			WebSocket: &WebSocketBackendMeta{
				Service: strings.TrimSpace(service),
			},
		},
	}
}

// UpstreamPath sets the HTTP backend path.
//
// Empty means the Gateway should use the matched public route path.
func (r GatewayRouteSpec) UpstreamPath(path string) GatewayRouteSpec {
	if r.Backend == nil {
		r.Backend = &BackendMeta{Type: BackendTypeHTTP}
	}
	if r.Backend.HTTP == nil {
		r.Backend.HTTP = &HTTPBackendMeta{}
	}
	r.Backend.HTTP.Path = normalizeHTTPBackendPath(path)
	return r
}

// SSE marks this HTTP backend route as an explicit Server-Sent Events stream.
//
// SSE stays within the HTTP backend contract instead of introducing a separate
// transport type. The route must still satisfy HTTP backend validation rules.
func (r GatewayRouteSpec) SSE() GatewayRouteSpec {
	if r.Backend == nil {
		r.Backend = &BackendMeta{Type: BackendTypeHTTP}
	}
	if r.Backend.HTTP == nil {
		r.Backend.HTTP = &HTTPBackendMeta{}
	}
	r.Backend.HTTP.Stream = &HTTPStreamMeta{Mode: HTTPStreamModeSSE}
	return r
}

// UpstreamWSPath sets the WebSocket backend path.
//
// Empty means the Gateway should use the matched public route path.
func (r GatewayRouteSpec) UpstreamWSPath(path string) GatewayRouteSpec {
	if r.Backend == nil {
		r.Backend = &BackendMeta{Type: BackendTypeWebSocket}
	}
	if r.Backend.WebSocket == nil {
		r.Backend.WebSocket = &WebSocketBackendMeta{}
	}
	r.Backend.WebSocket.Path = normalizeHTTPBackendPath(path)
	return r
}

// Path binds an HTTP path parameter to a protobuf request field.
//
// For example, id in /v1/resources/{id} can be bound to request.id.
func (r GatewayRouteSpec) Path(param string, field string) GatewayRouteSpec {
	if r.Binding.Path == nil {
		r.Binding.Path = make(map[string]string)
	}
	r.Binding.Path[param] = field
	return r
}

// Query binds an HTTP query parameter to a protobuf request field.
func (r GatewayRouteSpec) Query(param string, field string) GatewayRouteSpec {
	if r.Binding.Query == nil {
		r.Binding.Query = make(map[string]string)
	}
	r.Binding.Query[param] = field
	return r
}

// Body sets how the HTTP body maps to the protobuf request.
//
// The common "*" value maps the full JSON body to the request message.
func (r GatewayRouteSpec) Body(value string) GatewayRouteSpec {
	r.Binding.Body = value
	return r
}

// Timeout sets the upstream gRPC timeout for this route.
func (r GatewayRouteSpec) Timeout(value string) GatewayRouteSpec {
	r.TimeoutString = value
	return r
}

// Public marks this route as an authentication whitelist route.
//
// Gateways with authentication enabled skip Authenticator calls for public
// routes. Routes without this policy keep the default authenticated behavior.
func (r GatewayRouteSpec) Public() GatewayRouteSpec {
	if r.Auth == nil {
		r.Auth = &AuthPolicy{}
	}
	r.Auth.Public = true
	return r
}

// RawResponse marks this route as a raw HTTP response route.
//
// Raw routes bypass the Gateway JSON envelope and write the configured body
// field directly to the HTTP response. Ordinary routes should not set this and
// therefore keep the default envelope behavior without extra metadata.
func (r GatewayRouteSpec) RawResponse(contentType string) GatewayRouteSpec {
	r.Response = &ResponsePolicy{Raw: defaultRawResponsePolicy(contentType)}
	return r
}

// RawBody overrides the protobuf response field used as the raw HTTP body.
func (r GatewayRouteSpec) RawBody(field string) GatewayRouteSpec {
	r = r.ensureRawResponse()
	r.Response.Raw.Body = strings.TrimSpace(field)
	return r
}

// RawStatus overrides the optional protobuf response field used as HTTP status.
func (r GatewayRouteSpec) RawStatus(field string) GatewayRouteSpec {
	r = r.ensureRawResponse()
	r.Response.Raw.Status = strings.TrimSpace(field)
	return r
}

// RawHeaders overrides the optional protobuf response map field used as HTTP headers.
func (r GatewayRouteSpec) RawHeaders(field string) GatewayRouteSpec {
	r = r.ensureRawResponse()
	r.Response.Raw.Headers = strings.TrimSpace(field)
	return r
}

func (r GatewayRouteSpec) ensureRawResponse() GatewayRouteSpec {
	if r.Response == nil {
		r.Response = &ResponsePolicy{}
	}
	if r.Response.Raw == nil {
		r.Response.Raw = defaultRawResponsePolicy("")
	}
	return r
}

func defaultRawResponsePolicy(contentType string) *RawResponsePolicy {
	return &RawResponsePolicy{
		ContentType: strings.TrimSpace(contentType),
		Body:        "body",
		Status:      "status",
		Headers:     "headers",
	}
}

// NewGatewayPublication builds route metadata and descriptor bytes for Gateway publication.
//
// This is the service-side metadata publication entrypoint. Services declare
// compact route specs, while the runtime infers full gRPC call metadata and
// produces the FileDescriptorSet.
func NewGatewayPublication(spec GatewayPublicationSpec) ([]RouteMeta, map[string][]byte, error) {
	service := strings.Trim(strings.TrimSpace(spec.Service), "/")
	if service == "" {
		return nil, nil, fmt.Errorf("gateway service is required")
	}
	if len(spec.Routes) == 0 {
		return nil, nil, fmt.Errorf("gateway routes are required")
	}

	needsDescriptor := false
	for _, routeSpec := range spec.Routes {
		if !routeSpec.isHTTPBackend() && !routeSpec.isWebSocketBackend() {
			needsDescriptor = true
			break
		}
	}

	descriptors := map[string][]byte{}
	if needsDescriptor {
		if spec.File == nil {
			return nil, nil, fmt.Errorf("gateway proto file descriptor is required")
		}
		descriptorSet, err := GatewayDescriptorSet(spec.File)
		if err != nil {
			return nil, nil, err
		}
		descriptorID, err := DescriptorID(spec.File)
		if err != nil {
			return nil, nil, err
		}
		descriptors[descriptorID] = descriptorSet
	}

	routes := make([]RouteMeta, 0, len(spec.Routes))
	for _, routeSpec := range spec.Routes {
		if routeSpec.isHTTPBackend() {
			route, err := newHTTPBackendRoute(routeSpec)
			if err != nil {
				return nil, nil, err
			}
			routes = append(routes, route)
			continue
		}
		if routeSpec.isWebSocketBackend() {
			route, err := newWebSocketBackendRoute(routeSpec)
			if err != nil {
				return nil, nil, err
			}
			routes = append(routes, route)
			continue
		}

		// Route id defaults to service + RPC method for cross-deployment stability.
		// Services may still set an explicit ID for long-term compatibility.
		routeID := strings.TrimSpace(routeSpec.ID)
		if routeID == "" {
			routeID = defaultGatewayRouteID(service, routeSpec.Method)
		}
		route, err := NewDescriptorRoute(DescriptorRouteSpec{
			ID:         routeID,
			Enabled:    !routeSpec.Disabled,
			HTTPMethod: routeSpec.HTTPMethod,
			HTTPPath:   gatewayRoutePath(routeSpec.HTTPPath),
			Service:    service,
			File:       spec.File,
			Method:     routeSpec.Method,
			Binding:    routeSpec.Binding,
			Timeout:    routeSpec.TimeoutString,
			Auth:       cloneAuthPolicy(routeSpec.Auth),
			Response:   cloneResponsePolicy(routeSpec.Response),
		})
		if err != nil {
			return nil, nil, err
		}
		routes = append(routes, route)
	}

	if err := validatePublishedRoutes(routes); err != nil {
		return nil, nil, err
	}
	return routes, descriptors, nil
}

func validatePublishedRoutes(routes []RouteMeta) error {
	ids := make(map[string]struct{}, len(routes))
	httpRoutes := make(map[string]string, len(routes))
	anyRoutes := make(map[string]string, len(routes))
	concreteRoutes := make(map[string]string, len(routes))
	for _, route := range routes {
		id := strings.TrimSpace(route.ID)
		if _, ok := ids[id]; ok {
			return fmt.Errorf("gateway route id %q is duplicated", id)
		}
		ids[id] = struct{}{}

		method := route.HTTP.NormalizedMethod()
		path := strings.TrimSpace(route.HTTP.Path)
		httpKey := method + " " + path
		if existingID, ok := httpRoutes[httpKey]; ok {
			return fmt.Errorf("gateway route %s conflicts with %s on %s", route.ID, existingID, httpKey)
		}
		if method == HTTPMethodAny {
			if existingID, ok := anyRoutes[path]; ok {
				return fmt.Errorf("gateway route %s conflicts with %s on %s", route.ID, existingID, httpKey)
			}
			if existingID, ok := concreteRoutes[path]; ok {
				return fmt.Errorf("gateway route %s conflicts with %s on %s", route.ID, existingID, httpKey)
			}
			anyRoutes[path] = route.ID
		} else {
			if existingID, ok := anyRoutes[path]; ok {
				return fmt.Errorf("gateway route %s conflicts with %s on %s", route.ID, existingID, httpKey)
			}
			if _, ok := concreteRoutes[path]; !ok {
				concreteRoutes[path] = route.ID
			}
		}
		httpRoutes[httpKey] = route.ID
	}
	return nil
}

func (r GatewayRouteSpec) isHTTPBackend() bool {
	return r.Backend != nil && r.Backend.Type == BackendTypeHTTP
}

func (r GatewayRouteSpec) isWebSocketBackend() bool {
	return r.Backend != nil && r.Backend.Type == BackendTypeWebSocket
}

func newHTTPBackendRoute(spec GatewayRouteSpec) (RouteMeta, error) {
	route := RouteMeta{
		ID:      strings.TrimSpace(spec.ID),
		Enabled: !spec.Disabled,
		HTTP: HTTPMeta{
			Method: spec.HTTPMethod,
			Path:   gatewayRoutePath(spec.HTTPPath),
		},
		Binding:  spec.Binding,
		Backend:  cloneBackendMeta(spec.Backend),
		Timeout:  spec.TimeoutString,
		Auth:     cloneAuthPolicy(spec.Auth),
		Response: cloneResponsePolicy(spec.Response),
	}
	if route.Backend != nil && route.Backend.HTTP != nil {
		route.Backend.HTTP.Path = normalizeHTTPBackendPath(route.Backend.HTTP.Path)
	}
	if err := route.Validate(); err != nil {
		return RouteMeta{}, err
	}
	return route, nil
}

func newWebSocketBackendRoute(spec GatewayRouteSpec) (RouteMeta, error) {
	route := RouteMeta{
		ID:      strings.TrimSpace(spec.ID),
		Enabled: !spec.Disabled,
		HTTP: HTTPMeta{
			Method: spec.HTTPMethod,
			Path:   gatewayRoutePath(spec.HTTPPath),
		},
		Binding:  spec.Binding,
		Backend:  cloneBackendMeta(spec.Backend),
		Timeout:  spec.TimeoutString,
		Auth:     cloneAuthPolicy(spec.Auth),
		Response: cloneResponsePolicy(spec.Response),
	}
	if route.Backend != nil && route.Backend.WebSocket != nil {
		route.Backend.WebSocket.Path = normalizeHTTPBackendPath(route.Backend.WebSocket.Path)
	}
	if err := route.Validate(); err != nil {
		return RouteMeta{}, err
	}
	return route, nil
}

// gatewayRoutePath normalizes an explicit public Gateway path.
//
// Services must declare the full public path they want published. The runtime
// does not add a service-name prefix implicitly.
func gatewayRoutePath(routePath string) string {
	path := "/" + strings.Trim(strings.TrimSpace(routePath), "/")
	return path
}

func normalizeHTTPBackendPath(routePath string) string {
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return ""
	}
	if strings.Contains(routePath, "://") || strings.Contains(routePath, "?") || strings.Contains(routePath, "#") || strings.Contains(routePath, "//") {
		return routePath
	}
	return gatewayRoutePath(routePath)
}

func normalizeHTTPMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "*" {
		return HTTPMethodAny
	}
	return method
}

// defaultGatewayRouteID builds a stable route id from service name and RPC method.
//
// For example, service=resource and method=GetResource becomes resource.get.
func defaultGatewayRouteID(service string, method string) string {
	serviceWords := identifierWords(service)
	methodWords := identifierWords(method)
	actionWords := methodWords
	if len(serviceWords) > 0 && len(methodWords) > len(serviceWords) && hasWordSuffix(methodWords, serviceWords) {
		actionWords = methodWords[:len(methodWords)-len(serviceWords)]
	}
	if len(actionWords) == 0 {
		actionWords = methodWords
	}
	return strings.Join(serviceWords, "_") + "." + strings.Join(actionWords, "_")
}

// hasWordSuffix reports whether words ends with suffix.
//
// It helps strip service-name suffixes from methods such as GetUser or
// CreateOrder so route ids stay short and stable.
func hasWordSuffix(words []string, suffix []string) bool {
	if len(suffix) == 0 || len(words) < len(suffix) {
		return false
	}
	start := len(words) - len(suffix)
	for i, word := range suffix {
		if words[start+i] != word {
			return false
		}
	}
	return true
}

// identifierWords splits service or method names into lowercase words.
//
// It supports snake case, kebab case, spaces, and CamelCase for readable route ids.
func identifierWords(value string) []string {
	runes := []rune(strings.TrimSpace(value))
	words := make([]string, 0, 2)
	current := make([]rune, 0, len(runes))
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			words = appendIdentifierWord(words, current)
			current = current[:0]
			continue
		}
		if len(current) > 0 && startsIdentifierWord(runes, i) {
			words = appendIdentifierWord(words, current)
			current = current[:0]
		}
		current = append(current, unicode.ToLower(r))
	}
	return appendIdentifierWord(words, current)
}

// startsIdentifierWord reports whether a CamelCase position starts a new word.
//
// It treats consecutive uppercase acronyms such as HTTP in GetHTTPUser as one word.
func startsIdentifierWord(runes []rune, index int) bool {
	r := runes[index]
	if !unicode.IsUpper(r) {
		return false
	}
	prev := runes[index-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	if unicode.IsUpper(prev) && index+1 < len(runes) {
		return unicode.IsLower(runes[index+1])
	}
	return false
}

// appendIdentifierWord appends a non-empty rune slice as a word.
func appendIdentifierWord(words []string, word []rune) []string {
	if len(word) == 0 {
		return words
	}
	return append(words, string(word))
}
