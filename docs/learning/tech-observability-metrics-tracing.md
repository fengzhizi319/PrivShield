# 生产级可观测性、Prometheus 指标与分布式追踪技术指南 / Observability, Prometheus, OpenTelemetry & Logging Technical Guide

## 1. 技术简介 / Introduction

在分布式微服务与云原生隐私计算场景中，**可观测性（Observability）** 是保障高可用 SLA、排查分布式跨语言调用延迟与实现合规审计的基石。

`PrivShield` 实现了完整的“三位一体”可观测性架构：
1. **指标度量（Metrics）**：基于 **Prometheus** 暴露微秒级请求延迟直方图、隐私预算实时水位与分类漏斗命中计数；
2. **分布式追踪（Distributed Tracing）**：基于 **OpenTelemetry (OTel)** 统一 W3C TraceContext 跨 Go BFF、Python Agent 及调度中枢的服务链路串联；
3. **结构化日志（Structured Logging）**：支持标准 JSON / Text 双格式，通过 `ContextFilter` 自动注入全局唯一 `request_id` 与认证身份 `identity_name`。

```text
       外部请求 / Client Request
                  │ (携带 X-Request-ID / W3C traceparent)
                  ▼
   ┌──────────────────────────────┐
   │ Go BFF / 网关 (console/bff)   │ ──► 生成/透传 Span Context
   └──────────────┬───────────────┘
                  │ gRPC / HTTP (Metadata / Headers)
                  ▼
   ┌──────────────────────────────┐
   │ Python Agent (engine/main.py)│
   │  ┌────────────────────────┐  │
   │  │ ObservabilityMiddleware│  │ ──► 1. 提取 Request-ID 与 TraceContext
   │  └───────────┬────────────┘  │ ──► 2. 统计处理延迟与响应状态码
   │              ▼               │ ──► 3. 记录结构化访问日志 (JSON)
   │     业务处理 (DP/Mask/Funnel) │
   │              │               │
   │              ▼               │
   │  ┌────────────────────────┐  │
   │  │ Prometheus 指标注册中心  │  │ ──► 4. 更新预算水位 Gauge / 漏斗 Counter
   │  └────────────────────────┘  │
   └──────────────┬───────────────┘
                  │
                  ▼
  Prometheus Server / OTLP Collector (Grafana 实时监控与 Jaeger 链路大盘)
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 Prometheus 指标体系与业务埋点 / Prometheus Metrics Instrumentation

文件 / File：[`engine/observability/metrics.py`](engine/observability/metrics.py)

`PrivShield` 预设了细粒度的系统级与隐私业务级指标：

```python
from prometheus_client import Counter, Gauge, Histogram

# 1. 核心 HTTP/gRPC 请求耗时分布直方图 (Histogram)
REQUEST_DURATION = Histogram(
    "privacy_request_duration_seconds",
    "Request latency in seconds.",
    ["method", "path"],
    buckets=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0],
)

# 2. 动态分类分级三层漏斗命中 Counter
CLASSIFICATION_TOTAL = Counter(
    "privacy_classification_total",
    "Total number of classification results by final level and layer.",
    ["final_level", "layer"],  # layer: L1_RULE, L2_SMALL_NER, L3_LLM
)

# 3. 各命名空间剩余隐私预算实时水位 (Gauge)
BUDGET_REMAINING = Gauge(
    "privacy_budget_remaining",
    "Remaining privacy budget (epsilon or delta) per namespace.",
    ["namespace", "budget_type"],  # budget_type: epsilon, delta
)

# 4. 差分隐私查询调用计数
DP_QUERIES_TOTAL = Counter(
    "privacy_dp_queries_total",
    "Total number of differential privacy queries.",
    ["mechanism", "aggregation"],
)
```

#### 指标端点安全挂载与鉴权保护

文件 / File：[`engine/main.py`](engine/main.py#L460-L490)

在启用生产 API 鉴权时，`/metrics` 端点不会裸露，而是由专用 ASGI 中间件拦截并校验调用方是否具备 `ops:metrics` 作用域：

```python
metrics_app = make_asgi_app()
if _auth_enabled:
    metrics_app = ApiKeyAuthAsgiMiddleware(metrics_app, required_scope="ops:metrics")

app.mount("/metrics", metrics_app)
```

---

### 2.2 OpenTelemetry 分布式追踪集成 / OpenTelemetry Tracing

文件 / File：[`engine/observability/tracing.py`](engine/observability/tracing.py)

```python
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

