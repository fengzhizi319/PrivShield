# 数盾中台微服务群 (PrivShield Enterprise Services)

企业级数据流通与安全治理中台微服务群，基于 Go 语言构建，提供高吞吐、低延迟的数据资产管理、动态策略调度流水线与不可篡改审计存证能力。

---

## 1. 微服务拓扑与职责

```text
services/
├── service-hub/        # 数据服务调度中枢 (Pipeline Orchestrator REST :8082 / gRPC :50052)
│   ├── cmd/server/     # 服务启动入口
│   ├── internal/       # 调度引擎、上游 Agent 客户端、HTTP Handler
│   ├── proto/          # 调度中枢 gRPC 定义
│   └── docs/           # 模块设计、PRD、接口与测试文档
│
├── datasource-mgr/     # 数据源与资产管理微服务 (Datasource Manager REST :8083 / gRPC :50053)
│   ├── cmd/server/     # 服务启动入口
│   ├── internal/       # 数据源 CRUD、敏感特征探查、分类绑定
│   └── docs/           # 模块设计、PRD、接口与测试文档
│
└── audit-log/          # 脱敏审计与不可篡改存证微服务 (Audit Log REST :8084 / gRPC :50054)
    ├── cmd/server/     # 服务启动入口
    ├── internal/       # SHA-256 哈希链存证、合规报告生成、查询
    └── docs/           # 模块设计、PRD、接口与测试文档
```

---

## 2. 微服务详细介绍

### 2.1 数据服务调度中枢 (`service-hub` REST :8082 / gRPC :50052)
* **核心职责**：实现 6 阶段自动化调度流水线（`Ingest` ➔ `Fetch` ➔ `Classify` ➔ `Desensitize` ➔ `Return` ➔ `Audit`）；
* **与 Agent 联动**：自动请求 PrivShield Agent 进行字段安全级别判定并匹配执行脱敏算子；
* **高可用保障**：内置熔断器、重试队列与背压保护机制；
* **崩溃恢复与自动重试**：启动时自动回收孤立任务（running 标记失败、pending 保留队列），周期性后台重试失败任务（指数退避 + RetryCount）；
* **完整性校验与备份**：启动时 `PRAGMA integrity_check` 阻断损坏数据库，统一备份脚本支持全量/增量/验证模式；
* **HTTP/gRPC 双协议 mTLS**：共享 `pkg/tlsutil` 工具库，TLS 1.3 + 公钥固定；
* 📖 学习与设计文档：[学习指南](service-hub/docs/learning-guide.md) · [设计文档](service-hub/docs/design.md) · [可靠性能力](service-hub/docs/reliability.md)

### 2.2 数据源管理 (`datasource-mgr` REST :8083 / gRPC :50053)
* **核心职责**：管理结构化与半结构化数据源连接（MySQL, PostgreSQL, ClickHouse, Hive, CSV, API）；
* **特征探查**：自动探查数据源元数据，批量采样并联动 PrivShield 进行分类分级打标；
* **资产目录**：提供企业级数据资产目录与敏感字段分布视图；
* **HTTP/gRPC 双协议 mTLS**：共享 `pkg/tlsutil` 工具库，TLS 1.3 + 公钥固定；
* 📖 学习与设计文档：[学习指南](datasource-mgr/docs/learning-guide.md) · [设计文档](datasource-mgr/docs/design.md) · [可靠性能力](datasource-mgr/docs/reliability.md)

### 2.3 脱敏审计日志 (`audit-log` REST :8084 / gRPC :50054)
* **核心职责**：记录全链路所有脱敏与隐私计算操作；
* **不可篡改存证**：采用 SHA-256 包含 8 维度字段（`logID`, `timestamp`, `algorithm`, `inputHash`, `outputHash`, `user`, `securityLevel`, `params`）进行链式哈希计算；
* **合规报告**：支持按时间跨度、部门、数据源生成数据安全审计与合规评估报告；
* **完整性校验**：启动时 `PRAGMA integrity_check` + HMAC-SHA256 签名审计日志 + 独立校验脚本；
* 📖 学习与设计文档：[学习指南](audit-log/docs/learning-guide.md) · [设计文档](audit-log/docs/design.md) · [可靠性能力](audit-log/docs/reliability.md)

---

## 3. 运行与集成

### 3.1 本地协同运行
各微服务依赖根目录共享库 [pkg/](../pkg/)，在根目录的 [go.work](../go.work) 协同下运行：

```bash
# 启动三大微服务 (需 PrivShield Agent 已在 :8079 运行)
bash ./scripts/dev/dev-start-new-modules.sh

# 停止微服务
bash ./scripts/dev/dev-stop-new-modules.sh
```

### 3.2 自动化测试

```bash
# 运行全部微服务单元测试
make test-services

# 运行微服务 + 共享库 + BFF 全套 Go 测试
make test-go

# 运行真实端到端集成测试
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/
```
