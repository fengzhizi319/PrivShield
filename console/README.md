# Privacy Test Console

用于与运行中的 `PrivShield` 进行通信、发送测试数据并验证其全部功能的前端 + 后端测试控制台。

## 目录结构

- `pkg/` - **共享库**（Go），各模块共用的 agent 客户端、配置工具、中间件、存储接口、校验工具与 Prometheus 指标。
- `backend/` - Python FastAPI 代理服务，统一转发请求到 `PrivShield` REST 接口，并提供示例数据。
- `backend-go/` - Go gRPC 代理服务，将前端的 REST 请求转换为 gRPC 调用转发给 `PrivShield`，接口格式与 Python 后端保持一致；同时可直接挂载 `web/dist` 提供 Console UI 页面。
- `web/` - React + TypeScript + Vite 前端，按功能分组展示所有端点，支持一键加载示例和发送请求。
- `service-hub/` - **数据服务调度中枢**（Go），6 阶段调度流水线（ingest→fetch→classify→desensitize→return→audit），自动根据分类分级结果选择脱敏策略。端口 `:8082`。
- `datasource-mgr/` - **数据源管理**（Go），数据源 CRUD、连接测试、元数据自动采集与分类分级。端口 `:8083`。
- `audit-log/` - **脱敏审计日志**（Go），脱敏操作审计记录、SHA256 完整性校验、合规报告生成。端口 `:8084`。

> 📦 **Go Workspace**：`go.work` 统一管理 `pkg`、`backend-go`、`service-hub`、`datasource-mgr`、`audit-log` 五个模块，避免 `replace` 指令散落。

## 文档

两个代理后端各自维护一套 `docs/` 文档（设计 / 接口 / 测试 / 运维）：

- `migration-design.md` - **【架构重构】全平台目录架构重构与平滑迁移设计方案**：中台微服务（service-hub/datasource-mgr/audit-log）解耦与 Monorepo 演进
- `docs/modes.md` - 开发模式 vs 商业化产品模式的整条链路总览：前端、后端、agent 与服务器配置差异
- `docs/vite.md` - Vite 原理、项目结构、配置方式，以及在 `console/web` 中的实际用法
- `backend/docs/` - Python REST 代理后端：[design](backend/docs/design.md) · [api](backend/docs/api.md) · [test](backend/docs/test.md) · [ops](backend/docs/ops.md)
- `backend-go/docs/` - Go gRPC 代理后端：[design](backend-go/docs/design.md) · [api](backend-go/docs/api.md) · [test](backend-go/docs/test.md) · [ops](backend-go/docs/ops.md)
- `service-hub/docs/` - 数据服务调度中枢：[design](service-hub/docs/design.md) · [api](service-hub/docs/api.md) · [prd](service-hub/docs/prd.md) · [testing](service-hub/docs/testing.md) · [ops](service-hub/docs/ops.md)
- `datasource-mgr/docs/` - 数据源管理：[design](datasource-mgr/docs/design.md) · [api](datasource-mgr/docs/api.md) · [prd](datasource-mgr/docs/prd.md) · [testing](datasource-mgr/docs/testing.md) · [ops](datasource-mgr/docs/ops.md)
- `audit-log/docs/` - 脱敏审计日志：[design](audit-log/docs/design.md) · [api](audit-log/docs/api.md) · [prd](audit-log/docs/prd.md) · [testing](audit-log/docs/testing.md) · [ops](audit-log/docs/ops.md)

其中 **ops 运维文档** 说明了开发模式与生产模式的区别、环境变量配置、跨域（CORS）解决方案、启停脚本与常见问题排查，部署前建议先阅读。
如果你想先看“整条 console 链路在开发模式和商业化产品模式下怎么组合”，建议先读 `docs/modes.md`，再按需要查看各组件的 `ops.md`。

> 💡 **运行脚本手册**：`console/scripts/` 目录下全部 21 个 Shell 运行与部署脚本的作用与详细用法，请查阅 [console/scripts/README.md](scripts/README.md)。

## 快速开始

### 1. 一键启动（推荐）

确保已安装 `PrivShield` 和 `console/backend` 的虚拟环境依赖，并已构建前端（`console/web/dist` 存在），然后执行：

