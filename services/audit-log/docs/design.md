# 脱敏审计日志与存证 (Audit Log) — 详细设计文档

> 本文档定义 **数联天下 · 数盾 (`PrivShield`)** 脱敏审计日志模块（`services/audit-log`）的系统架构、双协议服务模型（REST + gRPC）、mTLS 双向认证与公钥固定、8 要素增强完整性校验、不可篡改存证与高可用持久化设计。

---

## 1. 背景与业务定位

在国家数据安全法与等保合规要求下，数据流通全链路必须具备**「可追溯、防篡改、抗抵赖」**的审计存证能力。**脱敏审计日志与存证服务 (Audit Log)** 作为独立的安全审计节点，承担着以下核心职责：

1. **双协议接入（REST + gRPC）**：对外提供标准 HTTP REST API 供前端控制台访问，同时对内提供高性能 gRPC 接口（端口 `:50054`）供 `service-hub` 调度中枢与微服务集群直接写入审计存证；
2. **零信任 mTLS 与公钥固定**：gRPC 通道支持 TLS 1.3 双向证书认证（mTLS），并内置客户端公钥固定（Public Key Pinning）机制，彻底防范中间人篡改；
3. **8 要素增强防篡改哈希存证**：采用 SHA-256 密码学哈希对审计事件全字段签名，自动生成存证快照；
4. **在线完整性校验与对账**：提供存证真实性核验端点，实时检测并告警任何底层数据篡改；
5. **SQL 级多维统计与合规报告**：基于 SQLite SQL 聚合引擎提供毫秒级操作概览（`GetStats`）与合规报告（`GenerateReport`），彻底杜绝内存加载溢出风险；
6. **企业级持久化与安全防护**：基于纯 Go SQLite 实现 WAL 读写分离持久化，配备 API Key 鉴权、常量时间比对防时序攻击与 Prometheus 监控；
7. **完整性校验与备份**：启动时 `PRAGMA integrity_check` 阻断损坏数据库，统一备份脚本支持全量/增量备份；
8. **独立校验脚本**：`scripts/prod/verify_audit.sh`（封装 `engine/privacy/verify_audit.py`）独立验证审计数据完整性，支持 CI/CD 集成。

> 📖 **可靠性能力详解**：[docs/reliability.md](docs/reliability.md)

---

## 2. 总体架构拓扑

```mermaid
graph TD
    subgraph Clients [审计核验与调用方]
        WebConsole[React 前端审计看板<br/>:5173]
        GatewayBFF[Go BFF 网关<br/>:8081]
        ServiceHub[Service Hub 调度中枢<br/>:8082]
        Auditor[局方安全审计员<br/>专用只读通道]
    end

    subgraph AuditLogService [Audit Log 微服务 :8084 / :50054]
        HTTPRouter[Gin REST 路由层<br/>/api/audit/* :8084]
        GRPCRouter[gRPC Server :50054<br/>mTLS + Key Pinning]
        MiddlewareStack[共享中间件链<br/>Auth / RequestID / Logger / Recovery / CORS / MaxBodySize]
        PromMetrics[Prometheus Collector<br/>/metrics]

        AuditController[审计业务控制器]
        IntegrityEngine[8要素 SHA-256 校验引擎]
        ReportGenerator[SQL 级合规报告生成器]

        AuditStore[(AuditStore 引擎<br/>SQLite / Memory)]
    end

    WebConsole -->|HTTP/JSON| HTTPRouter
    GatewayBFF -->|HTTP/JSON| HTTPRouter
    Auditor -->|只读核验| HTTPRouter
    ServiceHub -->|gRPC mTLS :50054| GRPCRouter

    HTTPRouter --> MiddlewareStack
    MiddlewareStack --> AuditController
    GRPCRouter --> AuditController
    HTTPRouter --> PromMetrics

    AuditController --> IntegrityEngine
    AuditController --> ReportGenerator
    AuditController --> AuditStore
    IntegrityEngine --> AuditStore
    ReportGenerator --> AuditStore
```

---

## 3. mTLS 与防篡改存证流程