def init_tracing(endpoint: str | None, service_name: str = "PrivShield") -> None:
    """初始化全局 OpenTelemetry TracerProvider 与 OTLP gRPC 导出器。"""
    if not endpoint:
        # 未配置 OTLP 端点时保持 no-op，零性能损耗
        return

    resource = Resource.create({"service.name": service_name})
    provider = TracerProvider(resource=resource)
    exporter = OTLPSpanExporter(endpoint=endpoint, insecure=True)
    processor = BatchSpanProcessor(exporter)
    provider.add_span_processor(processor)
    trace.set_tracer_provider(provider)
```

---

### 2.3 结构化 JSON 日志与请求上下文注入 / Structured Logging & Context Filter

文件 / File：[`engine/observability/logging_config.py`](engine/observability/logging_config.py) & [`engine/observability/context.py`](engine/observability/context.py)

为了在海量日志中实现毫秒级全链路检索，`PrivShield` 使用 Python `contextvars` 在请求入口捕获上下文，并通过自定义 `_ContextFilter` 自动为每条日志追加链路元数据：

```python
class _ContextFilter(logging.Filter):
    """自动将 request_id、identity_name、method 等字段注入每个 LogRecord。"""
    def filter(self, record: logging.LogRecord) -> bool:
        ctx = get_request_context()
        record.request_id = ctx.request_id if ctx else "-"
        record.identity_name = ctx.identity_name if ctx else "anonymous"
        record.method = ctx.method if ctx else "-"
        record.path = ctx.path if ctx else "-"
        return True
```

#### 输出示例（JSON 模式）：
```json
{
  "timestamp": "2026-08-25T19:20:00.123Z",
  "level": "INFO",
  "name": "engine.dynclassification.funnel",
  "message": "Field classified successfully",
  "request_id": "req-9b87f4c2-8412",
  "identity_name": "service-hub-client",
  "method": "POST",
  "path": "/v1/classify/field",
  "final_level": "S3",
  "layer": "L1_RULE"
}
```

---

## 3. 请求上下文传播机制 / Request Context Propagation via contextvars

文件 / File：[`engine/observability/context.py`](engine/observability/context.py)

在异步 Python中，传统的线程局部存储（`threading.local()`）无法在协程之间正确传播请求元数据。PrivShield 使用 Python 3.7+ 的 `contextvars` 模块实现**请求级上下文透传**，确保每个异步任务都能看到正确的 request_id、identity 等信息。

```python
from contextvars import ContextVar
from dataclasses import dataclass

@dataclass(frozen=True)
class RequestContext:
    """每个请求的元数据，贯穿日志、指标与追踪三大支柱。"""
    request_id: str       # 全局唯一请求关联 ID
    method: str           # HTTP 方法或 gRPC 方法名
    path: str             # REST 路径或 gRPC 完整方法名
    identity_name: str = ""  # 认证后的调用方身份

# 模块级 ContextVar —— 每个异步任务自动拥有独立副本
_request_context: ContextVar[RequestContext | None] = ContextVar(
    "request_context", default=None
)

def set_request_context(ctx: RequestContext) -> None:
    _request_context.set(ctx)

def get_request_context() -> RequestContext | None:
    return _request_context.get(None)
