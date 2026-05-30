# runtime-sdk

[English](README.md)

`github.com/opencode-sig/runtime-sdk` 为受管理的 Go gRPC 微服务提供通用运行时能力。

SDK 负责那些不应该泄漏到业务代码里的运行时关注点：

- 带上下文关联字段的结构化日志；
- 应用错误封装和 gRPC 业务错误；
- 健康检查、Prometheus 指标和 tracing 辅助能力；
- 认证契约和身份透传辅助能力；
- etcd、MySQL、Redis、Kafka、Elasticsearch、MinIO/S3 的基础设施配置与客户端构造；
- 生命周期组件；
- 配置中心、注册中心、服务发现、控制命令和 Gateway 元数据契约；
- `servicekit`，用于内部和外部 gRPC 微服务接入的标准 SDK。

## 服务入口

业务服务只需要提供 protobuf 注册函数和 Gateway 元数据。SDK 负责 transport、registry、lifecycle、observability 和 control-command rebuild。

### 快速开始

```go
err := servicekit.Run(ctx, servicekit.RunOptions{
    Spec: servicekit.Spec{
        Name: "payment",
        RegisterGRPC: func(s grpc.ServiceRegistrar) {
            paymentv1.RegisterPaymentServiceServer(s, handler.New())
        },
        GatewayPublication: paymentbootstrap.GatewayPublication,
    },
    LoadConfig: servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
        Root: ".",
    }),
})
```

约定式 loader 会从拆分配置合成完整 `servicekit.Config`：
`configs/runtime.yaml`、`configs/logger.yaml`、`configs/registry.yaml`、
`configs/infra/*.yaml` 和 `configs/service/<service>.yaml`。服务文件只描述
当前服务自己的监听地址和 `settings`。

file 模式直接读取本地目录；etcd 模式以本地 `configs/runtime.yaml` 作为
bootstrap，然后从配置中心读取同名逻辑 key。如果 etcd key 不存在且本地存在同名
文件，SDK 会通过 `PutIfAbsent` 自动 seed，随后再从 etcd 读取；已有 etcd 配置不会被覆盖，且只会 seed 当前服务的 `configs/service/<service>.yaml`。

`configs/runtime.yaml` 中的 `config.etcd` 是配置中心和控制通道的 bootstrap
配置；即使 `config.provider: file`，约定式 loader 也会在
`configs/registry.yaml` 声明 `provider: etcd` 但未写 endpoints 时，复用这里的
endpoints。业务代码通过 `ctx.Infra.Etcd()` 获取的 etcd client 则读取
`configs/infra/etcd.yaml`，两者可以指向同一个集群，也可以按部署需要分开。

当 `runtime.config.root` 为空时，SDK 会自动补齐为 loader root，这样服务在
`Init` / `InitDistributed` 中可以通过
`ctx.Configs.Decode(ctx, "configs/global/app.yaml", &cfg)` 使用与 file / etcd
一致的逻辑 key。

接入方仍然可以使用 `servicekit.NewConfigLoader` 读取旧的单文件完整
`servicekit.Config`，也可以为特殊部署环境提供自定义 `LoadConfig`。

### etcd 配置与 rebuild

服务需要从 etcd 加载托管配置时，在 `configs/runtime.yaml` 中声明
`config.provider: etcd`。本地拆分配置也是首次启动时 etcd key 缺失的
seed 来源。

```go
loader := servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
    Root: ".",
    // RuntimeKey 为空时默认使用 configs/runtime.yaml。
})
```

默认 managed key 位于 `configs/` 命名空间下，包括
`configs/service/<service>.yaml`。如果平台使用其他逻辑命名空间，可以覆盖
`ManagedConfigPrefix`。如果服务进程只能读配置中心，可以设置
`DisableEtcdAutoSeed` 关闭自动 seed。

完整外部服务接入示例见 [docs/zh-CN/go-template-service-example.md](docs/zh-CN/go-template-service-example.md)。可运行样例位于 [examples/go-template-payment](examples/go-template-payment)。

服务也可以通过 `servicekit.DistributedContext.Clients` 按服务名获取其他 gRPC 服务连接：

```go
userClient, err := servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
```

### 运行时日志身份

`servicekit` 会向服务初始化上下文中的 logger 注入运行时身份字段。通过
`ctx.Logger` 打出的日志会默认包含：

```text
runtime_service  service spec 中的逻辑服务名
runtime_mode     distributed、monolith 或 hybrid
instance_id      当前服务实例在注册中心中的实例 ID
```

