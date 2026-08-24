# 数据源管理 (Datasource Manager) — 测试规范与测试全景

> 本文档详细说明 **数联天下 · 数盾 (`PrivShield`)** 数据源管理模块（`services/datasource-mgr`）的测试架构、用例覆盖与执行方式。

---

## 1. 测试全景与模块覆盖

`datasource-mgr` 实现了全方位的自动化单元测试与集成测试，覆盖率达 **85%+**：

| 测试包 | 测试文件 | 覆盖内容与核心断言 |
|---|---|---|
| `internal/grpcserver` | `server_test.go` | **全部 11 个 gRPC 方法**（Health/CRUD/TestConnection/GetMetadata/GetRecords/GetAccessAudit/Seed）、输入校验、mTLS 凭证构造、CA 链校验与公钥固定 (Public Key Pinning) |
| `internal/handlers` | `handlers_test.go` | **HTTP REST Handler 层**（Health、CRUD、CSV 安全采样读取、防路径穿越 LFI 攻击测试、超长字段校验、AccessAudit、Seed） |
| `internal/config` | `config_test.go` | 默认配置、自定义环境变量加载、`Address()`、`GRPCAddress()`、`AgentBaseURLs()` 多节点轮询与 mTLS 配置解析 |
| `internal/models` | `models_test.go` | 所有核心数据结构的 JSON 序列化与反序列化双向无损性验证 |
| `internal/agent` | `client_test.go` | 上游 Agent HTTP 客户端（Health 探活、三层动态分类接口联动） |

---

## 2. 运行测试命令

```bash
# 1. 运行 datasource-mgr 全部单元测试
go test -v ./services/datasource-mgr/...

# 2. 运行带覆盖率统计的测试
go test -coverprofile=coverage.out ./services/datasource-mgr/...
go tool cover -func=coverage.out

# 3. 运行根工作区全量 Go 测试
make test-go
```

---

## 3. 核心测试用例清单

### 3.1 gRPC 与 mTLS 测试 (`internal/grpcserver/server_test.go`)
- `TestGRPCHealth`：验证 gRPC 探活接口及上游 Agent 状态解析；
- `TestGRPCHealthAgentUnreachable`：验证 Agent 宕机时的容错降级；
- `TestGRPCDataSourceCRUD`：全生命周期验证（Create -> Get -> List -> Update -> TestConnection -> GetMetadata -> GetAccessAudit -> Delete -> 404 NotFound）；
- `TestGRPCValidationErrors`：非法类型、非法端口、空 ID 的 ArgumentError 拦截；
- `TestGRPCSeedAndCSVOperations`：验证 `yibao.csv` 与 `kangyang.csv` 的预置与明细采样读取；
- `TestBuildServerCredentials`：覆盖 7 类 TLS/mTLS 场景（未启用、缺少证书、单向 TLS、mTLS 强制校验、公钥固定校验、CA 缺失失败、非法 client auth 模式）。

### 3.2 HTTP REST 测试 (`internal/handlers/handlers_test.go`)
- `TestHealth`：GET `/api/health` 探活及响应头；
- `TestListDataSourcesEmpty` / `TestCreateDataSource` / `TestUpdateDataSource`；
- `TestCreateAndGetAndDelete`：完整 REST CRUD 闭环；
- `TestLoadCSVRecords_PathTraversal`：使用 `../../etc/passwd` 触发路径穿越攻击，断言被安全沙箱拦截；
- `TestSeedAndFetchRecords`：预置样例并采样读取。
