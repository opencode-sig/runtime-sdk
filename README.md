# runtime-sdk

[简体中文](README.zh-CN.md)

`github.com/opencode-sig/runtime-sdk` provides shared runtime primitives for
managed Go gRPC microservices.

The SDK owns runtime concerns that should not leak into business code:

- structured logging with context correlation
- application error wrapping and gRPC business errors
- health, metrics, and tracing helpers
- authentication contract and identity propagation helpers
- infrastructure client configuration for etcd, MySQL, Redis, Kafka,
  Elasticsearch, and MinIO/S3
- lifecycle components
- config, registry, discovery, control command, and Gateway metadata contracts
- `servicekit`, the standard managed gRPC microservice SDK

## Service Entry

A service provides protobuf registration and Gateway metadata. The SDK provides
transport, registry, lifecycle, observability, and control-command rebuilds.

### Quick Start

```go
err := servicekit.Run(ctx, servicekit.RunOptions{
    Spec: servicekit.Spec{
        Name: "payment",
        RegisterGRPC: func(s grpc.ServiceRegistrar) {
            paymentv1.RegisterPaymentServiceServer(s, handler.New())
        },
        GatewayPublication: paymentbootstrap.GatewayPublication,
    },
    LoadConfig: servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
        Root: ".",
    }),
})
```

The convention loader composes a complete `servicekit.Config` from split files:
`configs/runtime.yaml`, `configs/logger.yaml`, `configs/registry.yaml`,
`configs/infra/*.yaml`, and `configs/service/<service>.yaml`. The service file
contains only service-owned settings such as listen addresses and `settings`.

File-mode configs are loaded directly from the local directory. Etcd-mode
configs use local `configs/runtime.yaml` as bootstrap, then read the same
logical keys from the configured config center. If an etcd key is missing and a
matching local file exists, the SDK seeds that key with `PutIfAbsent` and then
loads from etcd. Existing etcd config is never overwritten, and only the current
service's `configs/service/<service>.yaml` is seeded.

When `runtime.config.root` is empty, the SDK fills it with the loader root so
service initialization code can read shared config through
`ctx.Configs.Decode(ctx, "configs/global/app.yaml", &cfg)` with the same logical
keys in file and etcd modes.

Callers can still use `servicekit.NewConfigLoader` for the legacy single-file
complete `servicekit.Config` format, or provide a custom `LoadConfig` for
deployment-specific config sources.

### Etcd Config And Rebuild

Use `config.provider: etcd` in `configs/runtime.yaml` when the service
should load managed config fragments from etcd. The local files are also the
first-run seed source when the corresponding etcd keys are missing.

```go
loader := servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
    Root: ".",
    // RuntimeKey defaults to configs/runtime.yaml.
})
```

By default managed keys live under `configs/`, including
`configs/service/<service>.yaml`. Override `ManagedConfigPrefix` only when the
platform uses a different logical namespace. Set `DisableEtcdAutoSeed` when
service processes should be read-only against the config center.

For a complete external microservice example, see
[`docs/go-template-service-example.md`](docs/go-template-service-example.md).
The runnable sample lives in
[`examples/go-template-payment`](examples/go-template-payment).

Managed services can also resolve other services by name through
`servicekit.DistributedContext.Clients`:

```go
userClient, err := servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
```

### Runtime Logging Identity

`servicekit` injects runtime identity fields into the logger passed to service
initializers. Logs emitted through `ctx.Logger` automatically include:

```text
runtime_service  logical service name from the service spec
runtime_mode     distributed, monolith, or hybrid
instance_id      registry instance id for the current service instance
```

Request-scoped logs should pass the request context to the SDK logger. The
logger adds correlation fields such as `request_id`, `client_ip`, `trace_id`,
and `user_id` from context values or gRPC metadata. `instance_id` is reserved
for the current log producer; use `target_instance_id` when recording an
operation that targets another instance.

