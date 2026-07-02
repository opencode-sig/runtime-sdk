# Connect a gRPC Service to go-template

This guide shows how an external Go gRPC microservice can join a go-template
runtime cluster by using `github.com/opencode-sig/runtime-sdk`.

The goal is to keep the service business code independent from runtime details:

- business code only implements protobuf handlers and application logic
- `servicekit` owns gRPC transport, lifecycle, registry, Gateway metadata, and
  control-command rebuilds
- go-template Gateway discovers the service through registry and metadata
  entries, without adding Gateway code for each new service

The example service in this guide is `payment`.

## Runtime Flow

When the service starts:

1. The process loads `servicekit.Config` from a local file, etcd, or another
   bootstrap source chosen by the service.
2. `servicekit.Run` starts a managed DataPlane.
3. The DataPlane starts the gRPC server and service HTTP listener.
4. If etcd registry is enabled, the service registers its advertised gRPC
   address.
5. If Gateway metadata is configured, the service publishes route metadata and
   protobuf descriptors.
6. If etcd config and control command are configured, the service listens for
   rebuild commands and rebuilds its DataPlane with the latest config.

go-template Gateway then forwards HTTP requests to the service through dynamic
routes and service discovery.

## Example Project Layout

```text
payment-service/
  cmd/payment/main.go
  configs/runtime.yaml
  configs/logger.yaml
  configs/registry.yaml
  configs/infra/etcd.yaml
  configs/service/payment.yaml
  internal/bootstrap/gateway.go
  internal/bootstrap/module.go
  internal/handler/handler.go
  internal/service/service.go
  protobuf/payment/v1/payment.proto
  protobuf/payment/v1/payment.pb.go
  protobuf/payment/v1/payment_grpc.pb.go
  go.mod
```

The important boundary is simple:

- `internal/service` contains business logic
- `internal/handler` adapts protobuf requests to business logic
- `internal/bootstrap` is the only package that knows runtime-sdk service
  registration and Gateway publication details
- `cmd/payment` only wires the service spec and config loader into
  `servicekit.Run`

## Module Setup

```sh
go mod init github.com/acme/payment-service
go get github.com/opencode-sig/runtime-sdk
go get google.golang.org/grpc
go get google.golang.org/protobuf
```

## Protobuf Contract

`protobuf/payment/v1/payment.proto`

```proto
syntax = "proto3";

package payment.v1;

option go_package = "github.com/acme/payment-service/protobuf/payment/v1;paymentv1";

service PaymentService {
  rpc GetPayment(GetPaymentRequest) returns (PaymentResponse);
  rpc CreatePayment(CreatePaymentRequest) returns (PaymentResponse);
}

message GetPaymentRequest {
  string id = 1;
}

message CreatePaymentRequest {
  string order_id = 1;
  int64 amount = 2;
  string currency = 3;
}

message PaymentResponse {
  string id = 1;
  string order_id = 2;
  int64 amount = 3;
  string currency = 4;
  string status = 5;
}
```

After generating Go code, the generated package should expose:

- `paymentv1.RegisterPaymentServiceServer`
- `paymentv1.File_payment_v1_payment_proto`
- the request and response message types

## Business Service

`internal/service/service.go`

```go
package service

import (
	"context"
	"fmt"
)

type Payment struct {
	ID       string
	OrderID  string
	Amount   int64
	Currency string
	Status   string
}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GetPayment(ctx context.Context, id string) (Payment, error) {
	if id == "" {
		return Payment{}, fmt.Errorf("payment id is required")
	}
	return Payment{
		ID:       id,
		OrderID:  "order-1001",
		Amount:   9900,
		Currency: "CNY",
		Status:   "paid",
	}, nil
}

func (s *Service) CreatePayment(ctx context.Context, orderID string, amount int64, currency string) (Payment, error) {
	if orderID == "" {
		return Payment{}, fmt.Errorf("order id is required")
	}
	if amount <= 0 {
		return Payment{}, fmt.Errorf("amount must be positive")
	}
	if currency == "" {
		currency = "CNY"
	}
	return Payment{
		ID:       "pay-1001",
		OrderID:  orderID,
		Amount:   amount,
		Currency: currency,
		Status:   "created",
	}, nil
}
```