```bash
bash ./console/scripts/dev-start-all.sh
```

该脚本会同时启动 `PrivShield` 和测试控制台后端，等待健康检查后输出访问地址，按 `Ctrl+C` 停止所有服务。

### 2. Docker 模式启动 (Docker Container & Docker Compose)

若需要在 Docker 容器环境中一键构建并运行 Backend、Agent 或 LLM 模块，使用 `console/scripts/docker-*.sh` 系列脚本：

```bash
# 1. 单独在 Docker 中运行 Agent (Core / ML 镜像)
bash ./scripts/dev/docker-start-agent.sh [core|ml]

# 2. 启动 vLLM 大模型推理容器 (GPU 加速)
bash ./scripts/dev/docker-start-llm.sh

# 3. 启动 Agent + Go 代理 + React Web UI 容器套件
bash ./console/scripts/docker-start-go.sh

# 4. 启动 Agent + Python 代理 + React Web UI 容器套件
bash ./console/scripts/docker-start-python.sh

# 5. 启动全栈 Docker 容器套件（Agent + 双后端 + Web UI + 可选 vLLM）
bash ./console/scripts/docker-start-all.sh [--with-llm]

# 6. 一键停止并清理所有 Docker 容器服务
bash ./console/scripts/docker-stop.sh
```

### 3. 开发模式与生产静态代理脚本

也可以使用对应的停止脚本（例如在其他终端或 CI 场景中）：

```bash
./console/scripts/dev-stop.sh
```

`dev-stop.sh` 会读取 `console/.pids/` 下记录的 PID 并安全终止 `PrivShield` 与测试控制台后端。

若要通过 **Go gRPC** 后端访问同样的隐私能力，可改用：

```bash
./console/scripts/dev-start-go.sh
```

对应停止脚本：

```bash
./console/scripts/dev-stop.sh
```

该脚本会启动 `PrivShield`（同时监听 REST 与 gRPC）和 `console/backend-go` 中的 Go 代理服务，访问地址为 `http://127.0.0.1:8081`。

若要以 **mTLS 双向认证** 模式运行（Go 代理到 agent 的 gRPC 链路全程加密并互验证书），可执行：

```bash
./console/scripts/dev-start-go-mtls.sh
```

该脚本会在证书缺失时自动调用 `console/backend-go/scripts/gen-certs.sh` 生成一套自签名测试证书，随后同时以 mTLS 模式启动 agent 与 Go 代理。详见 [backend-go/docs/ops.md](backend-go/docs/ops.md) 第 5 节。

若要**同时启动两个后端**（Python REST + Go gRPC），以便在前端顶部 Backend Selector 中随意切换，可执行：

```bash
./console/scripts/dev-start-all.sh
```

对应停止脚本：

```bash
./console/scripts/dev-stop.sh
```

该脚本会同时启动 `PrivShield`（REST 8079 + gRPC 50051）、Python REST 代理后端（`http://127.0.0.1:8080`）与 Go gRPC 代理后端（`http://127.0.0.1:8081`）。打开任一 Console 地址，顶部 Backend Selector 即可在两个后端间自由切换。

### 4. 启动三个新模块（数据服务调度中枢 / 数据源管理 / 脱敏审计日志）

三个新模块均为 Go 服务，需要与 `PrivShield` Agent 联动。可一键启动全部真实服务进行全流程测试：

```bash
# 一键启动 Agent + 三个新模块（自动等待健康检查）
bash ./console/scripts/e2e-start-all-services.sh

# 启动后服务拓扑：
#   PrivShield Agent  → http://127.0.0.1:8079  (分级脱敏引擎)
#   service-hub       → http://127.0.0.1:8082  (调度中枢)
#   datasource-mgr    → http://127.0.0.1:8083  (数据源管理)
#   audit-log         → http://127.0.0.1:8084  (审计日志)

# 停止全部
bash ./console/scripts/e2e-stop-all-services.sh
```

也可以单独启动/停止三个新模块（不启动 Agent）：

```bash
# 启动三个 Go 模块（需 Agent 已在 :8079 运行）
bash ./console/scripts/dev-start-new-modules.sh

# 停止三个 Go 模块
bash ./console/scripts/dev-stop-new-modules.sh
```

