# API Boundary

`runtime-sdk` is a public runtime SDK. It must remain reusable by services that
do not know anything about a specific application repository.

## Public Packages

- `servicekit`: managed gRPC service entrypoint and service lifecycle
  contract, including the convention-based split config loader, compatible
  single-file bootstrap/managed config loader, SDK-defined DataPlane generation
  identifiers, and etcd first-run config seeding.
- `runtime/component`: generic lifecycle components for HTTP, gRPC, close hooks,
  and registry registration.
- `runtime/config`: configuration store contracts and etcd-backed store.
- `runtime/control`: rebuild and restart command contracts.
- `runtime/discovery`, `runtime/registry`, `runtime/grpcclient`: service lookup,
  registration, and gRPC client connection management.
- `runtime/gatewaymeta`: Gateway route and descriptor metadata contracts.
- `security/authn`: authentication request, decision, credential extraction,
  identity context, and identity metadata helpers.
- `security/authn/grpcauth`: gRPC adapter for calling the SDK security Auth
  service through runtime discovery.
- `protobuf/security/v1`: application-neutral Auth gRPC service contract.
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
  MySQL, Redis, Kafka, Elasticsearch, or MinIO/S3 packages.
- `runtime/*` packages must not depend on the top-level `servicekit` facade.
- core runtime packages must not depend on optional MySQL, Redis, Kafka,
  Elasticsearch, or MinIO/S3 packages.
- `logger`, `rpcerror`, and `apperror` must not depend on runtime or infra
  packages.
- Services declare explicit public Gateway paths. `runtime/gatewaymeta`
  normalizes slashes but does not add service-name prefixes. Gateways route by
  metadata, not by inferring a service from URL prefixes.
- Route authentication whitelist is declarative metadata. Services that own a
  public route must mark it with `runtime/gatewaymeta.Public`; Gateway
  implementations should default to authentication for non-public dynamic
  routes when auth is enabled and must not keep a separate path whitelist.
- Gateway implementations should authenticate through `security/authn` and
  `security/authn/grpcauth` instead of importing an application-local Auth
  protobuf contract. Auth rejection should be represented by
  `AuthenticateResponse.allowed=false`; gRPC errors are reserved for transport
  or infrastructure failures.
- `AuthenticateRequest.target_service` and `authn.Request.TargetService` are
  application-neutral delegation hints. The SDK forwards them to the receiving
  standard AuthService but must not maintain downstream AuthService lists or
  enforce delegation policy.
- Business services must not parse credentials or depend on Auth service
  internals. They should read Gateway-issued identity metadata such as
  `x-auth-subject`, `x-tenant-id`, and safe `x-auth-attr-*` values.
- `servicekit` may attach runtime log identity fields such as
  `runtime_service`, `runtime_mode`, and `instance_id` to the logger it passes
  into service initialization. These fields describe the current log producer
  and must not encode application-specific business identity. Operations aimed
  at another instance should use `target_instance_id`.
- DataPlane generation identifiers are runtime identity, not Gateway or
  business logic. Custom DataPlane owners should call `servicekit.NewGeneration`
  and keep their own lifecycle assembly instead of duplicating generation
  rules or forcing themselves through managed gRPC service templates.
- The external serialization contract for `runtime/registry.ServiceInstance`
  uses snake_case JSON/YAML fields such as `started_at`, `last_seen`, and
  `data_plane_generation`. Registry and discovery consumers must not depend on
  Go struct field names as the storage format.
- `runtime/registry.ErrRegistrationExpired` marks a registration whose lease or
  backing record has been lost. Lifecycle components may recreate the
  registration when this error is returned; transient registry connectivity
  errors should not be mapped to it.
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
