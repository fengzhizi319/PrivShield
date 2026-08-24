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
6. **企业级持久化与安全防护**：基于纯 Go SQLite 实现 WAL 读写分离持久化，配备 API Key 鉴权、常量时间比对防时序攻击与 Prometheus 监控。

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

## 4. 8 要素增强完整性哈希算法

新版实现将所有关键治理要素全面纳入哈希计算：

$$\text{Data} = \text{logID} \parallel \text{timestamp (RFC3339Nano)} \parallel \text{algorithm} \parallel \text{inputHash} \parallel \text{outputHash} \parallel \text{user} \parallel \text{securityLevel} \parallel \text{parametersJSON}$$

$$\text{IntegrityHash} = \text{SHA256}(\text{Data})$$

```go
func computeIntegrityHash(logID string, timestamp time.Time, algorithm, inputHash, outputHash, user, securityLevel, paramsJSON string) string {
    data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%v",
        logID, timestamp.Format(time.RFC3339Nano), algorithm,
        inputHash, outputHash, user, securityLevel, paramsJSON)
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("%x", hash)
}
```

任何微小篡改（甚至是配置 JSON 中的空格或敏感度等级由 L4 改为 L3）都将导致 SHA-256 产生雪崩效应，使 `VerifyIntegrity` 立即识别并报警。