### 5. 全流程 E2E 集成测试

启动全部真实服务后，运行端到端集成测试，验证「申请数据 → 分类分级 → 脱敏 → 拿到脱敏数据 → 审计」完整链路：

```bash
# 运行全流程 E2E 测试（真实调用 4 个服务）
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./console/service-hub/internal/handlers/

# 运行三模块集成测试（bash 脚本，curl 调用各服务）
bash ./console/scripts/integration-test-new-modules.sh
```

E2E 测试覆盖：

| 测试用例 | 覆盖流程 |
|---|---|
| `TestRealE2E_FullFlow` | 健康检查 → 注册数据源 → 分类分级 → 自动脱敏 → 审计记录 → 合规报告 |
| `TestRealE2E_AgentDirectCalls` | Agent 直接调用：分类(规则引擎) → 字段脱敏 → 整记录脱敏 |
| `TestRealE2E_MultiServiceCoordination` | 四服务协调：数据源注册 → 脱敏调度 → 审计追踪 → 统计验证 |

> 💡 不带 `PRIVSHIELD_E2E=1` 时 E2E 测试自动跳过，不影响日常开发。

### 6. 手动启动

启动 agent：

```bash
python -m PrivShield.server
```

启动 Python REST 代理后端（默认监听 `127.0.0.1:8080`）：

```bash
cd console/backend
./run.sh
```

启动 Go gRPC 代理后端（默认监听 `127.0.0.1:8081`）：

```bash
cd console/backend-go
go run ./cmd/server
```

### 7. 构建前端

```bash
cd console/web
# WSL 环境推荐使用 corepack pnpm；其它环境也可用 npm install
# 若使用 npm，请将下面命令中的 corepack pnpm 替换为 npm
corepack pnpm install
corepack pnpm build
```

构建产物输出到 `console/web/dist/`，后端会自动挂载为静态资源。

### 8. 打开控制台

- `./console/scripts/dev-start.sh` 启动后访问 `http://127.0.0.1:8080`（Python 后端提供 UI）
- `./console/scripts/dev-start-go.sh` 启动后访问 `http://127.0.0.1:8081`（Go 后端直接提供 UI）
- `./console/scripts/dev-start-all.sh` 启动后两个地址均可访问，顶部 Backend Selector 可随意切换后端

左侧选择功能分组和端点，点击「Send Request」即可测试。

页面顶部的 **Backend Selector** 可以切换后端地址（默认自动选中为当前页面提供服务的后端）：

- `Python REST (8080)` — 使用 Python FastAPI 后端代理，调用 `PrivShield` REST 接口。
- `Go gRPC (8081)` — 使用 Go gRPC 代理后端，将请求通过 gRPC 转发给 `PrivShield`。

每个示例卡片会显示 `backend` 标签（`rest` / `both`），标识该端点在两个后端中的可用性。

## 后端提供的 API

- `GET /api/health` - 检查后端与 agent 的连通性
- `GET /api/samples` - 获取所有端点的示例数据
- `POST /api/proxy` - 通用代理，将请求转发到 `PrivShield`

## 测试

### Python 后端单元测试

`console/backend/tests/` 目录包含基于 `pytest` + `fastapi.testclient.TestClient` 的单元测试，无需启动真实 agent，通过 mock `agent_client.request` 覆盖 `/api/health`、`/api/samples`、`/api/proxy` 等接口：

```bash
cd console/backend
source .venv/bin/activate
pytest tests -v
```

### Go gRPC 代理测试

Go 后端包含单元测试与集成测试：

```bash
cd console/backend-go

# 单元测试（无需 agent）
go test -short ./...

# 全部测试（集成测试需 agent 运行在 127.0.0.1:50051，否则自动跳过）
go test ./...

# 仅集成测试
go test ./tests -v
```

### 三个新模块测试

三个 Go 模块各自包含单元测试，另外提供全流程 E2E 集成测试：

```bash
# 共享库单元测试
cd console && go test ./pkg/... -v

# 各模块单元测试（在 workspace 根目录执行）
cd console && go test ./service-hub/... ./datasource-mgr/... ./audit-log/... -v

# 全流程 E2E 测试（需先启动全部真实服务）
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./console/service-hub/internal/handlers/
```