```mermaid
sequenceDiagram
    autonumber
    participant Pipeline as Service Hub 调度流水线
    participant AuditGRPC as audit-log gRPC (:50054)
    participant Engine as 8要素哈希引擎
    participant Store as SQLite WAL 存储
    participant Auditor as 安全审计员 / 控制台

    Pipeline->>AuditGRPC: gRPC TLS 1.3 握手 (客户端 X.509 证书 + 公钥固定校验)
    AuditGRPC->>AuditGRPC: 验证 Client CA 与公钥 Pinning
    Pipeline->>AuditGRPC: RecordAudit(op="mask", in_hash, out_hash, params, user, level="L4")
    AuditGRPC->>Engine: 计算 8 要素 SHA-256 完整性哈希
    Engine-->>AuditGRPC: integrity_hash = SHA-256(...)
    AuditGRPC->>Store: SaveLog(...) & SaveSnapshot(...)
    AuditGRPC-->>Pipeline: 返回 audit_id, success=true

    Auditor->>AuditGRPC: VerifyIntegrity(snapshot_id)
    AuditGRPC->>Store: GetSnapshot(id)
    AuditGRPC->>Engine: 重算当前数据的 SHA-256 哈希
    Engine-->>AuditGRPC: 比较 computed_hash 与存证 hash
    alt 哈希完全一致
        AuditGRPC-->>Auditor: valid=true (未发生篡改，抗抵赖存证有效)
    else 哈希不一致
        AuditGRPC-->>Auditor: valid=false (数据遭受篡改，触发安全告警)
    end
```

---

## 4. 连续防篡改哈希链 (Hash Chain) 与 9 要素密码学完整性

为了彻底防止特权用户物理删除/篡改中间日志行或注入虚假记录，系统引入了**前序哈希链（Blockchain-like Linked Hash Chain）**：

$$\text{Data} = \text{prevHash} \parallel \text{logID} \parallel \text{timestamp (RFC3339Nano)} \parallel \text{algorithm} \parallel \text{inputHash} \parallel \text{outputHash} \parallel \text{user} \parallel \text{securityLevel} \parallel \text{parametersJSON}$$

$$\text{IntegrityHash} = \text{SHA256}(\text{Data})$$

```go
func computeIntegrityHash(logID, prevHash string, timestamp time.Time, algorithm, inputHash, outputHash, user, securityLevel, paramsJSON string) string {
    data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%v",
        prevHash, logID, timestamp.Format(time.RFC3339Nano), algorithm,
        inputHash, outputHash, user, securityLevel, paramsJSON)
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("%x", hash)
}
```

- **创世存证与链式关联**：第一条记录的 `prev_hash` 为空；后续每条记录在生成时自动关联上一条记录的 `integrity_hash`。
- **全链对账核验**：提供 `POST /api/audit/chain/verify` 及 gRPC `VerifyChain` 接口，按时序逐行重算与比对链条连续性。任何删行、篡改或调序均会立即引起雪崩断链并精准定位断点。

---

## 5. 快照样本应用层信封加密 (Envelope Encryption)

快照表（`snapshots`）存储了脱敏前后的数据样本（`input_sample` 与 `output_sample`）。为了彻底避免审计数据库本身成为敏感 PII 泄露源，系统引入了应用层 SM4-GCM 信封加密：

1. **落盘加密**：配置 `AUDIT_LOG_ENCRYPTION_KEY`（或 `PRIVACY_AUDIT_KEY`）后，样本在入库前自动以随机 12-byte Nonce 进行 SM4-GCM 加密，存储格式为 `enc:v1:<base64>`；
2. **透明解密**：仅在经过认证与鉴权的 API 调用方查询快照时，在内存中动态解密呈现；
3. **向后兼容**：未加密的历史遗留样本与未配置密钥环境平滑兼容。

---

## 6. 存储引擎架构（SQLite WAL / PostgreSQL Phase B）

系统支持根据部署规模无缝切换存储引擎底座：

1. **单机/轻量模式 (SQLite WAL)**：
   - 采用纯 Go SQLite 驱动，配置 `PRAGMA journal_mode=WAL` 与 `PRAGMA busy_timeout=5000`；
   - 启动时执行 `PRAGMA integrity_check` 进行损坏防御；
2. **企业级集群模式 (PostgreSQL Phase B)**：
   - 通过配置 `AUDIT_LOG_PG_DSN`（或 `PG_DSN`）启用 `postgres.AuditStore`；
   - 消除 SQLite 单写者锁限制，支持 `audit-log` 多副本水平扩展与超高吞吐并发存证；
   - 支持 `SaveLogsBatch` 批量管道聚合刷盘。

---

## 7. 业务合规存证与基础设施运维日志 (Loki / ELK) 的架构定位辨析

在系统总体架构与运维体系中，必须清晰区分 **「业务级数据脱敏合规存证」** 与 **「基础设施级运行日志聚合 (如 Grafana Loki / ELK)」** 两个维度的概念：

### 7.1 核心差异矩阵

