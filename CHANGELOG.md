# Changelog

## v0.7.6 - 2026-06-17

- Added support for managed services listening on port `0`: when advertised
  addresses are empty, registry addresses and HTTP upstream metadata now use the
  listener's actual bound port without writing it back to config. Targeted
  control commands now match the current runtime registry instance id so
  rebuilds that pick a new random port keep instance targeting consistent.

## v0.7.5 - 2026-06-10

- Added `gatewaymeta.WSProxy` for declarative WebSocket backend proxy routes.
- Added `gatewaymeta.GatewayRouteSpec.SSE()` so HTTP backend proxy routes can
  explicitly publish Server-Sent Events stream metadata and validation rules.

## v0.7.4 - 2026-06-09

- Added automatic advertised address resolution for managed services. When
  `advertise_grpc_addr` is empty and `grpc_addr` listens on `:port`,
  `0.0.0.0:port`, or `[::]:port`, servicekit now registers a usable local
  runtime IP plus the gRPC port instead of the wildcard listen address. The same
  rule now fills registry metadata `advertise_http_addr` from `http_addr` for
  Gateway HTTP backend proxy routes.

## v0.7.3 - 2026-06-04

- Changed Gateway metadata reconcile to skip etcd `Put` calls when descriptor or
  route content is unchanged, reducing unnecessary Gateway watch events.

## v0.7.2 - 2026-06-04

- Increased Gateway metadata reconcile interval to 5 minutes to reduce
  unnecessary etcd writes and Gateway watch churn while keeping periodic
  metadata self-healing.

## v0.7.1 - 2026-06-04

- Increased Gateway metadata reconcile publish timeout to 30 seconds so larger
  descriptor sets and slower etcd writes have enough time to complete.

## v0.7.0 - 2026-06-04

- Split managed service HTTP probes into `/healthz` local liveness and
  `/readyz` readiness. `servicekit.Spec` and `GRPCSpec` now accept optional
  `ReadinessChecks`; etcd, registry, Gateway metadata publication, and control
  watchers no longer fail service liveness by default.
- Added control-plane logs and Prometheus metrics for registry and Gateway
  metadata degradation and recovery:
  `runtime_control_plane_status`, `runtime_control_plane_errors_total`, and
  `runtime_control_plane_recoveries_total`.

## v0.6.0 - 2026-06-02

- Renamed managed service HTTP configuration from `admin_addr` /
  `advertise_admin_addr` to `http_addr` / `advertise_http_addr`. The service
  HTTP listener still serves `/healthz`, `/metrics`, and optional pprof, and
  `advertise_http_addr` is now the registry metadata address used by HTTP
  backend proxy routes.
- Added Gateway HTTP backend route metadata and `gatewaymeta.HTTPProxy` for
  declaring HTTP upstream routes without protobuf descriptors.
- Added `gatewaymeta.HTTPMethodAny` support so HTTP backend routes can publish
  `ANY` method routes; `ANY + path` conflicts with concrete methods on the
  same path.
- Added optional `servicekit.Spec.RegisterHTTP` / `GRPCSpec.RegisterHTTP` so
  services can register business HTTP handlers on the service HTTP listener for
  HTTP backend proxy routes.

## v0.5.4 - 2026-06-02

- Added `AuthenticateRequest.target_service` and
  `authn.Request.TargetService` so callers can ask a standard AuthService to
  delegate authentication to a selected downstream standard AuthService without
  adding runtime SDK config.

## v0.5.3 - 2026-05-30

- Added `servicekit.NewGeneration` so custom DataPlane owners can reuse the
  SDK-defined `data_plane_generation` contract without adopting managed gRPC
  service assembly.

## v0.5.2 - 2026-05-30

- Added resilient etcd registry recovery: managed service registrations now
  renew in the background, re-register automatically when the etcd lease or
  registry key is lost, and ignore missing leases during deregistration.
- Added `runtime/registry.ErrRegistrationExpired` so registry implementations
  can explicitly signal that a service registration must be recreated.
- Added distributed example release gates for normal service discovery smoke
  verification and etcd-backed registry resilience verification.

## v0.5.1 - 2026-05-30

- Added a runnable distributed `payment` + `user` example under
  `examples/go-template-payment`, including a unified `cmd/distributed`
  launcher, a sample client, separate service-owned internal package trees, and
  user protobuf/service wiring for service-name gRPC calls through etcd
  registry/discovery.

## v0.5.0 - 2026-05-30

- Added `servicekit.NewConventionConfigLoader` for go-template-style split
  service config fragments, including etcd `PutIfAbsent` seeding for missing
  convention keys.
- Added registry endpoint inheritance for convention configs: when
  `configs/registry.yaml` uses etcd without endpoints, it inherits
  `configs/runtime.yaml` `config.etcd.endpoints`.
- Fixed `runtime/registry.ServiceInstance` JSON/YAML serialization to use the
  documented snake_case contract, including `data_plane_started_at` and
  `data_plane_generation`.
- Added optional Elasticsearch and MinIO/S3 infra config and client helpers,
  including `servicekit.Infra` lazy client accessors.
- Added `servicekit.NewConfigLoader` and `ManagedConfigLoader` as the standard
  bootstrap/managed config loader for external services, including first-run
  etcd config seeding with `PutIfAbsent`.
- Removed the older `SeedServiceConfig` helper so service config seeding has a
  single supported path through `NewConfigLoader`.
- Added route-level Gateway authentication whitelist metadata through
  `GatewayRouteSpec.Public` and `RouteMeta.Auth.Public`.
- Added explicit Gateway raw response metadata for browser-renderable and
  file-like outputs through `GatewayRouteSpec.RawResponse`, `RawBody`,
  `RawStatus`, and `RawHeaders`.
- Added default gRPC server metrics in `observability/metrics`, including
  started/handled/latency counters, in-flight gauges, panic/deadline counters,
  and protobuf message-size histograms.
- Added `servicekit.Configs` for read-oriented config-center access during
  service initialization, including consistent logical keys for file and etcd
  config sources.
- Added `runtime.config.root` to `servicekit.Config` so file-backed global
  config reads can resolve from the intended config root.
- Added control command TTL support and DataPlane generation metadata in service
  registry instances.
- Updated API boundary rules to match the current `servicekit` facade design:
  `servicekit` may aggregate optional infra config and client facades, while
  core runtime packages remain independent from optional infra.
- Renamed generic runtime observability helpers from `platform` to
  `observability` for clearer public API semantics.
- Split `servicekit` responsibilities across focused files without
  changing public types or behavior.
- Added runtime control command unit coverage and explicit SDK boundary
  documentation.
- Clarified etcd config-store creation with `Config.EtcdConfigStore`.
- Added DataPlane manager status tracking and richer rebuild failure logging.
- Added formatting checks to the release verification gate.
- Added release engineering targets, CI workflow, Apache-2.0 license, and
  package documentation for public packages.
- Promoted `servicekit` from `runtime/servicekit` to the top-level
  `servicekit` package as the stable microservice onboarding facade.
