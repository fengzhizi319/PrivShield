# 数据服务调度中枢 (Service Hub) — 测试规范与用例全集

> 对应模块源码：[services/service-hub](file:///home/charles/code/sfwork/PrivShield/services/service-hub)  
> 模块定位：政务云数据流通调度中枢，负责串联国密 VPN 专线网关、任务流转、敏感度分类分级、动态脱敏处理、存证上链回传 6 阶段流水线。

---

## 1. 测试体系概览

| 测试套件 / 层次 | 测试文件 | 覆盖范围与关键断言 | 覆盖率 |
|---|---|---|:---:|
| **Agent 客户端** | `internal/agent/client_test.go` | 上游健康检查、字段级脱敏、整记录脱敏、动态分类端点调用 | **100.0%** |
| **共享领域模型** | `internal/models/models_test.go` | 敏感度等级到脱敏操作映射 (`LevelToOperation`)、所有核心模型 JSON 序列化与反序列化 | **100.0%** |
| **配置加载器** | `internal/config/config_test.go` | 默认配置、多节点 `AgentBaseURLs` 轮询解析、mTLS 与公钥固定环境变量、生产加固参数 | **100.0%** |
| **HTTP REST 处理器** | `internal/handlers/handlers_test.go` | Health、HubStatus、PipelineStatus、GetTask、ListTasks 分页与状态过滤、Dispatch 边界拦截、Classify 自动编排、API Key 认证、优雅停机 | **83.7%** |
| **gRPC 服务端与 mTLS** | `internal/grpcserver/server_test.go` | Health、HubStatus、Dispatch、ClassifyAndDispatch、GetTask、ListTasks、PipelineStatus、mTLS 证书链生成与校验、公钥比对、流水线异常恢复与停机中断 | **76.8%** |
| **真实跨服务 E2E 流水线** | `internal/handlers/real_e2e_test.go` | 真实 Agent + Service Hub + Datasource Mgr + Audit Log 跨服务 6 阶段完整流水线调度验证 | 条件触发 |

---

## 2. 快速运行测试

```bash
# 1. 运行 service-hub 内部全部单元测试与覆盖率统计
go test -v -cover ./services/service-hub/...

# 2. 仅运行 gRPC 服务端与 mTLS 证书校验测试
go test -v ./services/service-hub/internal/grpcserver/

# 3. 仅运行 HTTP REST 接口与 E2E 模拟流水线测试
go test -v ./services/service-hub/internal/handlers/

# 4. 运行全栈真实 E2E 调度测试（需先启动真实 PrivShield Agent 8079）
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/

# 5. 生成可视化覆盖率 HTML 报告
go test -coverprofile=coverage.out ./services/service-hub/...
go tool cover -html=coverage.out -o coverage.html
```

---

## 3. 详细测试用例清单

### 3.1 HTTP REST 接口测试 (`internal/handlers/handlers_test.go`)

| 测试函数 | 对应接口 / 场景 | 验证内容与防护重点 |
|---|---|---|
| `TestHealth` | `GET /health` / `GET /api/health` | 自身正常 + 上游 Agent 连通性探测（包含可达与不可达降级分支） |
| `TestHubStatus` | `GET /api/hub/status` | 调度中枢运行状态、活跃/排队/完成/失败任务计数汇总 |
| `TestGetTask_SuccessAndNotFound` | `GET /api/hub/tasks/:id` | 正常查询任务详情与不存在 ID 返回 404 Not Found |
| `TestListTasksEmpty` | `GET /api/hub/tasks` | 无任务时返回 `total=0` 且任务列表为空切片 |
| `TestListTasksWithFilter` | `GET /api/hub/tasks?status=...` | 按 `pending` / `running` / `completed` / `failed` 状态精准过滤 |
| `TestListTasks_InvalidStatusFilter` | `GET /api/hub/tasks?status=invalid` | 非法状态参数返回 400 Bad Request 校验错误 |
| `TestDispatchInvalidBody` | `POST /api/hub/dispatch` | 缺失必需字段（`source` 或 `operation`）时返回 400 Bad Request |
| `TestDispatch_OversizedSource` | `POST /api/hub/dispatch` | `source` 字段超出 1024 字符防超大字符串攻击，返回 400 Bad Request |
| `TestDispatchAccepted` | `POST /api/hub/dispatch` | 合法请求立即返回 202 Accepted + 任务 ID，后台异步调度流水线 |
| `TestClassifyAndDispatch_Validations` | `POST /api/hub/classify` | 校验非法 JSON、空 `source` 与超长 `source` 参数防护 |
| `TestAuthMiddleware_Protection` | `pkg/middleware.Auth` | 验证未携带 Token 返回 401 Unauthorized、合法 Bearer 放行及 `/health` 免认证 |
| `TestServer_ShutdownGraceful` | `Server.Shutdown` | 验证停机信号触发 Context 取消并安全等待在途任务 Goroutine 完成 |
| `TestE2E_FullPipeline_DispatchMasking` | 完整脱敏流水线 | 任务进入 `ingest` ➔ `fetch` ➔ `classify` ➔ `desensitize` ➔ `return` ➔ `audit` 6 阶段全周期流转并归档耗时 |
| `TestE2E_FullPipeline_ClassifyAndDesensitize` | 联动分类流水线 | 自动探查数据级别（如 L3）并智能路由至对应的脱敏算子（如 `k_anon`） |
| `TestE2E_FullPipeline_MultiLevelDesensitize` | L1~L4 多级流水线 | L1 (公开/无脱敏)、L2 (内部/字段脱敏)、L3 (敏感/K-匿名)、L4 (机密/差分隐私) 矩阵测试 |

### 3.2 gRPC 服务与 mTLS 测试 (`internal/grpcserver/server_test.go`)

| 测试函数 | 对应 RPC / 场景 | 验证内容与防护重点 |
|---|---|---|
| `TestGRPCServer_Health` | `Health` RPC | 验证自身与上游 Agent 连通性、毫秒延迟与不可达错误捕获 |
| `TestGRPCServer_HubStatus` | `HubStatus` RPC | 验证从存储层读取运行中、排队、完成与失败计数并封装 Proto |
| `TestGRPCServer_Dispatch` | `Dispatch` RPC | 校验空参数、超长参数拦截与成功分发异步任务 |
| `TestGRPCServer_ClassifyAndDispatch` | `ClassifyAndDispatch` RPC | 验证先调用分类探查敏感度等级，再自动分派处理任务 |
| `TestGRPCServer_GetAndListTasks` | `GetTask` & `ListTasks` | 验证单个任务获取、不存在任务 404 NotFound、多任务时间倒序排列与非法状态过滤拦截 |
| `TestGRPCServer_PipelineStatus` | `PipelineStatus` RPC | 统计 6 大流水线阶段的活跃任务分布与上游 Agent 可达性 |
| `TestGRPCServer_ProcessTask_FailureBranches` | 流水线异常与停机 | 验证分类阶段失败、脱敏阶段失败以及服务下线时任务安全标记为 `failed` |
| `TestBuildServerCredentials*` | mTLS 证书体系 | 测试纯明文、单向 TLS、mTLS 双向认证、缺失 CA 报错与非法认证模式拦截 |
| `TestLoadPublicKey` & `TestPublicKeysEqual` | 公钥固定 (Key Pinning) | 测试从 PEM 加载客户端公钥、RSA 公钥相等性比对及防伪造校验 |

### 3.3 共享领域模型测试 (`internal/models/models_test.go`)

| 测试函数 | 验证内容 |
|---|---|
| `TestLevelToOperation` | 严格验证 L1 ➔ `none`、L2 ➔ `mask`、L3 ➔ `k_anon`、L4 ➔ `dp`、L5 ➔ `dp`、未知等级兜底 ➔ `mask` |
| `TestModelSerialization` | 针对 `HubStatus`、`Task`、`PipelineStatus`、`DispatchRequest`、`DispatchResponse`、`ProxyResponse` 进行 JSON 往返序列化一致性测试 |

---

## 4. 模拟与隔离策略 (Mock & Isolation Strategy)

1. **上游 Agent 隔离**：使用 Go 标准库 `httptest.NewServer` 模拟 upstream Agent 的 REST 返回，实现毫秒级纯内存单测；
2. **任务存储隔离**：单元测试使用 `pkg/store/memory.NewTaskStore()` 内存存储，用例间完全物理隔离，测试完毕自动释放；
3. **真实联调隔离**：真实 E2E 测试受 `PRIVSHIELD_E2E=1` 环境变量守卫，避免在离线 CI 或纯单元测试环境中误触发超时。