请求级日志应传入当前请求 `context.Context`。SDK logger 会从 context 或
gRPC metadata 中追加 `request_id`、`client_ip`、`trace_id`、`user_id` 等关联字段。
`instance_id` 只表示当前产生日志的实例；如果日志描述的是一个被操作的目标实例，
请使用 `target_instance_id`。

Gateway route spec 使用显式公网网关路径，例如 `/v1/payments/{id}`。
`runtime/gatewaymeta` 只规范化斜杠，不会自动添加服务名前缀。Gateway
应该根据发布后的 `RouteMeta.GRPC.Service` 和 `RouteMeta.GRPC.FullMethod`
转发，而不是从 URL 前缀解析服务名。

路由级认证白名单也属于 route metadata。Gateway 启用认证时，动态路由默认
应该需要认证；服务必须通过 `Public()` 显式声明白名单路由：

```go
gatewaymeta.POST("Authenticate", "/v1/auth/authenticate").
    Body("*").
    Public()
```

Gateway 实现应读取 `RouteMeta.Auth.Public`，不应在 Gateway 配置中维护独立
path pattern 白名单。

SDK 拥有跨服务认证契约。Gateway 应通过 `security/authn` 提取认证凭证，
通过 `security/authn/grpcauth` 调用 Auth gRPC 服务，并且只向下游服务
透传由 Gateway 生成的身份 metadata：

```text
Authorization: Bearer <token> -> credential_type=bearer
Authorization: Basic <value>  -> credential_type=basic
X-API-Key: <api-key>          -> credential_type=api_key
apitoken: <legacy-api-token>  -> credential_type=api_key
```

Auth 服务实现 `protobuf/security/v1.AuthService`。业务服务不应解析
credential，也不应直接调用 Auth；只应读取 `x-auth-subject`、
`x-tenant-id` 和安全的 `x-auth-attr-*` 等身份 metadata。

普通 Gateway 路由默认由应用网关包标准 JSON envelope。需要浏览器直接渲染
或文件型输出的路由必须显式声明 raw response：

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
    Body("*").
    RawResponse("text/html; charset=utf-8")
```

`RawResponse` 会发布 `response.raw` 元数据。Gateway 实现应在加载路由时
用 protobuf response descriptor 静态编译该元数据，然后把配置的 `body`
字段直接写入 HTTP response。可选的 `status` 和 `headers` 字段可以通过
`RawStatus`、`RawHeaders` 声明。Gateway 不应根据方法名或普通 response
字段名猜测 raw 输出。

受管理 gRPC component 会在服务 admin `/metrics` 暴露 Prometheus 指标。
除兼容旧看板的 `runtime_grpc_*` 指标外，SDK 默认记录常用 gRPC server
指标，包括 `grpc_server_started_total`、`grpc_server_handled_total`、
`grpc_server_handling_seconds`、in-flight gauge、panic 计数、deadline
计数和 protobuf 消息大小直方图。

## Rebuild 语义

`servicekit` rebuild 的流程是：从最新配置创建新的 DataPlane，停止旧 generation，再启动新的 generation。这个设计让单进程、同端口服务的运行时核心保持简单和可预测；它不是同进程零停机切换。

control command channel 属于 bootstrap 级配置。如果 etcd endpoints 或 command prefix 发生变化，需要重启进程，让 watcher 移动到新的控制通道。

## 运行模式

`servicekit` 暴露稳定的运行模式常量：

- `servicekit.RuntimeModeDistributed`
- `servicekit.RuntimeModeMonolith`

业务服务不应该根据这些值写分支逻辑。它们主要用于运行时元数据、运维观察和 discovery 消费方。

## Infra 构造规则

infra 包遵循统一规则：

- `Config.IsZero()` 判断组件是否未配置；
- `Config.Normalize()` 填充常用默认值，不返回错误；
- `Config.Validate()` 允许零配置，对非零配置严格校验；
- `New*` 构造函数创建 client 或 pool，但不强制连通；
- `Ping` 或 `Check` 方法用于显式连通性检查。

这样进程可以在可选 infra 未启用或暂时不可达时启动，由更上层 runtime 决定何时检查 readiness。

## 边界

SDK 必须保持应用无关。它不能引用应用侧 `internal` 包，不能内置项目配置路径，也不能硬编码服务名。

包边界规则见 [docs/zh-CN/api-boundary.md](docs/zh-CN/api-boundary.md)。

发布前执行：

```sh
make verify
make race
```

发布检查清单见 [docs/zh-CN/release.md](docs/zh-CN/release.md)。
