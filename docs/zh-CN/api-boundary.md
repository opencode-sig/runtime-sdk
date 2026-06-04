# API 边界

`runtime-sdk` 是公开运行时 SDK。它必须可以被不同应用仓库复用，而不依赖任何具体业务项目。

## 公开包

- `servicekit`：受管理 gRPC 服务入口、服务生命周期契约、约定式拆分配置 loader、兼容单文件的 bootstrap / managed config loader、SDK 统一的 DataPlane generation 标识和 etcd 首次配置 seed。
- `runtime/component`：HTTP、gRPC、close hook、服务注册等通用 lifecycle 组件。
- `runtime/config`：配置存储契约、本地文件 provider、etcd provider。
- `runtime/control`：rebuild/restart 控制命令契约和命令存储。
- `runtime/discovery`、`runtime/registry`、`runtime/grpcclient`：服务发现、注册和 gRPC client 连接管理。
- `runtime/gatewaymeta`：Gateway 路由元数据和 protobuf descriptor 发布辅助能力。
- `security/authn`：认证请求、认证决策、凭证提取、身份上下文和身份 metadata 辅助能力。
- `security/authn/grpcauth`：通过 runtime discovery 调用 SDK security Auth 服务的 gRPC 适配器。
- `protobuf/security/v1`：应用无关的 Auth gRPC 服务契约。
- `observability`：通用健康检查、指标和 tracing 辅助能力。
- `infra`：常见基础设施 client 的可选构造器和配置对象。
- `logger`、`rpcerror`、`apperror`：独立的横切能力。

没有列在这里的包，除非在 release note 中明确提升为公开能力，否则都应视为实现细节。

## 边界规则

- SDK 代码不能引用外部项目的 `internal` 包。
- SDK 代码不能硬编码应用名、服务名、本地配置路径或项目默认 prefix。
- `servicekit` 不能依赖 protobuf 生成包、Gin、Gateway response envelope 或具体平台应用。
- `servicekit` 可以作为接入门面聚合可选 infra 的公开配置和 client facade；底层 core runtime 仍然不能依赖 MySQL、Redis、Kafka、Elasticsearch、MinIO/S3 等可选 infra。
- `runtime/*` 包不能反向依赖顶层 `servicekit` 门面。
- core runtime 包不能依赖可选的 MySQL、Redis、Kafka、Elasticsearch、MinIO/S3 包。
- `logger`、`rpcerror`、`apperror` 不能依赖 runtime 或 infra 包。
- 服务声明 Gateway 路由时必须写显式公网网关路径。`runtime/gatewaymeta`
  只规范化斜杠，不会自动添加服务名前缀。Gateway 应根据元数据转发，
  不应从 URL 前缀反推服务名。
- 路由认证白名单属于声明式 metadata。服务拥有 public 路由时必须通过
  `runtime/gatewaymeta.Public` 标记；Gateway 启用认证时，非 public 动态
  路由应默认需要认证，且不应维护独立 path 白名单。
- Gateway 实现应通过 `security/authn` 和 `security/authn/grpcauth`
  完成认证，而不是引用应用本地 Auth protobuf 契约。Auth 拒绝应通过
  `AuthenticateResponse.allowed=false` 表达；gRPC error 只表示 transport
  或基础设施失败。
- `AuthenticateRequest.target_service` 和 `authn.Request.TargetService`
  是应用无关的委托提示。SDK 只将它们透传给接收方标准 AuthService，
  不能维护下游 AuthService 列表，也不能执行委托策略。
- 业务服务不能解析 credential，也不能依赖 Auth 服务内部实现。业务服务
  只应读取 Gateway 生成的 `x-auth-subject`、`x-tenant-id` 和安全的
  `x-auth-attr-*` 等身份 metadata。
- `servicekit` 可以向服务初始化上下文中的 logger 注入
  `runtime_service`、`runtime_mode`、`instance_id` 等运行时日志身份字段。
  这些字段只描述当前产生日志的实例，不能承载应用业务身份。日志描述被操作的
  其他实例时应使用 `target_instance_id`。
- DataPlane generation 是 runtime 层身份，不属于 Gateway 或业务逻辑。
  自定义 DataPlane owner 应调用 `servicekit.NewGeneration`，并保留自己的
  lifecycle 组装逻辑；不要复制 generation 规则，也不要为了复用规则而强行套入
  普通 gRPC 服务模板。
- HTTP backend Gateway 路由属于声明式 route metadata。服务或平台组件应通过
  `runtime/gatewaymeta.HTTPProxy` 声明。Gateway 实现应根据
  `backend.http.service` 解析 registry 实例，并使用实例 metadata 中的
  `advertise_http_addr` 作为 HTTP upstream 地址；registry instance 的
  `address` 字段仍然表示 gRPC 地址。
- `servicekit.Spec.RegisterHTTP` 是基于标准库 `http.ServeMux` 的业务 HTTP
  handler 注册 hook。它不应要求 Gin、应用 response envelope 或 Gateway
  专用 handler 包。
- 服务 HTTP `/healthz` 表示本地 liveness；服务 HTTP `/readyz` 表示本地
  readiness 加服务显式声明的关键依赖检查。etcd、registry、Gateway metadata
  发布和 control watcher 属于控制面问题，默认不应让这两个 endpoint 失败。
  这些问题应通过结构化日志和 `runtime_control_plane_status`、
  `runtime_control_plane_errors_total`、`runtime_control_plane_recoveries_total`
  等控制面指标暴露。
- `runtime/registry.ServiceInstance` 的外部序列化契约使用 snake_case JSON/YAML
  字段，例如 `started_at`、`last_seen`、`data_plane_generation`。registry 或
  discovery 消费方不应依赖 Go 结构体字段名作为存储格式。
- `runtime/registry.ErrRegistrationExpired` 表示注册租约或底层记录已经丢失。
  lifecycle 组件收到该错误时可以重建注册；普通 registry 连接抖动不应映射为
  这个错误。
- Gateway 响应行为默认由应用网关包标准 JSON envelope。浏览器可直接渲染
  或文件型 raw 输出属于 Gateway 路由契约，必须通过
  `runtime/gatewaymeta.RawResponse` 显式声明。Gateway 实现应基于 protobuf
  descriptor 静态编译 `response.raw` 元数据，不能根据方法名、URL path、
  content-type 字段或普通 `body` 字段猜测 raw 输出。
- Raw response 元数据只负责声明契约。`runtime/gatewaymeta` 可以描述
  `content_type`、`body`、`status`、`headers`，但不能依赖 Gin、具体 HTTP
  handler 或应用 response envelope 包。

发布前执行：

```sh
make verify
```

该命令会执行格式检查、go module tidy 检查、单元测试、`go vet` 和边界检查。
