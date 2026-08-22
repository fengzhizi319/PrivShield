# 数据服务调度中枢 — 测试文档

## 1. 测试概览

| 测试类型 | 文件 | 覆盖范围 |
|---|---|---|
| 单元测试 | `internal/handlers/handlers_test.go` | HTTP handler 层 |
| 单元测试 | `internal/config/config_test.go` | 配置加载 |
| 集成测试 | `console/scripts/integration-test-new-modules.sh` | 端到端流水线 |

## 2. 运行测试

```bash
# 进入模块目录
cd console/service-hub

# 运行全部测试
export PATH="/Users/charles/go/go1.27.0/bin:$PATH"
go test ./... -v

# 仅运行 handler 测试
go test ./internal/handlers/ -v

# 仅运行 config 测试
go test ./internal/config/ -v

# 带覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 3. 单元测试清单

### 3.1 Handler 测试 (`handlers_test.go`)

| 测试用例 | 验证内容 |
|---|---|
| `TestHealth` | GET /api/health 返回 200 + backend=ok |
| `TestHubStatus` | GET /api/hub/status 返回模块名称和版本 |
| `TestListTasksEmpty` | 无任务时返回 total=0 |
| `TestDispatchInvalidBody` | 空 JSON body 返回 400 |
| `TestDispatchAccepted` | 合法请求返回 202 + task_id |
| `TestPipeline` | POST /api/tasks/pipeline 返回分类+脱敏结果 |
| `TestLevelToOperation` | L1→none, L2→mask, L3→k_anon, L4→dp, L5→dp |
| `TestLevelToPriority` | L1→low, L3→high, L5→critical |
| `TestListTasksWithFilter` | 按状态过滤任务列表 |

### 3.2 Config 测试 (`config_test.go`)

| 测试用例 | 验证内容 |
|---|---|
| `TestLoadDefaults` | 默认值正确（host/port/agent 等） |
| `TestLoadFromEnv` | 环境变量覆盖默认值 |
| `TestAddress` | Host:Port 拼接正确 |
| `TestAgentBaseURL` | Agent REST base URL 拼接正确 |
| `TestGetEnvIntInvalid` | 非法整数环境变量回退默认值 |
| `TestGetEnvBool` | 布尔环境变量解析 |

## 4. 集成测试

```bash
# 前置条件：三个模块 + Agent 已启动
bash console/scripts/dev-start-new-modules.sh

# 运行集成测试
bash console/scripts/integration-test-new-modules.sh
```

### 集成测试覆盖

- 健康检查（3 模块）
- 任务调度（dispatch → status → list）
- Agent 联动分类脱敏（L3 级别）
- 审计日志记录

## 5. Mock 策略

- Agent 不可达：使用端口 19999（unreachable），验证降级行为
- Mock Agent：使用 `httptest.NewServer` 模拟 Agent 响应
- L1 任务（operation=none）：不依赖 Agent，直接完成

## 6. 已知限制

- 内存任务存储，进程重启后任务丢失
- 并发处理受 `MaxQueueDepth` 限制
- Agent 不可达时 L2+ 任务会失败