## gRPC Handler

`internal/handler/handler.go`

```go
package handler

import (
	"context"

	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
	"github.com/acme/payment-service/internal/service"
)

type Handler struct {
	paymentv1.UnimplementedPaymentServiceServer

	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := h.service.GetPayment(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toResponse(payment), nil
}

func (h *Handler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := h.service.CreatePayment(ctx, req.GetOrderId(), req.GetAmount(), req.GetCurrency())
	if err != nil {
		return nil, err
	}
	return toResponse(payment), nil
}

func toResponse(payment service.Payment) *paymentv1.PaymentResponse {
	return &paymentv1.PaymentResponse{
		Id:       payment.ID,
		OrderId:  payment.OrderID,
		Amount:   payment.Amount,
		Currency: payment.Currency,
		Status:   payment.Status,
	}
}
```

## Gateway Publication

`internal/bootstrap/gateway.go`

```go
package bootstrap

import (
	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

const ServiceName = "payment"

func GatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	return gatewaymeta.NewGatewayPublication(gatewaymeta.GatewayPublicationSpec{
		Service: ServiceName,
		File:    paymentv1.File_payment_v1_payment_proto,
		Routes: []gatewaymeta.GatewayRouteSpec{
			gatewaymeta.GET("GetPayment", "/v1/payments/{id}").
				Path("id", "id").
				Timeout("3s"),
			gatewaymeta.POST("CreatePayment", "/v1/payments").
				Body("*").
				Timeout("3s"),
		},
	})
}
```

This publishes:

- HTTP `GET /v1/payments/{id}` to gRPC `PaymentService/GetPayment`
- HTTP `POST /v1/payments` to gRPC `PaymentService/CreatePayment`
- the protobuf descriptor set required by dynamic Gateway invocation

Service code declares the explicit public Gateway path such as
`/v1/payments/{id}`. The SDK normalizes slashes but does not add a service-name
prefix. If an application wants a prefix such as `/payment`, declare it
explicitly in the route path.

If Gateway authentication is enabled, dynamic routes are authenticated by
default. Routes that intentionally belong to the authentication whitelist must
opt out in the service-owned route declaration:

```go
gatewaymeta.POST("Authenticate", "/v1/auth/authenticate").
	Body("*").
	Public()
```

This publishes `auth.public=true` on the route metadata. Keep whitelist
ownership with the service route contract rather than a separate Gateway config
path list.

Ordinary routes use the Gateway JSON response envelope. If a service needs to
return HTML, CSV, PDF, plain text, or another browser/file-like response, declare
the route as raw output:

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
	Body("*").
	RawResponse("text/html; charset=utf-8")
```

The response message can use the default raw field names:

```proto
message RenderHTMLResponse {
  string body = 1;
  string content_type = 2;
  int32 status = 3;
  map<string, string> headers = 4;
}
```

`RawResponse` writes the configured `body` field directly to the HTTP response
instead of wrapping it in the JSON envelope. `content_type` in route metadata has
priority over the response message field. `status` defaults to `200`, and
`headers` is optional. If a service uses different response field names, override
them explicitly:

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
	Body("*").
	RawResponse("text/html; charset=utf-8").
	RawBody("html").
	RawStatus("http_status").
	RawHeaders("response_headers")
```

Gateway implementations should compile `response.raw` metadata against protobuf
descriptors when routes are loaded. They must not infer raw output from method
names, URL paths, or the mere presence of `body` / `content_type` fields.

### HTTP Backend Proxy

Services can also publish HTTP backend routes for business HTTP handlers that
run on the service HTTP listener:

```go
gatewaymeta.HTTPProxy("payment.http_report", "GET", "/v1/payments/http/report", ServiceName).
	UpstreamPath("/internal/payments/report").
	Timeout("3s")
```

