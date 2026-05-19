# runtime-sdk

[简体中文](README.zh-CN.md)

`github.com/opencode-sig/runtime-sdk` provides shared runtime primitives for
managed Go gRPC microservices.

The SDK owns runtime concerns that should not leak into business code:

- structured logging with context correlation
- application error wrapping and gRPC business errors
- health, metrics, and tracing helpers
- infrastructure client configuration for etcd, MySQL, Redis, and Kafka
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
    LoadConfig: loadConfig,
})
```

`LoadConfig` is owned by the caller. It can load local files, etcd-backed
configuration, or any deployment-specific source, then return a `servicekit.Config`.

### File Config

```go
func loadFileConfig(ctx context.Context, root string, key string) (servicekit.Config, error) {
    provider := runtimeconfig.NewFileProvider(root)
    data, err := provider.Load(ctx, key)
    if err != nil {
        return servicekit.Config{}, err
    }
    return runtimeconfig.Decode[servicekit.Config](data)
}
```

### Etcd Config And Rebuild

Use a small local bootstrap file to decide whether the service should load its
managed config from etcd. When the returned config uses an etcd config source,
`servicekit.Run` starts a control watcher and applies rebuild/restart commands
published through `runtime/control`.

```go
func loadManagedConfig(ctx context.Context, bootstrap servicekit.Config) (servicekit.Config, error) {
    store, ok := bootstrap.EtcdConfigStore()
    if !ok {
        return bootstrap, nil
    }
    defer func() { _ = store.Close() }()

    data, err := store.Load(ctx, bootstrap.Runtime.Config.Key)
    if err != nil {
        return servicekit.Config{}, err
    }
    return runtimeconfig.Decode[servicekit.Config](data)
}
```

For a complete external microservice example, see
[`docs/go-template-service-example.md`](docs/go-template-service-example.md).
The runnable sample lives in
[`examples/go-template-payment`](examples/go-template-payment).

Managed services can also resolve other services by name through
`servicekit.DistributedContext.Clients`:

```go
userClient, err := servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
```

Gateway route specs use explicit public Gateway paths, for example
`/v1/payments/{id}`. `runtime/gatewaymeta` normalizes slashes but does not add a
service-name prefix. Gateways should forward by the published
`RouteMeta.GRPC.Service` and `RouteMeta.GRPC.FullMethod`, not by parsing URL
prefixes.

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
