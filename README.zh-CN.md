# runtime-sdk

[English](README.md)

`github.com/opencode-sig/runtime-sdk` 为受管理的 Go gRPC 微服务提供通用运行时能力。

SDK 负责那些不应该泄漏到业务代码里的运行时关注点：

- 带上下文关联字段的结构化日志；
- 应用错误封装和 gRPC 业务错误；
- 健康检查、Prometheus 指标和 tracing 辅助能力；
- etcd、MySQL、Redis、Kafka 的基础设施配置与客户端构造；
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
    LoadConfig: loadConfig,
})
```

`LoadConfig` 由接入方实现。它可以从本地文件、etcd 配置中心或任意部署侧配置源加载配置，并返回 `servicekit.Config`。
如果服务需要在 `Init` / `InitDistributed` 中读取全局配置，可以通过
`ctx.Configs.Decode(ctx, "configs/global/app.yaml", &cfg)` 使用与 file / etcd
一致的逻辑 key。文件模式下建议在返回配置前设置 `cfg.Runtime.Config.Root`，
让 `servicekit.Configs` 能定位配置根目录。

### 文件配置

```go
func loadFileConfig(ctx context.Context, root string, key string) (servicekit.Config, error) {
    provider := runtimeconfig.NewFileProvider(root)
    data, err := provider.Load(ctx, key)
    if err != nil {
        return servicekit.Config{}, err
    }
    cfg, err := runtimeconfig.Decode[servicekit.Config](data)
    if err != nil {
        return servicekit.Config{}, err
    }
    if cfg.Runtime.Config.Root == "" {
        cfg.Runtime.Config.Root = root
    }
    return cfg, nil
}
```

### etcd 配置与 rebuild

推荐使用一个很小的本地 bootstrap 配置决定服务是否从 etcd 加载托管配置。当返回的配置启用了 etcd 配置源时，`servicekit.Run` 会启动 control watcher，并响应通过 `runtime/control` 发布的 rebuild/restart 命令。

```go
func loadManagedConfig(ctx context.Context, bootstrap servicekit.Config) (servicekit.Config, error) {
    store, ok := bootstrap.EtcdConfigStore()
    if !ok {
        return bootstrap, nil
    }
    defer func() { _ = store.Close() }()

    data, err := store.Load(ctx, bootstrap.Runtime.Config.Key)
    if err != nil {
        return servicekit.Config{}, err
    }
    return runtimeconfig.Decode[servicekit.Config](data)
}
```

完整外部服务接入示例见 [docs/zh-CN/go-template-service-example.md](docs/zh-CN/go-template-service-example.md)。可运行样例位于 [examples/go-template-payment](examples/go-template-payment)。

服务也可以通过 `servicekit.DistributedContext.Clients` 按服务名获取其他 gRPC 服务连接：

```go
userClient, err := servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
```

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
