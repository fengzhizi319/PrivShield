# 数据服务调度中枢 (Service Hub) — 测试规范与用例全集

> 对应模块源码：[services/service-hub](file:///home/charles/code/sfwork/PrivShield/services/service-hub)  
> 模块定位：政务云数据流通调度中枢，负责串联模拟数据源、任务流转、敏感度分类分级、动态脱敏处理、存证上链回传 6 阶段流水线。

---

## 1. 测试体系概览

| 测试套件 / 层次 | 测试文件 | 覆盖范围与关键断言 | 覆盖率 |
|---|---|---|:---:|
| **Agent 客户端** | `internal/agent/client_test.go` | 上游健康检查、字段级脱敏、整记录脱敏、动态分类端点调用 | **100.0%** |
| **模拟数据源客户端** | `internal/datasource/client_test.go` | 医保数据（API 1）、康养数据（API 2）、mock3/4 及通用取数、探活与连通性测试 | **100.0%** |
| **共享领域模型** | `internal/models/models_test.go` | 敏感度等级到脱敏操作映射 (`LevelToOperation`)、所有核心模型 JSON 序列化与反序列化 | **100.0%** |
| **配置加载器** | `internal/config/config_test.go` | 默认配置、多节点 `AgentBaseURLs` 轮询解析、Datasource 地址方法、mTLS 与公钥固定环境变量、生产加固参数 | **100.0%** |
| **HTTP REST 处理器** | `internal/handlers/handlers_test.go` | Health、HubStatus、PipelineStatus、GetTask、ListTasks 分页与状态过滤、Dispatch 边界拦截、Classify 自动编排、TriggerDataSourcePipeline、ListDataSources 代理、API Key 认证、优雅停机 | **85%+** |
| **gRPC 服务端与 mTLS** | `internal/grpcserver/server_test.go` | Health、HubStatus、Dispatch、ClassifyAndDispatch、GetTask、ListTasks、PipelineStatus、mTLS 证书链生成与校验、公钥比对、流水线异常恢复与停机中断 | **78%+** |
| **真实跨服务 E2E 流水线** | `internal/handlers/real_e2e_test.go` | 真实 Agent + Service Hub + Datasource Mgr + Audit Log 跨服务 6 阶段完整流水线调度验证 | 条件触发 |

---

## 2. 快速运行测试

```bash
# 1. 运行 service-hub 内部全部单元测试与覆盖率统计
go test -v -cover ./services/service-hub/...

# 2. 仅运行 gRPC 服务端与 mTLS 证书校验测试
go test -v ./services/service-hub/internal/grpcserver/

# 3. 仅运行 HTTP REST 接口与数据源联动流水线测试
go test -v ./services/service-hub/internal/handlers/

# 4. 运行全栈真实 E2E 调度测试（需先启动真实 PrivShield Agent 8079）
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/
```

---

## 3. 详细测试用例清单

### 3.1 HTTP REST 接口测试 (`internal/handlers/handlers_test.go`)

| 测试函数 | 对应接口 / 场景 | 验证内容与防护重点 |
|---|---|---|
| `TestHealth` | `GET /health` / `GET /api/health` | 自身正常 + 上游 Agent + 下游 Datasource-Mgr 连通性探测 |
| `TestHubStatus` | `GET /api/hub/status` | 调度中枢运行状态、活跃/排队/完成/失败任务计数汇总 |
| `TestGetTask_SuccessAndNotFound` | `GET /api/hub/tasks/:id` | 正常查询任务详情与不存在 ID 返回 404 Not Found |
| `TestListTasksEmpty` | `GET /api/hub/tasks` | 无任务时返回 `total=0` 且任务列表为空切片 |
| `TestListTasksWithFilter` | `GET /api/hub/tasks?status=...` | 按 `pending` / `running` / `completed` / `failed` 状态精准过滤 |
| `TestListTasks_InvalidStatusFilter` | `GET /api/hub/tasks?status=invalid` | 非法状态参数返回 400 Bad Request 校验错误 |
| `TestDispatchInvalidBody` | `POST /api/hub/dispatch` | 缺失必需字段（`source` 或 `operation`）时返回 400 Bad Request |
| `TestDispatch_OversizedSource` | `POST /api/hub/dispatch` | `source` 字段超出 1024 字符防超大字符串攻击，返回 400 Bad Request |
| `TestDispatchAccepted` | `POST /api/hub/dispatch` | 合法请求立即返回 202 Accepted + 任务 ID，后台异步调度流水线 |
| `TestClassifyAndDispatch_Validations` | `POST /api/hub/classify` | 校验非法 JSON、空 `source` 与超长 `source` 参数防护 |
| `TestTriggerDataSourcePipeline` | `POST /api/hub/pipeline/trigger-datasource` | 联动 `datasource-mgr` 获取模拟数据并自动分发脱敏流水线 |
| `TestListDataSourcesProxy` | `GET /api/hub/datasources` | 代理列出 `datasource-mgr` 模拟数据源清单 |
| `TestAuthMiddleware_Protection` | `pkg/middleware.Auth` | 验证未携带 Token 返回 401 Unauthorized、合法 Bearer 放行及 `/health` 免认证 |
| `TestServer_ShutdownGraceful` | `Server.Shutdown` | 验证停机信号触发 Context 取消并安全等待在途任务 Goroutine 完成 |
