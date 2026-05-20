# API 边界

`runtime-sdk` 是公开运行时 SDK。它必须可以被不同应用仓库复用，而不依赖任何具体业务项目。

## 公开包

- `servicekit`：受管理 gRPC 服务入口和服务生命周期契约。
- `runtime/component`：HTTP、gRPC、close hook、服务注册等通用 lifecycle 组件。
- `runtime/config`：配置存储契约、本地文件 provider、etcd provider。
- `runtime/control`：rebuild/restart 控制命令契约和命令存储。
- `runtime/discovery`、`runtime/registry`、`runtime/grpcclient`：服务发现、注册和 gRPC client 连接管理。
- `runtime/gatewaymeta`：Gateway 路由元数据和 protobuf descriptor 发布辅助能力。
- `observability`：通用健康检查、指标和 tracing 辅助能力。
- `infra`：常见基础设施 client 的可选构造器和配置对象。
- `logger`、`rpcerror`、`apperror`：独立的横切能力。

没有列在这里的包，除非在 release note 中明确提升为公开能力，否则都应视为实现细节。

## 边界规则

- SDK 代码不能引用外部项目的 `internal` 包。
- SDK 代码不能硬编码应用名、服务名、本地配置路径或项目默认 prefix。
- `servicekit` 不能依赖 protobuf 生成包、Gin、Gateway response envelope 或具体平台应用。
- `servicekit` 可以作为接入门面聚合可选 infra 的公开配置和 client facade；底层 core runtime 仍然不能依赖 MySQL、Redis、Kafka 等可选 infra。
- `runtime/*` 包不能反向依赖顶层 `servicekit` 门面。
- core runtime 包不能依赖可选的 MySQL、Redis、Kafka 包。
- `logger`、`rpcerror`、`apperror` 不能依赖 runtime 或 infra 包。
- 服务声明 Gateway 路由时必须写显式公网网关路径。`runtime/gatewaymeta`
  只规范化斜杠，不会自动添加服务名前缀。Gateway 应根据元数据转发，
  不应从 URL 前缀反推服务名。

发布前执行：

```sh
make verify
```

该命令会执行格式检查、go module tidy 检查、单元测试、`go vet` 和边界检查。
