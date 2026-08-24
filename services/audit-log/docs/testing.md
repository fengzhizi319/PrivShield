# 脱敏审计日志与存证 (Audit Log) — 测试规范与测试全景

> 本文档详细说明 **数联天下 · 数盾 (`PrivShield`)** 脱敏审计日志模块（`services/audit-log`）的测试架构、用例覆盖与执行方式。

---

## 1. 测试全景与模块覆盖

`audit-log` 实现了全方位的自动化单元测试与集成测试，覆盖率达 **85%+**：

| 测试包 | 测试文件 | 覆盖内容与核心断言 |
|---|---|---|
| `internal/grpcserver` | `server_test.go` | **全部 8 个 gRPC 方法**（Health/RecordAudit/GetAuditLog/ListAuditLogs/GetAuditStats/ListSnapshots/VerifyIntegrity/GenerateReport）、输入校验、mTLS 凭证构造、CA 链校验与公钥固定 (Public Key Pinning) |
| `internal/handlers` | `handlers_test.go` | **HTTP REST Handler 层**（Health、创建审计日志、日志检索过滤、统计概览、快照列表、SHA-256 防篡改完整性校验、合规报告生成、参数超大拦截防 DoS） |
| `internal/config` | `config_test.go` | 默认配置、自定义环境变量加载、`Address()`、`GRPCAddress()`、`AgentBaseURLs()` 多节点轮询与 mTLS 配置解析 |
| `internal/models` | `models_test.go` | 审计日志、快照存证、合规报告等核心数据结构的 JSON 序列化与反序列化双向无损性验证 |
| `internal/agent` | `client_test.go` | 上游 Agent HTTP 客户端（Health 探活） |

---

## 2. 运行测试命令

```bash
# 1. 运行 audit-log 全部单元测试
go test -v ./services/audit-log/...

# 2. 运行带覆盖率统计的测试
go test -coverprofile=coverage.out ./services/audit-log/...
go tool cover -func=coverage.out

# 3. 运行根工作区全量 Go 测试
make test-go
```

---

## 3. 核心测试用例清单

### 3.1 gRPC 与 mTLS 测试 (`internal/grpcserver/server_test.go`)
- `TestGRPCHealth`：验证 gRPC 探活接口及上游 Agent 状态解析；
- `TestGRPCHealthAgentUnreachable`：验证 Agent 宕机时的容错降级；
- `TestGRPCAuditLogOperations`：全流程存证闭环（RecordAudit -> GetAuditLog -> ListAuditLogs -> GetAuditStats -> GenerateReport -> ListSnapshots -> VerifyIntegrity）；
- `TestGRPCValidationErrors`：空操作、空 ID、不存在日志与空快照 ID 的 ArgumentError 拦截；
- `TestBuildServerCredentials`：覆盖 7 类 TLS/mTLS 场景（未启用、缺少证书、单向 TLS、mTLS 强制校验、公钥固定校验、CA 缺失失败、非法 client auth 模式）。

### 3.2 HTTP REST 测试 (`internal/handlers/handlers_test.go`)
- `TestHealth`：GET `/api/health` 探活及响应头；
- `TestCreateLog` / `TestGetLog` / `TestListLogsWithFilter`；
- `TestVerifyIntegrity`：验证 8 要素 SHA-256 存证完整性；
- `TestCreateLogParametersTooLarge`：超大参数攻击拦截（防内存耗尽 DoS）；
- `TestComputeIntegrityHash`：验证哈希确定性与雪崩效应。