The public path is the Gateway path. `UpstreamPath` is the path on the selected
service instance. Gateway implementations should resolve `backend.http.service`
through registry and proxy to the instance metadata `advertise_http_addr`.

If the HTTP backend is an explicit Server-Sent Events stream, declare it
directly in route metadata:

```go
gatewaymeta.HTTPProxy("payment.events_sse", "GET", "/v1/payments/events", ServiceName).
	UpstreamPath("/internal/payments/events").
	SSE().
	Timeout("30s")
```

`SSE()` is only valid on HTTP backend routes, currently requires `GET`, and
still does not use protobuf bindings or raw response metadata.

If the upstream is a WebSocket service, publish an explicit WebSocket proxy
route:

```go
gatewaymeta.WSProxy("payment.events_ws", "/v1/payments/ws", ServiceName).
	UpstreamWSPath("/internal/payments/ws").
	Timeout("30s")
```

WebSocket proxy routes stay declarative: no protobuf descriptor, no protobuf
binding, and no raw response policy.

The service registers the upstream handler with standard library `net/http`:

```go
func RegisterHTTP(mux *http.ServeMux, paymentService *service.Service) {
	mux.HandleFunc("/internal/payments/report", func(w http.ResponseWriter, r *http.Request) {
		// Return a normal raw HTTP response. The Gateway passes status,
		// headers, and body through instead of applying the JSON envelope.
	})
}
```

Runtime reserves `/healthz`, `/readyz`, `/metrics`, and optional
`/debug/pprof/*` on the same listener. `/healthz` is local liveness and does
not fail just because etcd, registry, or Gateway metadata publication is
degraded. Business-critical dependencies can be declared as readiness checks
for `/readyz`. Business handlers should use their own internal paths and must
still be exposed through explicit Gateway metadata.

## Service Spec

`internal/bootstrap/module.go`

```go
package bootstrap

import (
	"net/http"

	"github.com/acme/payment-service/internal/handler"
	"github.com/acme/payment-service/internal/service"
	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func Module() (servicekit.Spec, error) {
	paymentService := service.New()
	paymentHandler := handler.New(paymentService)

	return servicekit.NewGRPCSpec(servicekit.GRPCSpec[paymentv1.PaymentServiceServer]{
		Name:               ServiceName,
		Server:             paymentHandler,
		Register:           paymentv1.RegisterPaymentServiceServer,
		RegisterHTTP: func(mux *http.ServeMux) {
			handler.RegisterHTTP(mux, paymentService)
		},
		GatewayPublication: GatewayPublication,
	})
}
```

For a real service, this is also the right place to assemble repositories,
infra clients, domain services, and handlers. Keep those dependencies injected
into business code instead of reading runtime globals from handlers.

## Calling Other Services

In distributed mode, `servicekit.DistributedContext` exposes `Clients`, a
service-name based gRPC client manager. It uses the same registry/discovery
contract as Gateway, so external services do not need to wire etcd discovery or
gRPC resolver details themselves.

```go
InitDistributed: func(ctx servicekit.DistributedContext) error {
	userClient, err := servicekit.Client(ctx.Clients, context.Background(), "user", userv1.NewUserServiceClient)
	if err != nil {
		return err
	}
	paymentService.SetUserClient(userClient)
	return nil
}
```

Prefer creating typed clients lazily inside the dependency that needs them when
startup should not depend on every downstream service being available.

## Local File Config

The convention loader uses the same split config layout as go-template. Common
runtime settings live in shared files, while `configs/service/payment.yaml`
only contains the payment service fragment.

`configs/runtime.yaml`

```yaml
config:
  provider: file
  key: configs/runtime.yaml
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/config
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
```

`configs/logger.yaml`

```yaml
service_name: payment
file_prefix: payment
level: info
stacktrace_level: error
format: json
enable_stdout: true
enable_file: true
caller: true
```

`configs/registry.yaml`

```yaml
provider: etcd
etcd:
  prefix: /runtime/registry
```

