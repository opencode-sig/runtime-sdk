# Changelog

## Unreleased

- Added `servicekit.NewConventionConfigLoader` for go-template-style split
  service config fragments, including etcd `PutIfAbsent` seeding for missing
  convention keys.
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
