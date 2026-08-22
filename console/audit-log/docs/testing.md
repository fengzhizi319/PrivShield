# 脱敏审计日志 — 测试文档

## 1. 测试概览

| 测试类型 | 文件 | 覆盖范围 |
|---|---|---|
| 单元测试 | `internal/handlers/handlers_test.go` | HTTP handler 层（审计 CRUD + 统计 + 完整性 + 报告） |
| 集成测试 | `console/scripts/integration-test-new-modules.sh` | 端到端审计流程 |

## 2. 运行测试

```bash
# 进入模块目录
cd console/audit-log

# 运行全部测试
export PATH="/Users/charles/go/go1.27.0/bin:$PATH"
go test ./... -v

# 带覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 3. 单元测试清单

### Handler 测试 (`handlers_test.go`)

| 测试用例 | 验证内容 |
|---|---|
| `TestHealth` | GET /api/health 返回 200 + backend=ok + via=audit-log |
| `TestListLogsEmpty` | 初始状态返回 total=0 |
| `TestCreateLog` | POST /api/audit/logs 返回 201 + id |
| `TestCreateLogInvalidBody` | 空 JSON 仍返回 201（无必填字段） |
| `TestGetLogNotFound` | 不存在的 ID 返回 404 |
| `TestGetLog` | 创建后读取，验证 operation/security_level |
| `TestGetStats` | 4 条记录 → total=4, mask=2 |
| `TestListSnapshots` | 创建日志后自动生成快照 |
| `TestVerifyIntegrity` | SHA256 完整性验证返回 valid=true |
| `TestVerifyIntegrityNotFound` | 不存在的快照 ID 返回 404 |
| `TestGenerateReport` | 生成报告：total=5, success_rate=100%, 含建议 |
| `TestListLogsWithFilter` | 按 operation=mask 过滤 |
| `TestComputeIntegrityHash` | 相同输入相同哈希，不同输入不同哈希，64 字符 hex |

## 4. 集成测试

```bash
# 前置条件：三个模块已启动
bash console/scripts/dev-start-new-modules.sh

# 运行集成测试
bash console/scripts/integration-test-new-modules.sh
```

### 集成测试覆盖（audit-log 部分）

- 创建审计记录（含完整字段）
- 读取审计记录详情
- 统计概览（total_operations）
- 快照列表
- 完整性验证（valid=true）
- 合规报告生成（含建议）

## 5. 完整性哈希测试

`computeIntegrityHash` 函数验证：

```go
hash := computeIntegrityHash("log-1", timestamp, "field_mask")
// 输出：64 字符 hex（SHA256）
// 确定性：相同输入 → 相同哈希
// 雪崩效应：不同输入 → 完全不同哈希
```

## 6. 合规报告测试

报告内容验证：

| 字段 | 预期 |
|---|---|
| total_operations | 创建的记录数 |
| success_rate | 100（全部成功） |
| recommendations | 至少 1 条建议 |
| period | 请求的时段 |

## 7. Mock 策略

- Agent 不可达：使用端口 19999，审计功能不依赖 Agent
- 内存存储：每个测试使用 `newTestServer()` 新建实例
- 快照自动生成：创建日志时自动生成快照

## 8. 已知限制

- 内存存储，进程重启后审计记录丢失
- 快照为内存实现，无持久化
- 合规报告为实时计算，非增量聚合