When `configs/registry.yaml` omits endpoints, the convention loader inherits
them from `config.etcd.endpoints` in `configs/runtime.yaml`.
`configs/infra/etcd.yaml` is only for business code that requests
`ctx.Infra.Etcd()`; it does not select the registry or config-center cluster.

`configs/infra/etcd.yaml`

```yaml
endpoints:
  - 127.0.0.1:2379
dial_timeout: 3s
```

`configs/service/payment.yaml`

```yaml
grpc_addr: :9003
advertise_grpc_addr: 127.0.0.1:9003
http_addr: :9103
advertise_http_addr: 127.0.0.1:9103
enable_pprof: true
settings:
  payment_provider: sandbox
```

This file-mode config is useful for local development. It starts the service,
registers it, and publishes Gateway metadata. It does not enable runtime-admin
control-command rebuilds because `config.provider` in `configs/runtime.yaml` is
`file`.

Address rules:

- `grpc_addr` is the local listen address
- `advertise_grpc_addr` is the address registered for other services and
  Gateway discovery
- `http_addr` is the local service HTTP listener address for runtime endpoints
  such as `/healthz`, `/readyz`, `/metrics`, and optional pprof
- `advertise_http_addr` is the HTTP address published for Gateway HTTP backend
  proxying and management tools

If an advertised address is empty and the corresponding listen address is a
wildcard such as `:9003`, `0.0.0.0:9003`, or `[::]:9003`, `servicekit` resolves a
usable runtime-local IP and publishes `IP:port`. This lets containerized
services register their current container IP without hard-coding it. If the
service runs across machines or networks with DNS-based routing, an explicit
advertised address such as `payment:9003` or `10.0.1.8:9003` still takes
priority and should be reachable by other processes.

`grpc_addr` and `http_addr` may use port `0` to let the operating system choose
an available listen port. When the matching advertised address is empty,
`servicekit` uses the listener's actual bound port for registry addresses or
Gateway HTTP upstream metadata, but it does not write that dynamic port back to
the `advertise_grpc_addr` / `advertise_http_addr` config fields.

Multi-homed hosts can set `advertise_ip_cidrs` to constrain the advertised IP
that `servicekit` selects automatically:

```yaml
advertise_ip_cidrs:
  - 192.168.0.0/16
```

This setting only affects SDK-derived registry addresses and
`advertise_http_addr` metadata. It does not change the gRPC/HTTP listener bind
addresses; listener binding is still controlled by `grpc_addr` and `http_addr`.
Explicit `advertise_grpc_addr` / `advertise_http_addr` values continue to take
priority and are not constrained by `advertise_ip_cidrs`.

## Config Loader

`cmd/payment/main.go`

```go
package main

import (
	"context"
	"flag"

	"github.com/acme/payment-service/internal/bootstrap"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func main() {
	configRoot := flag.String("config-root", ".", "project root that contains configs")
	flag.Parse()

	ctx := context.Background()
	spec, err := bootstrap.Module()
	if err != nil {
		panic(err)
	}

	err = servicekit.Run(ctx, servicekit.RunOptions{
		Spec: spec,
		LoadConfig: servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
			Root: *configRoot,
		}),
	})
	if err != nil {
		panic(err)
	}
}
```

The convention loader reads `configs/runtime.yaml` as bootstrap, then composes a
complete `servicekit.Config` from the fixed split keys. `configs/runtime.yaml`
and `configs/service/<service>.yaml` are required. Logger, registry, and infra
fragments are optional; missing optional files keep defaults or empty config.

Use `servicekit.NewConfigLoader` only when you intentionally keep a single
complete `servicekit.Config` file for backward compatibility.

## Etcd Managed Config

For production-managed runtime, switch `configs/runtime.yaml` to etcd mode:

```yaml
config:
  provider: etcd
  key: configs/runtime.yaml
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/config
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
```

