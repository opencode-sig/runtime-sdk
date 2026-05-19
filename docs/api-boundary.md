# API Boundary

`runtime-sdk` is a public runtime SDK. It must remain reusable by services that
do not know anything about a specific application repository.

## Public Packages

- `servicekit`: managed gRPC service entrypoint and service lifecycle
  contract.
- `runtime/component`: generic lifecycle components for HTTP, gRPC, close hooks,
  and registry registration.
- `runtime/config`: configuration store contracts and etcd-backed store.
- `runtime/control`: rebuild and restart command contracts.
- `runtime/discovery`, `runtime/registry`, `runtime/grpcclient`: service lookup,
  registration, and gRPC client connection management.
- `runtime/gatewaymeta`: Gateway route and descriptor metadata contracts,
  including standard `/{service}` public route prefix publication.
- `observability`: generic health, metrics, and tracing helpers.
- `infra`: optional constructors and configuration objects for common
  infrastructure clients.
- `logger`, `rpcerror`, `apperror`: independent cross-cutting utilities.

Packages not listed here are implementation details unless explicitly promoted
in a release note.

## Boundary Rules

- SDK code must not import external `internal` packages.
- SDK code must not hardcode application names, service names, local config
  paths, or project prefixes.
- `servicekit` must not depend on protobuf generated packages, Gin, Gateway
  response envelopes, or optional infra implementations.
- `runtime/*` packages must not depend on the top-level `servicekit` facade.
- core runtime packages must not depend on optional MySQL, Redis, or Kafka
  packages.
- `logger`, `rpcerror`, and `apperror` must not depend on runtime or infra
  packages.
- Services declare service-local Gateway paths. `runtime/gatewaymeta` owns the
  public `/{service}` prefix convention so Gateways do not need service-specific
  route rules.

Run `make verify` before release. It executes formatting checks, tests,
`go vet`, and boundary checks.