| 维度 | `services/audit-log` (业务存证中台) | Grafana Loki / ELK (运维日志平台) |
|---|---|---|
| **核心定位** | **业务合规与法律证据**（解决“谁在何时对什么数据执行了何种脱敏”的法定合规溯源） | **系统运维与故障排查**（解决“服务是否健康、报错堆栈为何、请求延迟与网络抖动”的 SRE 观测） |
| **存储内容** | 9 要素哈希链、原始数据 SHA-256 哈希、脱敏结果哈希、SM4 密文快照 | 容器与进程标准输出 stdout / stderr 的非结构化/半结构化文本或 JSON |
| **密码学防篡改** | **链式防篡改**（SHA-256 连续哈希链 + 动态核验对账接口，杜绝任何删改） | **不具备**（日志以分块 Chunk 或倒排索引存储，依赖存储介质本身的写保护） |
| **法律合规效力** | 满足《数据安全法》第二十七条、《个人信息保护法》第六十九条与 GDPR 第三十条之规定 | 面向运维与内部分析，通常不具备直接的抗抵赖与司法存证签名能力 |
| **存储底座** | 独立 SQLite WAL 读写分离引擎 / 专用关系型 PostgreSQL 存证库（Append-Only） | 分布式对象存储（S3/MinIO）+ 索引存储（BoltDB/Cassandra/DynamoDB） |
| **查询接口** | 结构化 RESTful API (`:8084`) + 高并发 gRPC 接口 (`:50054`) | LogQL / Kibana 查询语言与 Grafana 仪表盘 |

### 7.2 与集中式日志平台 (Loki / ELK) 的协同集成方案

`services/audit-log` 自身**不依赖 Loki 作为业务存证底座**，但其作为企业级微服务，**完全原生支持接入 Grafana Loki / Promtail / ELK** 作为运维可观测底座：

```mermaid
flowchart TB
    subgraph AuditLogNode [audit-log 微服务节点]
        BusinessEngine[9要素哈希链存证引擎]
        SlogLogger[Go log/slog 结构化日志组件]
        
        TaskStore[(PostgreSQL / SQLite WAL 存证库<br/>Append-Only 业务账本)]
        StdOut[标准输出 stdout / stderr<br/>PRIVACY_LOG_FORMAT=json]
    end

    subgraph BusinessFlow [业务合规面]
        Auditor[安全合规审计员]
        ServiceHub[service-hub 流水线]
        WebConsole[Web 前端大屏]
    end

    subgraph ObservabilityFlow [基础设施运维面]
        Promtail[Promtail / Vector / Fluentd 日志收集 Agent]
        Loki[Grafana Loki 集中日志引擎]
        GrafanaUI[Grafana SRE 监控面板]
    end

    ServiceHub -->|1. 提交存证与快照| BusinessEngine
    BusinessEngine -->|2. 哈希链加密落盘| TaskStore
    Auditor -->|3. 全链核验 / 调取合规报告| BusinessEngine
    WebConsole -->|3. 存证列表查询| BusinessEngine

    BusinessEngine -.->|"运行时记录 (RequestID/耗时/握手)"| SlogLogger
    SlogLogger -->|单行 JSON 日志流| StdOut
    StdOut -->|容器日志抓取| Promtail
    Promtail -->|按标签流式推送| Loki
    Loki -->|LogQL 检索与告警| GrafanaUI
```

1. **结构化 JSON 运行日志**：
   配置 `PRIVACY_LOG_FORMAT=json` 时，微服务的所有运行事件（如 HTTP 请求接收、gRPC mTLS 握手、SQLite 落盘耗时、Slowloris 超时拦截）均由 `pkg/config/logger.go`（基于 Go `log/slog`）输出为单行标准 JSON。
2. **Promtail / Vector 自动采集**：
   在 Kubernetes 或 Docker Compose 部署环境中，宿主机日志收集 Agent 自动抓取容器日志流并追加 `app="audit-log"`, `env="prod"` 标签，投递给 Grafana Loki。
3. **职责分离的零信任原则**：
   - **业务数据存证面**：由 `services/audit-log` 独立负责，保障高防篡改等级与合规报告生成；
   - **运维可观测面**：由 Loki + Prometheus 负责，保障全集群指标与日志的统一告警与链路分析。

---

## 6. SQL 级多维统计与合规报告生成机制

为了防范大批量历史存证导致内存溢出 (OOM)，系统采用 SQL 原生聚合查询实现高性能统计：

```sql
-- 统计总操作数与平均耗时
SELECT COUNT(*), COALESCE(AVG(duration_ms), 0) FROM audit_logs;

-- 按治理算子聚合
SELECT operation, COUNT(*) FROM audit_logs GROUP BY operation;

-- 按数据敏感等级聚合 (L1~L5)
SELECT security_level, COUNT(*) FROM audit_logs GROUP BY security_level;
```

---

## 7. 存储引擎设计与 Append-Only 不可篡改约束

- **SQLite WAL 并发模式**：写操作通过写前日志 (Write-Ahead Log) 顺序追加，读操作完全无锁并发，提供高吞吐存证写入能力。
- **只增不改 (Append-Only) 架构约束**：
  - 数据层接口仅暴露 `SaveLog`、`SaveSnapshot`、`GetLog`、`ListLogs`；
  - 核心业务层不提供任何 `Update` 或 `Delete` 接口，从代码级根绝人为篡改或删除历史存证的可能。
