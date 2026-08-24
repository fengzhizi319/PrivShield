# gateway 可靠性能力说明

> 网关与负载均衡器（gateway）的崩溃恢复、自动重试、完整性校验与备份能力详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| 崩溃恢复 | ⚪ 不适用 | 无状态代理，重启后自动从健康检查恢复后端节点 |
| HTTP 故障重试 | ✅ | 最多 3 次重试，幂等方法或 ConnectError 时触发，指数退避 + 随机抖动 |
| gRPC 故障重试 | ✅ | 最多 3 次重试，UNAVAILABLE 或未知异常时触发，指数退避 + 随机抖动 |
| 主动健康检查 | ✅ | 双协议（HTTP + gRPC）周期性探针，默认 5 秒间隔 |
| 被动健康感知 | ✅ | 请求失败时毫秒级下线节点（5s 冷却退避） |
| 熔断器保护 | ✅ | 节点独立三态熔断器（Closed → Open → Half-Open） |
| 负载均衡 | ✅ | 6 种算法（RR/SWRR/LeastConn/P2C/Random/WeightedRandom） |
| 动态拓扑管理 | ✅ | 运行时 API 注册/注销/隔离/排空/激活后端节点 |
| 备份 | ⚪ 不适用 | 无持久化数据 |
| 优雅停机 | ✅ | FastAPI Lifespan 连接池清理 + gRPC 通道关闭 |

---

## 2. 自动重试（Automatic Retry）

### 2.1 HTTP 代理重试

| 参数 | 值 | 说明 |
|---|---|---|
| 最大重试次数 | 3 | 包含首次调用在内的总尝试次数 |
| 重试条件（幂等） | GET/HEAD/OPTIONS | 幂等方法无条件重试 |
| 重试条件（非幂等） | ConnectError only | 仅 TCP 连接未建立时重试（请求未到达后端，无副作用） |
| 非幂等超时/响应失败 | 不重试 | 防止产生重复副作用 |

**重试流程：**

```
请求到达 → 选择健康节点 → 转发请求
  → 成功：返回响应，反馈熔断器
  → 失败（可重试）：标记节点不健康(5s冷却) → 指数退避+抖动 → 选择下一节点重试
  → 失败（不可重试）：立即中断，返回 502
  → 重试耗尽：返回 502（不暴露后端真实错误）
```

**退避 + 抖动策略（#6）：**

每次重试失败后，等待指数增长的退避时间 + 随机抖动，避免重试风暴：

| 重试次数 | 基础退避 | 实际等待范围 |
|---|---|---|
| 第 1 次 | 0.1s | 0.1s ~ 0.15s |
| 第 2 次 | 0.2s | 0.2s ~ 0.3s |
| 第 3 次 | 0.4s | 0.4s ~ 0.6s |

> 上限封顶 2.0s，抖动为退避值的 0~50% 随机偏移。

### 2.2 gRPC 代理重试

| 参数 | 值 | 说明 |
|---|---|---|
| 最大重试次数 | 3 | 包含首次调用在内的总尝试次数 |
| 重试条件 | `UNAVAILABLE` | 后端不可达（连接重置、服务未启动等） |
| 业务异常 | 不重试 | `INVALID_ARGUMENT`、`NOT_FOUND` 等直接透传 |
| 未知异常 | 重试 | 视为节点故障，触发故障转移 |

---

## 3. 健康检查（Health Checking）

### 3.1 主动健康检查

后台守护任务 `health_check_loop()` 周期性对所有注册节点执行双协议探针：

**检查流程（每 5 秒一轮）：**

```
遍历所有节点 → HTTP GET /health (2s超时)
             → gRPC Health RPC (2s超时)
             → 综合判定：HTTP OK && gRPC OK && 被动冷却已过 → 健康
             → 反馈熔断器（健康 record_success / 不健康 record_failure）
             → 状态变迁时输出日志
             → 刷新 Prometheus 指标
             → sleep(interval)
```

**判定标准（必须同时满足 3 项）：**

1. `is_healthy == True`（主动探针检查通过）；
2. `time.monotonic() >= passive_unhealthy_until`（被动故障冷却已过）；
3. `circuit_breaker.allow_request() == True`（熔断器允许请求通过）。

### 3.2 被动健康感知

请求处理过程中检测到故障时，立即触发被动健康下线：

| 触发条件 | 动作 | 冷却时间 |
|---|---|---|
| HTTP 连接失败/超时 | `mark_unhealthy()` | 5 秒 |
| gRPC UNAVAILABLE | 被动下线 | 5 秒 |
| HTTP 5xx 响应 | 熔断器惩罚 | 不触发被动下线 |

**冷却机制**：被动下线后 5 秒内不参与调度，但主动健康检查仍可将其恢复。

---

## 4. 熔断器保护（Circuit Breaker）

### 4.1 三态状态机

每个后端节点配备独立的熔断器实例，隔离单节点故障：

```
Closed（正常）──连续失败 5 次──→ Open（熔断）
     ↑                              │
     │                         30 秒冷却
     │                              ↓
Half-Open（探测）←──冷却期过─── Half-Open
     │                              │
  探测成功                        探测失败
     ↓                              ↓
  Closed                         Open
```

### 4.2 参数配置

| 参数 | 默认值 | 说明 |
|---|---|---|
| `failure_threshold` | 5 | 触发熔断的连续失败次数 |
| `recovery_timeout` | 30 秒 | 熔断冷却时间，到期后进入半开状态 |

### 4.3 触发条件

