# 数据源管理 — 测试文档

## 1. 测试概览

| 测试类型 | 文件 | 覆盖范围 |
|---|---|---|
| 单元测试 | `internal/handlers/handlers_test.go` | HTTP handler 层（CRUD + 元数据 + 审计） |
| 集成测试 | `console/scripts/integration-test-new-modules.sh` | 端到端数据源管理 |

## 2. 运行测试

```bash
# 进入模块目录
cd console/datasource-mgr

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
| `TestHealth` | GET /api/health 返回 200 + backend=ok + via=datasource-mgr |
| `TestListDataSourcesEmpty` | 初始状态返回 total=0 |
| `TestCreateDataSource` | POST /api/datasources 返回 201 + id + via |
| `TestCreateDataSourceInvalidBody` | 空 JSON `{}` 返回 400 |
| `TestGetDataSourceNotFound` | 不存在的 ID 返回 404 |
| `TestDeleteDataSourceNotFound` | 不存在的 ID 删除返回 404 |
| `TestCreateAndGetAndDelete` | 完整 CRUD 生命周期（创建→读取→删除→验证已删） |
| `TestGetMetadata` | 获取元数据返回表结构 + 自动分类结果 |
| `TestGetAccessAudit` | 获取访问审计（至少包含 create 记录） |

## 4. 集成测试

```bash
# 前置条件：三个模块已启动
bash console/scripts/dev-start-new-modules.sh

# 运行集成测试
bash console/scripts/integration-test-new-modules.sh
```

### 集成测试覆盖（datasource-mgr 部分）

- 创建数据源（含 tags/security_level）
- 读取数据源详情
- 列表查询（验证 total）
- 获取元数据（含自动分类分级）
- 获取访问审计日志
- 删除数据源

## 5. Mock 策略

- Agent 不可达：使用端口 19999，元数据返回模拟数据（不依赖 Agent）
- 内存存储：测试间隔离，每个测试使用 `newTestServer()` 新建实例
- UUID 生成：使用 `crypto/rand` 真实 UUID

## 6. 测试数据

```json
{
  "name": "测试数据库",
  "type": "database",
  "host": "192.168.1.100",
  "port": 5432,
  "database": "test_db",
  "security_level": "high",
  "tags": ["卫健", "高密"]
}
```

## 7. 已知限制

- 内存存储，进程重启后数据丢失
- 连接测试为模拟实现（不真正连接数据库）
- 元数据为模拟数据（不真正查询数据库 schema）