Gateway route specs use explicit public Gateway paths, for example
`/v1/payments/{id}`. `runtime/gatewaymeta` normalizes slashes but does not add a
service-name prefix. Gateways should forward by the published
`RouteMeta.GRPC.Service` and `RouteMeta.GRPC.FullMethod`, not by parsing URL
prefixes.

Route-level authentication whitelist behavior is also part of route metadata.
When a Gateway enables authentication, dynamic routes should require
authentication by default. A service must opt out explicitly with `Public()`:

```go
gatewaymeta.POST("Authenticate", "/v1/auth/authenticate").
    Body("*").
    Public()
```

Gateway implementations should read `RouteMeta.Auth.Public` and must not keep a
separate path-pattern whitelist in Gateway config.

The SDK owns the cross-service authentication contract. A Gateway should extract
credentials through `security/authn`, call the Auth gRPC service through
`security/authn/grpcauth`, and propagate only Gateway-issued identity metadata
to downstream services:

```text
Authorization: Bearer <token> -> credential_type=bearer
Authorization: Basic <value>  -> credential_type=basic
X-API-Key: <api-key>          -> credential_type=api_key
apitoken: <legacy-api-token>  -> credential_type=api_key
```

Auth services implement `protobuf/security/v1.AuthService`. Business services
must not parse credentials or call Auth directly; they should read identity from
metadata such as `x-auth-subject`, `x-tenant-id`, and safe
`x-auth-attr-*` values.

Ordinary Gateway routes use the application's standard JSON response envelope.
Routes that need browser-renderable or file-like output must opt in explicitly:

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
    Body("*").
    RawResponse("text/html; charset=utf-8")
```

`RawResponse` publishes `response.raw` metadata. Gateway implementations should
compile that metadata against the protobuf response descriptor and then write
the configured `body` field directly to the HTTP response. Optional `status`
and `headers` protobuf fields can be exposed through `RawStatus` and
`RawHeaders`. Gateways must not infer raw output from method names or from
ordinary response fields.

Managed gRPC components expose Prometheus metrics on the service admin
`/metrics` endpoint. In addition to legacy `runtime_grpc_*` metrics, the SDK
records common gRPC server metrics such as `grpc_server_started_total`,
`grpc_server_handled_total`, `grpc_server_handling_seconds`, in-flight request
gauges, panic counters, deadline counters, and protobuf message-size
histograms.

## Rebuild Semantics

`servicekit` rebuilds a service by creating a new DataPlane from the latest
config, stopping the old generation, and starting the new one. This keeps the
runtime core simple and predictable for single-process services that reuse the
same gRPC/admin addresses. It is not a zero-downtime in-process handoff.

The control command channel is bootstrap-level configuration. If the etcd
endpoints or command prefix change, restart the process so the watcher can move
to the new channel.

## Runtime Modes

`servicekit` exposes stable runtime mode constants:

- `servicekit.RuntimeModeDistributed`
- `servicekit.RuntimeModeMonolith`

Business services should not branch on these values. They are published as
runtime metadata for operations and discovery consumers.

## Infra Constructors

Infra packages follow one consistent rule:

- `Config.IsZero()` reports whether the component is not configured.
- `Config.Normalize()` applies common defaults and does not return errors.
- `Config.Validate()` accepts zero config and validates non-zero config strictly.
- `New*` constructors create clients or pools but do not force connectivity.
- `Ping` or `Check` methods perform explicit connectivity checks.

This allows a process to start with optional infra disabled or temporarily
unreachable, while a higher-level runtime can decide when to check readiness.

## Boundaries

This SDK must stay application-neutral. It must not import application
`internal` packages, embed project config paths, or hardcode service names.
See [`docs/api-boundary.md`](docs/api-boundary.md) for the package ownership
rules.
Run the release gate before publishing:

```sh
make verify
make race
```

Release checklist details live in [`docs/release.md`](docs/release.md).