```

**为什么用 contextvars 而不是 threading.local？**

| 特性 | `threading.local()` | `contextvars.ContextVar` |
|---|---|---|
| 协程安全 | ❌ 同一线程内多协程共享 | ✅ 每个 Task 独立副本 |
| asyncio 集成 | ❌ 需手动传递 | ✅ `asyncio.create_task()` 自动复制 |
| 线程池兼容 | ✅ 每线程独立 | ✅ `asyncio.to_thread()` 安全 |

> **学习要点**：`ContextVar` 是 Python 异步编程中实现「请求作用域数据」的标准方案。它类似于 Java 的 `ThreadLocal`，但粒度细化到了协程级别。

---

## 4. 结构化日志系统深度解析 / Structured Logging Deep Dive

文件 / File：[`engine/observability/logging_config.py`](engine/observability/logging_config.py)

### 4.1 双格式输出（JSON / Text）

PrivShield 支持通过 `PRIVACY_LOG_FORMAT` 环境变量切换日志格式：
- **text**（默认）：人类可读的彩色文本格式，适合开发调试
- **json**：机器可解析的 JSON 格式，适合生产环境 ELK/Loki 采集

```python
def configure_logging(log_level: str = "INFO", json_format: bool = False,
                      service_name: str = "PrivShield") -> None:
    root = logging.getLogger()
    # 清除已有 handler，避免多次调用时重复添加
    for handler in root.handlers[:]:
        root.removeHandler(handler)

    handler = logging.StreamHandler(sys.stdout)
    handler.addFilter(_ContextFilter())  # 注入请求上下文

    if json_format:
        # JSON 格式：使用 python-json-logger 库
        # 格式字符串列出所有要输出的字段
        fmt = ("%(asctime)s %(levelname)s %(name)s %(message)s "
               "%(request_id)s %(identity_name)s %(method)s %(path)s "
               "%(lineno)d %(funcName)s")
        formatter = jsonlogger.JsonFormatter(
            fmt,
            rename_fields={"asctime": "timestamp", "levelname": "level"},
        )
        handler.setFormatter(formatter)
    else:
        # 文本格式：简洁可读
        handler.setFormatter(logging.Formatter(
            "%(asctime)s [%(levelname)s] %(name)s: %(message)s "
            "request_id=%(request_id)s identity=%(identity_name)s"
        ))

    root.addHandler(handler)
    root.setLevel(log_level.upper())
    # 降低第三方库日志级别，减少噪音
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
```

### 4.2 _ContextFilter 自动注入原理

`_ContextFilter` 是连接「请求上下文」与「日志系统」的桥梁。它在每条 `LogRecord` 生成时自动从 `ContextVar` 中读取当前请求的元数据：

```python
class _ContextFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        ctx = get_request_context()
        record.request_id = ctx.request_id if ctx else ""
        record.identity_name = ctx.identity_name if ctx else ""
        record.method = ctx.method if ctx else ""
        record.path = ctx.path if ctx else ""
        return True  # 始终返回 True，表示不丢弃任何日志记录
```

> **关键设计**：Filter 的 `filter()` 方法不仅用于过滤，还用于**修改** LogRecord。这里我们总是返回 `True`（不丢弃），但趁机注入了 4 个自定义字段。

### 4.3 JSON 日志输出示例与查询

```json
{
  "timestamp": "2026-08-25T19:20:00.123Z",
  "level": "INFO",
  "name": "engine.dynclassification.funnel",
  "message": "Field classified successfully",
  "request_id": "req-9b87f4c2-8412",
  "identity_name": "service-hub-client",
  "method": "POST",
  "path": "/v1/classify/field",
  "lineno": 142,
  "funcName": "classify_field"
}
```

在 Grafana Loki 中查询特定请求的所有日志：
```logql
{job="privshield"} | json | request_id="req-9b87f4c2-8412"
```

---

## 5. OpenTelemetry 分布式追踪集成 / Distributed Tracing

文件 / File：[`engine/observability/tracing.py`](engine/observability/tracing.py)

### 5.1 可选依赖与零开销降级

OpenTelemetry 是 PrivShield 的**可选依赖**。未安装或未配置时，使用 NoOp tracer，零性能开销：

```python
try:
    from opentelemetry import trace
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
    _HAS_OTEL = True
except ImportError:
    _HAS_OTEL = False
    trace = None
```

### 5.2 初始化流程与 NoOp 回退

```python
def init_tracing(endpoint: str | None = None,
                 service_name: str = "PrivShield") -> Any:
    global _tracer
    if not _HAS_OTEL:
        _tracer = _noop_tracer()  # 库未安装，使用空操作 tracer
        return _tracer

    endpoint = endpoint or os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT")
    if not endpoint:
        _tracer = trace.get_tracer(__name__)  # 未配置端点，使用默认 tracer
        return _tracer

    # 配置完整的 OTLP 导出管道
    provider = TracerProvider(
        resource=Resource({"service.name": service_name})
    )
    exporter = OTLPSpanExporter(endpoint=endpoint)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)
    _tracer = provider.get_tracer(service_name)
    return _tracer
```

### 5.3 业务代码中的 Span 使用

```python
from engine.observability.tracing import start_span

# 使用上下文管理器创建 Span
with start_span("classify_field", {"field_name": "patient_name", "domain": "healthcare"}) as span:
    result = funnel.classify_field(field_name, field_value)
    if span:
        span.set_attribute("final_level", result.final_level)
        span.set_attribute("layer", result.engine_layer)
