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
- `runtime/gatewaymeta`: Gateway route and descriptor metadata contracts.
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
  response envelopes, or concrete platform applications.
- `servicekit` may aggregate public optional-infra config and client facades for
  service onboarding. Core runtime packages must still not depend on optional
  MySQL, Redis, or Kafka packages.
- `runtime/*` packages must not depend on the top-level `servicekit` facade.
- core runtime packages must not depend on optional MySQL, Redis, or Kafka
  packages.
- `logger`, `rpcerror`, and `apperror` must not depend on runtime or infra
  packages.
- Services declare explicit public Gateway paths. `runtime/gatewaymeta`
  normalizes slashes but does not add service-name prefixes. Gateways route by
  metadata, not by inferring a service from URL prefixes.
- Gateway response behavior defaults to the application JSON envelope.
  Browser-renderable or file-like raw output is part of the Gateway route
  contract and must be declared through `runtime/gatewaymeta.RawResponse`.
  Gateway implementations should compile `response.raw` metadata from protobuf
  descriptors and must not infer raw output from method names, URL paths,
  content-type fields, or generic `body` fields.
- Raw response metadata is declarative only. `runtime/gatewaymeta` may describe
  `content_type`, `body`, `status`, and `headers`, but it must not depend on
  Gin, concrete HTTP handlers, or application response-envelope packages.

Run `make verify` before release. It executes formatting checks, tests,
`go vet`, and boundary checks.
