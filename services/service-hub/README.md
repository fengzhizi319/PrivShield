# 数据服务调度中枢 (Service Hub)

数据服务调度中枢是 PrivShield 控制台的核心调度微服务，负责统一管理数据请求的全生命周期：从请求接入、原数取用（联动 `datasource-mgr`）、分类分级（联动 `PrivShield Agent`）、动态脱敏、跨机存证（联动 `audit-log`）到安全回传。

---

## 功能特性

- **跨服务联动编排**：自动对接 `datasource-mgr` 抓取医保/康养等模拟数据，并打通 `audit-log` 不可篡改存证；
- **全链路流水线可视化**：实时追踪 `ingest` ➔ `fetch` ➔ `classify` ➔ `desensitize` ➔ `return` ➔ `audit` 六大阶段；
- **分类分级智能调度**：根据数据敏感度等级（L1~L5）自动匹配最适脱敏原语（明文/掩码/K-匿名/差分隐私/查询混淆）；
- **双协议暴露**：同时支持面向 Web 控制台的 HTTP REST (:8082) 与面向高性能内部调用的 gRPC mTLS (:50052)；
- **生产级高可用**：SQLite WAL 持久化、并发信号量防 DoS 击穿、Slowloris 慢连接防御及 Prometheus `/metrics` 监控；
- **崩溃恢复与自动重试**：启动时自动回收孤立任务（running 标记失败、pending 保留队列），周期性后台重试失败任务（指数退避 + RetryCount）；
- **完整性校验与备份**：启动时 `PRAGMA integrity_check` 阻断损坏数据库，统一备份脚本支持全量/增量/验证模式；
- **HTTP/gRPC 双协议 mTLS**：共享 `pkg/tlsutil` 工具库，TLS 1.3 + 公钥固定；
- 📖 **可靠性能力详解**：[docs/reliability.md](docs/reliability.md)

> 📖 **深度学习指南**：完整架构解析、六阶段调度流水线实现与源码导读见 [docs/learning-guide.md](docs/learning-guide.md)。

---

## 快速开始

### 开发模式

```bash
cd services/service-hub
bash run.sh
```

默认监听：
- HTTP REST: `http://127.0.0.1:8082`
- gRPC: `127.0.0.1:50052`

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `SERVICE_HUB_PORT` | `8082` | HTTP 监听端口 |
| `SERVICE_HUB_GRPC_PORT` | `50052` | gRPC 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | 模拟数据源 HTTP 地址 |
| `DATASOURCE_MGR_PORT` | `8083` | 模拟数据源 HTTP 端口 |
| `SERVICE_HUB_TLS_ENABLED` | `false` | 是否开启 gRPC mTLS 双向认证 |

---

## API 接口清单

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查（自身 + 上游 Agent + 下游 Datasource-Mgr） |
| GET | `/api/hub/status` | 调度中枢状态概览 |
| GET | `/api/hub/tasks` | 任务列表（支持分页与 `?status=` 过滤） |
| GET | `/api/hub/tasks/:id` | 获取单个任务详情 |
| POST | `/api/hub/dispatch` | 手动分发新任务到流水线 |
| GET | `/api/hub/pipeline` | 流水线各阶段活跃状态 |
| POST | `/api/hub/classify` | 分类分级 + 自动策略脱敏分发 |
| POST | `/api/hub/pipeline/trigger-datasource` | 申请模拟数据源数据并触发脱敏流水线 |
| GET | `/api/hub/datasources` | 代理列出已接入的模拟数据源列表 |
| GET | `/metrics` | Prometheus 监控指标采集端点 |

---

## 构建与测试

```bash
# 运行单元测试
go test -v ./services/service-hub/...

# 编译二进制
make build
```
