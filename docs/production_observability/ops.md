# PrivShield 可观测性运维手册

> 对应 PRD/设计: `docs/production_observability/prd.md`, `design.md`

---


## 目录 (Table of Contents)

- [1. 环境变量](#1-环境变量)
- [2. 日志样例](#2-日志样例)
  - [文本格式（默认）](#文本格式默认)
  - [JSON 格式](#json-格式)
- [3. Prometheus 指标抓取](#3-prometheus-指标抓取)
- [4. Grafana Dashboard 关键面板](#4-grafana-dashboard-关键面板)
- [5. Jaeger / Tempo Tracing](#5-jaeger-tempo-tracing)
- [6. 审计事件](#6-审计事件)

---

## 1. 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_LOG_LEVEL` | `INFO` | 日志级别：DEBUG/INFO/WARNING/ERROR/CRITICAL。 |
| `PRIVACY_LOG_FORMAT` | `text` | `text` 或 `json`。 |
| `PRIVACY_SERVICE_NAME` | `PrivShield` | 日志/tracing 中的服务名。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | 设置后启用 OpenTelemetry OTLP 导出，例 `http://jaeger:4317`。 |
| `OTEL_SERVICE_NAME` | — | OpenTelemetry service name；未设置时使用 `PRIVACY_SERVICE_NAME`。 |

---

## 2. 日志样例

### 文本格式（默认）

```text
2026-07-11 14:30:27,123 [INFO] engine.main: POST /v1/privacy/mask 200 1.2ms request=45B response=32B request_id=abc identity=portal
```

### JSON 格式

```bash
PRIVACY_LOG_FORMAT=json python -m engine.server
```

```json
{
  "timestamp": "2026-07-11T14:30:27.123Z",
  "level": "INFO",
  "logger": "engine.main",
  "message": "POST /v1/privacy/mask 200 1.2ms",
  "request_id": "abc",
  "method": "POST",
  "path": "/v1/privacy/mask",
  "status": 200,
  "duration_ms": 1.2,
  "identity_name": "portal",
  "lineno": 120,
  "funcName": "mask"
}
```

---

## 3. Prometheus 指标抓取

REST 端口（默认 8079）直接访问 `/metrics`：

```bash
curl http://127.0.0.1:8079/metrics
```

K8s ServiceMonitor 示例：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: PrivShield
spec:
  selector:
    matchLabels:
      app: PrivShield
  endpoints:
    - port: rest
      path: /metrics
      interval: 15s
```

---

## 4. Grafana Dashboard 关键面板

### 4.1 核心 Agent 算力层
| 面板 | PromQL |
|---|---|
| QPS | `sum(rate(privacy_requests_total[1m]))` |
| P99 延迟 | `histogram_quantile(0.99, sum(rate(privacy_request_duration_seconds_bucket[5m])) by (le))` |
| 错误率 | `sum(rate(privacy_requests_total{status!~"2.."}[5m])) / sum(rate(privacy_requests_total[5m]))` |
| DP 查询速率 | `sum(rate(privacy_dp_queries_total[1m])) by (mechanism)` |
| 剩余预算 | `privacy_budget_remaining` |
| 拒绝事件 | `sum(rate(privacy_auth_denials_total[1m])) by (reason)` |
| 入站流量 | `sum(rate(privacy_traffic_bytes_total{direction="request"}[1m])) by (path)` |
| 出站流量 | `sum(rate(privacy_traffic_bytes_total{direction="response"}[1m])) by (path)` |

### 4.2 企业级中台微服务群 (Service Hub & 微服务群)
| 面板 | PromQL |
|---|---|
| Service Hub 调度 QPS | `sum(rate(http_requests_total{module="service-hub"}[5m])) by (path)` |
| Service Hub P95 调度延迟 | `histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{module="service-hub"}[5m])) by (le, path))` |
| 上游 Agent 算力调用 QPS | `sum(rate(agent_requests_total{module="service-hub"}[5m])) by (endpoint, status)` |
| 上游 Agent 算力调用延迟 P95 | `histogram_quantile(0.95, sum(rate(agent_request_duration_seconds_bucket{module="service-hub"}[5m])) by (le, endpoint))` |
| 数据源管理探查吞吐 | `sum(rate(http_requests_total{module="datasource-mgr"}[5m])) by (path)` |
| 脱敏审计存证写入吞吐 | `sum(rate(http_requests_total{module="audit-log"}[5m])) by (path)` |

> 预置仪表盘文件：
> - `deploy/grafana/dashboard.json`（全平台联合监控总览大屏）
> - `deploy/grafana/service-hub-dashboard.json`（Service Hub 专属流水线调度大屏）

---

## 5. Jaeger / Tempo Tracing

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4317
export OTEL_SERVICE_NAME=PrivShield
python -m engine.server
```

需先安装可选依赖：

```bash
pip install -e ".[observability]"
```

---

## 6. 审计事件

以下事件会以 warning/error 级别打印日志，并累加 `privacy_auth_denials_total`：

- 认证失败（401 / `UNAUTHENTICATED`）
- 越权（403 / `PERMISSION_DENIED`）
- 限速（429 / `RESOURCE_EXHAUSTED`）

可直接在日志平台搜索：`level:ERROR OR reason:unauthenticated`。