```

### 5.4 优雅关闭与 Span 刷出

进程退出前必须刷出缓冲的 Span，否则最后一批追踪数据会丢失：

```python
def shutdown_trace() -> None:
    """刷新并关闭 TracerProvider，确保所有缓冲 span 在进程退出前导出。"""
    if not _HAS_OTEL:
        return
    provider = trace.get_tracer_provider() if trace else None
    if provider and hasattr(provider, "shutdown"):
        provider.shutdown()
```

---

## 6. /metrics 端点安全挂载 / Metrics Endpoint Security

文件 / File：[`engine/main.py`](engine/main.py)

在生产环境启用 API 鉴权时，`/metrics` 端点不会裸露，而是由专用 ASGI 中间件拦截并校验调用方是否具备 `ops:metrics` 作用域：

```python
from prometheus_client import make_asgi_app

metrics_app = make_asgi_app()
if _auth_enabled:
    # 将 Prometheus ASGI 应用包装在鉴权中间件内
    metrics_app = ApiKeyAuthAsgiMiddleware(metrics_app, required_scope="ops:metrics")

app.mount("/metrics", metrics_app)
```

> **安全设计原则**：Prometheus 指标包含服务内部状态（预算水位、节点拓扑等），不应暴露给未认证的调用方。通过 ASGI 中间件包装，实现了「指标端点与应用共用端口但独立鉴权」的安全模型。

---

## 7. Go 微服务统一可观测性对接 / Go Microservices Observability

文件 / File：[`pkg/metrics/metrics.go`](pkg/metrics/metrics.go) & [`pkg/middleware/middleware.go`](pkg/middleware/middleware.go)

### 7.1 独立 Registry 设计

Go 中台微服务（`service-hub`, `datasource-mgr`, `audit-log`, `bff-go`）统一使用 `pkg/metrics` 包。每个模块创建**独立的 `prometheus.Registry`**，避免全局注册冲突：

```go
func NewCollector(module string) *Collector {
    reg := prometheus.NewRegistry()  // 独立注册表，非全局 DefaultRegisterer
    c := &Collector{
        module: module,
        registry: reg,
        HTTPRequestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name:        "http_requests_total",
                Help:        "Total HTTP requests processed.",
                ConstLabels: prometheus.Labels{"module": module},
            },
            []string{"method", "path", "status"},
        ),
        // ... 更多指标
    }
    reg.MustRegister(c.HTTPRequestsTotal, c.HTTPRequestDuration, ...)
    return c
}
```

### 7.2 Gin 自动指标中间件

```go
func (c *Collector) HTTPMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        start := time.Now()
        path := ctx.FullPath()  // 使用路由模板而非实际路径，避免高基数
        if path == "" {
            path = ctx.Request.URL.Path
        }
        ctx.Next()  // 执行后续 handler

        // 跳过 /metrics 端点自身，避免递归采集
        if path == "/metrics" {
            return
        }
        duration := time.Since(start).Seconds()
        c.RecordHTTP(ctx.Request.Method, path, ctx.Writer.Status(), duration)
    }
}
```

> **学习要点**：`ctx.FullPath()` 返回路由模板（如 `/api/lz/tasks/:id`）而非实际路径（如 `/api/lz/tasks/123`）。如果用实际路径作为 Prometheus 标签，会导致标签基数爆炸（每个 task ID 一个时间序列），压垂 Prometheus 内存。

### 7.3 Phase B 租约指标

针对 PostgreSQL 租约机制（`FOR UPDATE SKIP LOCKED`），`pkg/metrics` 提供了专用的租约可观测性指标：

| 指标名 | 类型 | 用途 |
|---|---|---|
| `task_lease_conflicts_total` | Counter | 租约所有权冲突数 |
| `task_lease_expired_total` | Counter | 租约到期回收事件数 |
| `task_claim_latency_seconds` | Histogram | 任务领取（ClaimNext）延迟 |
| `task_transitions_total` | Counter | 任务状态转换次数（from/to/result） |
| `service_hub_ready` | Gauge | 服务就绪标志（1=ready, 0=not ready） |

---

## 8. 完整指标体系速查表 / Metrics Quick Reference

### 8.1 Python Agent 指标

| 指标名 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_requests_total` | Counter | method, path, status | 请求总数 |
| `privacy_request_duration_seconds` | Histogram | method, path | 请求延迟分布 |
| `privacy_dp_queries_total` | Counter | mechanism, aggregation | DP 查询计数 |
| `privacy_budget_remaining` | Gauge | namespace, budget_type | 剩余隐私预算 |
| `privacy_classification_total` | Counter | final_level, layer | 分类结果计数 |
| `privacy_auth_denials_total` | Counter | reason | 认证/授权拒绝计数 |
| `privacy_traffic_bytes_total` | Counter | method, path, direction | 流量字节数 |
| `privacy_masking_operations_total` | Counter | operation | 脱敏操作计数 |
| `privacy_kano_operations_total` | Counter | operation | K-匿名操作计数 |
| `privacy_qol_operations_total` | Counter | domain | 查询混淆操作计数 |
| `classification_rule_hits_total` | Counter | rule_id, domain, standard | 动态分类规则命中 |
| `classification_operator_calls_total` | Counter | operator, result | 算子调用计数 |
| `classification_engine_load_duration_seconds` | Histogram | domain, standard | 引擎加载耗时 |

