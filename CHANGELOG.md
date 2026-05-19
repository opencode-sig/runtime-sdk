# Changelog

## Unreleased

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