| 事件 | 熔断器操作 |
|---|---|
| HTTP 请求成功（< 400） | `record_success()` → 重置为 Closed |
| HTTP 5xx 响应 | `record_failure()` → 累计失败 |
| HTTP 连接失败 | `record_failure()` → 累计失败 |
| gRPC 调用成功 | `record_success()` → 重置为 Closed |
| gRPC UNAVAILABLE | `record_failure()` → 累计失败 |
| 主动健康检查通过 | `record_success()` → 重置为 Closed |
| 主动健康检查失败 | `record_failure()` → 累计失败 |
| 节点重新注册 | `record_success()` → 重置为 Closed |

---

## 5. 负载均衡策略

| 策略 | 算法 | 适用场景 |
|---|---|---|
| `round_robin` | 简单轮询 | 后端实例同构 |
| `weighted_round_robin` | Nginx SWRR 算法 | 后端实例异构（不同权重） |
| `least_connections` | 最少活跃连接 | 长耗时请求场景 |
| `p2c` | Power of Two Choices | 大规模集群，防止羊群效应 |
| `random` | 均匀随机 | 简单场景 |
| `weighted_random` | 加权随机 | 后端实例异构 |

---

## 6. 动态拓扑管理

### 6.1 API 端点

| 端点 | 方法 | 说明 |
|---|---|---|
| `/v1/gateway/register` | POST | 注册或热更新后端节点 |
| `/v1/gateway/deregister` | POST | 注销后端节点并关闭 gRPC 通道 |
| `/v1/gateway/isolate` | POST | 手动隔离节点：强制从调度池排除，不参与任何请求分发 |
| `/v1/gateway/drain` | POST | 排空节点：不再接受新请求，但在途请求可继续完成 |
| `/v1/gateway/activate` | POST | 激活节点：取消隔离或排空状态，恢复正常调度 |

### 6.2 安全防护

- **Fail-Closed 鉴权**：未配置 `GATEWAY_API_KEY` 时接口返回 503 禁用；
- **常量时间比对**：使用 `hmac.compare_digest` 防止时序攻击；
- **SSRF 防护**：`http_url` 必须以 `http://` 或 `https://` 开头；
- **幂等注册**：相同 `(http_url, grpc_address)` 的节点原地更新，自动恢复健康状态；
- **管理操作同样需要鉴权**：isolate/drain/activate 均需 `GATEWAY_API_KEY` Bearer Token。

### 6.3 节点管理状态模型

每个节点维护一个管理状态字段 `admin_state`，影响调度决策：

| 状态 | 含义 | 调度影响 |
|---|---|---|
| `active` | 正常参与调度 | 健康检查通过时正常分配请求 |
| `isolated` | 运维手动隔离 | 完全排除，不分配任何请求 |
| `drained` | 排空中 | 不再分配新请求，但在途请求可完成 |

> 健康节点筛选条件：`is_healthy && 被动冷却已过 && 熔断器允许 && admin_state == "active"`。

---

## 7. TLS 安全体系

### 7.1 南北向（客户端 → 网关）

- 支持可选 TLS/mTLS，通过 `PRIVACY_TLS_ENABLED` 控制；
- mTLS 模式下支持证书指纹（SPKI Pinning）和 CN 白名单。

### 7.2 东西向（网关 → 后端）

- 默认明文回源（同可信内网场景）；
- 通过 `PRIVACY_GATEWAY_BACKEND_TLS_ENABLED=true` 启用 TLS 回源；
- **Fail-Fast CA 校验**：启用回源 TLS 但未配置 CA 路径时立即报错；
- 支持回源 mTLS（需提供客户端证书 + 私钥）。

---

## 8. 优雅停机

### 8.1 停机流程

```
FastAPI shutdown → 关闭 httpx 连接池 → 关闭所有 gRPC 通道 → 进程退出
```

- **HTTP 客户端清理**：`await app.state.http_client.aclose()` 释放连接池；
- **gRPC 通道关闭**：`await balancer.close_all()` 逐个关闭后端 gRPC 通道；
- **节点注销清理**：`remove_node()` 在后台守护线程中异步关闭 gRPC 通道。

---

## 9. 运维建议

### 9.1 部署建议

- 网关本身无状态，建议部署 **≥ 2 个副本** 配合 L4 LB（如 K8s Service）；
- 配置 `GATEWAY_API_KEY` 启用动态拓扑管理 API；
- 生产环境建议启用东西向 TLS 回源；
- 监控 Prometheus 指标：
  - `privacy_gateway_healthy_nodes`：健康节点数（降至 0 时告警）；
  - `privacy_gateway_retries_total`：重试次数（持续增长说明后端不稳定）；
  - `privacy_gateway_requests_total{status="502"}`：502 次数（重试耗尽）；
  - `privacy_gateway_circuit_breaker_state{node}`：熔断器状态（0=closed, 1=open, 2=half_open）；
  - `privacy_gateway_node_admin_state{node}`：管理状态（0=active, 1=isolated, 2=drained）。

### 9.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| 503 No healthy backend | 所有后端节点不健康 | 检查后端 /health 端点和熔断器状态 |
| 502 Bad Gateway | 重试 3 次均失败 | 检查后端网络连通性和日志 |
| 节点频繁上下线 | 健康检查间隔过短或后端不稳定 | 调整 `interval` 参数或排查后端 |
| 熔断器持续 Open | 后端连续失败 | 检查后端服务状态和错误日志 |
| 节点被意外隔离 | 运维操作后忘记恢复 | 检查 `privacy_gateway_node_admin_state` 指标，调用 `/v1/gateway/activate` 恢复 |