### 前端构建检查

```bash
cd console/web
corepack pnpm install
corepack pnpm build
```

## 烟雾测试

```bash
cd console/backend
source .venv/bin/activate
python smoke_test.py
```

该脚本会遍历所有示例端点，通过后端代理发送请求并统计结果。需要预存资源的端点（如异步任务查询、复核确认）会被跳过。

## 覆盖的隐私功能

- Health / 健康检查
- Masking / 数据脱敏（字段、记录、批量、DataFrame）
- Hash / HMAC 哈希
- DP / 差分隐私（count、sum、mean、histogram、noisy、aggregate、vector、adaptive clip、groupby、chunked、Arrow IPC）
- LDP / 本地差分隐私（二值/类别扰动与估计）
- K-Anonymity / K-匿名（记录、表、DataFrame）
- Query Obfuscation / 查询混淆
- Classification / 数据分类（字段、记录、表、异步、SecretFlow、复核、导出）
- Budget / 隐私预算查询
- Profile / 隐私参数推荐

## 已知限制

- 默认使用 Python REST 后端与 agent 通信；新增 Go gRPC 后端通过 gRPC 支持同样的隐私原语（ Masking、Hash、DP、LDP、K-Anonymity、Query Obfuscation、Classification、Profile 等），但 `/livez`、`/readyz`、`/readyz/llm`、`/v1/privacy/budget`、`/v1/privacy/dp/arrow_ipc` 等 REST 专属端点以及部分路径差异端点仅在 Python 后端可用。
- 若 agent 启用了认证或限速，请正确配置 `PRIVACY_AGENT_API_KEY` 或相应环境变量。
- `Arrow IPC` 端点的二进制响应会被后端解析为 JSON 记录后返回。

## 共享库架构 (`pkg/`)

三个 Go 模块（service-hub / datasource-mgr / audit-log）共用 `console/pkg/` 下的基础库，避免代码重复：

| 包 | 职责 |
|---|---|
| `pkg/agent` | 上游 agent HTTP 客户端（熔断器、请求重试、Bearer Token 认证） |
| `pkg/config` | 环境变量读取工具（`EnvString` / `EnvInt` / `EnvBool` / `EnvStringSlice`） |
| `pkg/metrics` | Prometheus 指标收集器（每模块独立 Registry，暴露 `GET /metrics`） |
| `pkg/middleware` | 共享 Gin 中间件：RequestID → StructuredLogger → CORS → Auth |
| `pkg/store` | 存储接口定义（`TaskStore` / `DataSourceStore` / `AuditStore`） |
| `pkg/store/memory` | 内存实现（开发/测试场景，`DB_PATH` 为空时自动回退） |
| `pkg/store/sqlite` | SQLite 实现（生产场景，纯 Go `modernc.org/sqlite` 无 CGO 依赖） |
| `pkg/validation` | 输入校验工具（白名单、端口范围、最大长度） |

## 生产加固特性

三个 Go 模块均已通过生产级加固，主要特性：

| 特性 | 说明 |
|---|---|
| **持久化存储** | `DB_PATH` 环境变量指定 SQLite 路径，空值自动回退内存实现 |
| **结构化日志** | `log/slog` 标准库，支持 JSON/Text 格式，可配置日志级别 |
| **Prometheus 指标** | `GET /metrics` 暴露 HTTP/Agent 请求指标 |
| **API Key 鉴权** | `*_API_KEY` 环境变量启用，`/health` 豁免，防时序攻击 |
| **CORS 配置** | `*_CORS_ORIGINS` 逗号分隔白名单，空值降级 `*` |
| **输入校验** | 白名单校验 operation / type / security_level / status / port |
| **请求追踪** | X-Request-ID 中间件自动注入，传递至上游 agent |
| **熔断器** | 上游 agent 调用内置熔断（5 次失败熔断，30s 冷却） |
| **增强完整性哈希** | 审计日志 SHA256 包含 8 个字段（logID/timestamp/algorithm/inputHash/outputHash/user/securityLevel/params） |
