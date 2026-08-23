# console/pkg — 控制台共享基础库

`console/pkg` 是 **数盾 (PrivShield)** 控制台各 Go 微服务（`backend-go`、`service-hub`、`datasource-mgr`、`audit-log`）共享的核心基础库。

---

## 包含的包

| 子包 | 描述 |
|---|---|
| [`pkg/store`](./store/store.go) | 任务、数据源与脱敏审计日志的数据模型与存储接口，提供 SQLite 持久化与内存存储两套引擎 |
| [`pkg/middleware`](./middleware/middleware.go) | 统一 Gin 中间件：API Key 鉴权、CORS 跨域、Request ID 链路追踪、结构化日志、Panic Recovery 与安全响应头 |
| [`pkg/metrics`](./metrics/metrics.go) | 基于 Prometheus 的模块级指标收集器（Counter / Histogram）与 `/metrics` HTTP 端点 |
| [`pkg/agent`](./agent/client.go) | 访问上游 PrivShield Agent REST API 的共享 HTTP 客户端，具备熔断器、超时与 64MB 内存防护 |
| [`pkg/config`](./config/env.go) | 统一的环境变量解析工具（String/Int/Bool/Slice）与 `slog` 结构化日志器初始化 |
| [`pkg/validation`](./validation/validation.go) | 参数白名单校验、端口范围检查、字符串长度检查、抗碰撞唯一 ID 生成与安全分页解析 |

---

## 快速使用

### 1. 初始化持久化存储

```go
// 自动根据 dbPath 选择 SQLite 持久化或内存模式
func initTaskStore(dbPath string, logger *slog.Logger) (store.TaskStore, error) {
    if dbPath == "" {
        return memory.NewTaskStore(), nil
    }
    db, err := sqlite.Open(dbPath, logger)
    if err != nil {
        return nil, err
    }
    return sqlite.NewTaskStore(db)
}
```

### 2. 注入通用中间件

```go
router := gin.New()
router.Use(middleware.RequestID())
router.Use(middleware.StructuredLogger(logger, "service-hub"))
router.Use(middleware.CORS(cfg.CORSOrigins))
router.Use(middleware.Auth(cfg.APIKey))
router.Use(middleware.SecurityHeaders())
router.Use(middleware.Recovery(logger, "service-hub"))
```

### 3. 收集与暴露指标

```go
mc := metrics.NewCollector("service-hub")
router.Use(mc.HTTPMiddleware())
router.GET("/metrics", mc.Handler())
```

---

## 架构设计文档

详细架构设计与接口说明请参阅 [docs/design.md](./docs/design.md)。
