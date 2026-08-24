# 数据服务调度中枢 (Service Hub)

数据服务调度中枢是 PrivShield 控制台的核心调度模块，负责统一管理数据请求的全生命周期：从请求接入、原数取用、分类分级、同机脱敏、跨机存证到安全回传。

## 功能特性

- **请求调度**：统一管理所有数据请求的接入、排队、分发与执行
- **流水线可视化**：实时展示 ①~⑦ 全链路时序状态
- **分类分级联动**：根据数据敏感度等级（L1~L5）自动选择对应脱敏策略
- **任务管理**：查看任务状态、进度、耗时与错误信息
- **上游集成**：与 PrivShield Agent 的分类分级、脱敏等模块无缝对接

## 快速开始

### 开发模式

```bash
cd services/service-hub
bash run.sh
```

默认监听 `http://127.0.0.1:8082`。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `SERVICE_HUB_PORT` | `8082` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 可选认证密钥 |
| `SERVICE_HUB_MAX_QUEUE` | `1000` | 最大任务队列深度 |
| `SERVICE_HUB_SCHEDULE_TIMEOUT` | `30` | 调度超时（秒） |

### 构建

```bash
make build
```

### 测试

```bash
make test
```

### Docker

构建上下文为仓库根目录（需复制共享库 `pkg/`）：

```bash
# 在仓库根目录执行
docker build -f services/service-hub/Dockerfile -t privshield-service-hub .
docker run -p 8082:8082 -e PRIVACY_AGENT_REST_HOST=host.docker.internal privshield-service-hub
```

## API 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查（自身 + 上游 Agent） |
| GET | `/api/hub/status` | 调度中枢状态概览 |
| GET | `/api/hub/tasks` | 任务列表（可选 `?status=` 过滤） |
| POST | `/api/hub/dispatch` | 分发新任务 |
| GET | `/api/hub/pipeline` | 流水线各阶段状态 |
| POST | `/api/hub/classify` | 分类分级 + 自动脱敏分发 |

## 与分级脱敏模块的集成

调度中枢通过 `POST /api/hub/classify` 实现与分级脱敏模块的关键集成：

1. 接收数据后先调用 Agent 的 `/v1/dynclassification/classify` 进行分类分级
2. 根据返回的敏感度等级（L1~L5）自动选择脱敏策略：
   - **L1**（公开）：无需脱敏
   - **L2**（内部）：字段级脱敏
   - **L3**（机密）：K-匿名泛化
   - **L4**（秘密）：差分隐私
   - **L5**（绝密）：差分隐私 + 查询混淆
3. 自动分发到对应的脱敏流水线阶段

## 文档

- [设计文档](docs/design.md)
- [API 参考](docs/api.md)
- [运维手册](docs/ops.md)
