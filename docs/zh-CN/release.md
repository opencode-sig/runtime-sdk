# 发布指南

本仓库按可复用 Go module 的标准发布。

发布前必须执行：

```sh
make verify
make race
```

可选验证：

```sh
make integration     # 需要 etcd，默认 ETCD_ENDPOINT=127.0.0.1:2379
make smoke-consumer  # 需要消费方项目 checkout 和 etcd
```

打 tag 前检查：

- 确认公开 API 仍然保持应用无关；
- 通过 `make verify` 执行格式、tidy、test、vet 和边界检查；
- 不要新增引用消费方项目的默认值、路径或 prefix；
- core runtime 和 `servicekit` 不能依赖可选 infra；
- 更新 `CHANGELOG.md`，记录用户可感知变更；
- 使用本地 `replace` 在消费方项目中跑集成 smoke。

## 版本策略

建议先发布 `v0.x`。在 `servicekit` 和配置契约足够稳定之前，不急于发布 `v1.0.0`。

一旦发布 `v1.0.0`，破坏性 API 变更应该只进入新的 major version。

## 推荐发布流程

```sh
make verify
make race
make integration
make smoke-consumer
```

确认无误后：

```sh
git tag v0.1.0
git push origin v0.1.0
```

如果当前环境没有 etcd 或消费方项目，至少必须完成 `make verify` 和 `make race`，并在 release note 中说明未执行的外部依赖型验证。
