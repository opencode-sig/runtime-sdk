# Release Guide

This repository is intended to be published as a reusable Go module:

```sh
make verify
make race
```

Optional checks:

```sh
make integration      # requires etcd at ETCD_ENDPOINT, default 127.0.0.1:2379
make smoke-distributed # runs the local payment/user distributed example
make resilience       # requires Docker etcd; verifies registry recovery
make smoke-consumer   # requires a consuming project checkout and etcd
```

Before tagging a release:

- confirm the public API remains application-neutral;
- run formatting checks through `make verify`;
- avoid adding defaults that reference any consuming project;
- keep optional infra packages out of core runtime; `servicekit` may aggregate
  public infra config and client facades for onboarding;
- update `CHANGELOG.md` with user-visible changes;
- run the consuming project's integration smoke tests with the local `replace`
  target before publishing.

Use semantic versioning. Prefer `v0.x` until the public `servicekit` and config
contracts are stable enough for a compatibility promise. Breaking API changes
after `v1.0.0` should be reserved for major versions.