In etcd mode, `servicekit.Run` uses the returned config for the DataPlane and
calls `LoadConfig` again whenever a runtime-admin rebuild command is received.
The managed `configs/runtime.yaml` value should keep `config.provider: etcd`
when the process should keep watching runtime-admin commands.

The loader reads these logical keys from etcd:

```text
configs/runtime.yaml
configs/logger.yaml
configs/registry.yaml
configs/infra/etcd.yaml
configs/infra/mysql.yaml
configs/infra/redis.yaml
configs/infra/kafka.yaml
configs/infra/elastic.yaml
configs/infra/minio.yaml
configs/service/payment.yaml
```

`configs/infra/mysql.yaml` contains named MySQL instances. `default` is used by
`ctx.Infra.MySQL()`, and explicit names are selected with
`ctx.Infra.MySQL("report")`:

```yaml
default:
  write_dsns:
    - user:pass@tcp(127.0.0.1:3306)/payment?parseTime=true

report:
  write_dsns:
    - user:pass@tcp(127.0.0.1:3306)/payment_report?parseTime=true
```

When a key is missing and the same local file exists, the SDK writes it with
`PutIfAbsent`. Existing etcd values are never overwritten. The loader only seeds
the current service file, so `configs/service/user.yaml` is not uploaded by the
payment process. Set `DisableEtcdAutoSeed` when service processes should only
read config center values.

## Service Private Settings

`servicekit.Config.Settings` is reserved for service-specific configuration.
Keep common runtime fields in the standard config and decode private settings
inside the service bootstrap layer.

```go
type PaymentSettings struct {
	PaymentProvider string `json:"payment_provider" yaml:"payment_provider"`
}

func decodePaymentSettings(cfg servicekit.Config) (PaymentSettings, error) {
	return servicekit.DecodeSettings[PaymentSettings](cfg)
}
```

This keeps the SDK stable while allowing each service to define its own private
configuration.

## Runtime Admin Rebuild

When `config.provider` in `configs/runtime.yaml` is `etcd` and
`control.commands_prefix` is configured, the service listens for control
commands. The composed `servicekit.Config` stores those values under
`runtime.config` and `runtime.control`. A rebuild command causes the process to:

1. call `LoadConfig` again
2. build a new DataPlane
3. start the new DataPlane
4. stop the old DataPlane gracefully

The business service does not need to watch etcd or restart itself manually.
That behavior belongs to `servicekit`.

When a control command includes `instance_id`, it is matched against the
current runtime registry instance id. If `grpc_addr` uses port `0`, that
instance id is based on the listener's actual bound port; after a rebuild, a new
random port may produce a new instance id. Runtime-admin tools should query the
registry for the current instance before sending targeted commands.

## How go-template Gateway Finds the Service

The external service joins go-template through three shared contracts:

- registry instance under the configured registry prefix
- route metadata under `metadata.routes_prefix`
- protobuf descriptor bytes under `metadata.descriptors_prefix`

Once those entries are available, go-template Gateway can discover the upstream
gRPC address and invoke the service dynamically. No Gateway business code is
required for the new service.

## Startup Checklist

1. Generate protobuf Go files.
2. Implement business service and gRPC handler.
3. Create `GatewayPublication`.
4. Create `servicekit.Spec`.
5. Provide a `servicekit.Config` loader.
6. Start the process with `servicekit.Run`.
7. Confirm the service registered into etcd.
8. Confirm Gateway route metadata and descriptor metadata were published.
9. Call the HTTP route through go-template Gateway.
10. Send a rebuild command through runtime-admin and confirm the service
    rebuilds with the latest config.

## Production Notes

- Keep Gateway metadata close to the service module so route ownership stays
  with the service team.
- Do not put database, Redis, Kafka, search, object storage, or business logic
  inside Gateway.
- Do not make business logic branch on monolith or distributed mode.
- Use advertised addresses for cross-process discovery.
- Treat local config as bootstrap input and etcd config as the managed runtime
  source when running in a centrally managed environment.
- Keep service private config under `settings` to avoid changing SDK contracts
  for every service-specific option.