### 8.2 网关指标

| 指标名 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_gateway_requests_total` | Counter | protocol, method, status | 代理请求计数 |
| `privacy_gateway_latency_seconds` | Histogram | protocol | 代理延迟 |
| `privacy_gateway_retries_total` | Counter | protocol, reason | 重试计数 |
| `privacy_gateway_healthy_nodes` | Gauge | — | 健康节点数 |
| `privacy_gateway_circuit_breaker_state` | Gauge | node | 熔断器状态 |
| `privacy_gateway_node_admin_state` | Gauge | node | 管理状态 |

### 8.3 Go 微服务指标

| 指标名 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `http_requests_total` | Counter | module, method, path, status | HTTP 请求计数 |
| `http_request_duration_seconds` | Histogram | module, method, path | HTTP 延迟 |
| `agent_requests_total` | Counter | module, endpoint, status | Agent 调用计数 |
| `agent_request_duration_seconds` | Histogram | module, endpoint | Agent 调用延迟 |
| `orphaned_tasks_recovered_total` | Counter | module, type | 崩溃恢复任务数 |
| `tasks_retried_total` | Counter | module, result | 重试任务数 |
| `circuit_breaker_state` | Gauge | module, node | 熔断器状态 |

---

## 9. Prometheus Histogram Bucket 设计原理 / Bucket Design

Histogram 的 bucket 配置直接决定了分位数计算的精度。PrivShield 针对不同延迟量级的指标设计了差异化的 bucket 序列：

```python
# 快速操作（脱敏、规则匹配）：微秒到秒级
buckets=[0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0]

# 慢速操作（LLM 推理）：秒到分钟级
buckets=[0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0]

# 认证检查：亚毫秒级
buckets=[0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1]
```

**设计原则**：
1. **E 系列倍数**：bucket 边界大致按 2x 或 2.5x 递增，兼顾精度与存储开销
2. **覆盖 P99 预期值**：最大 bucket 应大于指标的 P99 值，否则 `histogram_quantile` 会返回最后一个 bucket 边界
3. **避免过高基数**：bucket 数量控制在 10~13 个，每个 bucket 是一个独立时间序列

---

## 10. 运维实战：Prometheus 查询与 Grafana 配置 / Operations

### 10.1 常用 PromQL 查询

```promql
# 请求 QPS（每秒请求数）
rate(privacy_requests_total[5m])

# P99 请求延迟
histogram_quantile(0.99, rate(privacy_request_duration_seconds_bucket[5m]))

# 各分类层级的占比
sum by (layer) (rate(privacy_classification_total[5m]))

# 隐私预算剩余百分比
privacy_budget_remaining{budget_type="epsilon"} / 1.0 * 100

# 网关健康节点数
privacy_gateway_healthy_nodes

# 熔断器状态（0=closed, 1=open, 2=half_open）
privacy_gateway_circuit_breaker_state
```

### 10.2 环境变量速查

| 变量 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_LOG_LEVEL` | `INFO` | 日志级别 |
| `PRIVACY_LOG_FORMAT` | `text` | 日志格式（text/json） |
| `PRIVACY_SERVICE_NAME` | `PrivShield` | 服务名（日志/追踪） |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP 追踪导出端点 |

### 10.3 启动命令

```bash
# 启动 Agent（文本日志）
python -m engine.server

# 启动 Agent（JSON 日志 + OTLP 追踪）
export PRIVACY_LOG_FORMAT=json
export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318/v1/traces
python -m engine.server

# 启动 Prometheus + Grafana 监控栈
docker compose --profile monitoring up -d
```
