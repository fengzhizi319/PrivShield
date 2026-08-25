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

## 3. Go 微服务统一可观测性对接 / Go Microservices Observability

文件 / File：[`pkg/metrics/metrics.go`](pkg/metrics/metrics.go) & [`pkg/middleware/middleware.go`](pkg/middleware/middleware.go)

在 Go 中台微服务（`service-hub`, `datasource-mgr`, `audit-log`, `bff-go`）中，统一引入 `pkg/metrics`，自动为 Gin 路由注入 Prometheus 拦截器，并生成格式完全对齐的指标命名空间。
