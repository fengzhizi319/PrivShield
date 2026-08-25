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
| 熔断器保护 | ✅ | 节点独立三态熔断器（Closed → Open → Half-Open），半开期仅允许一个恢复探测请求 |
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
  探测成功                        探测失败
     ↓                              ↓
  Closed                         Open
```

`Half-Open` 不是恢复后的正常分流状态。冷却期到达后，熔断器只开放一个原子“探测许可证”：第一个经 `select_node()` 实际选中的请求占用该许可证，其他并发请求不会再被分配到这个节点；调度器会选择其他健康节点，或在无备用节点时按无健康节点处理。探测请求成功时 `record_success()` 清空失败计数、释放许可证并转换为 `Closed`；探测失败时 `record_failure()` 释放许可证并重新转换为 `Open`，开始新的冷却窗口。候选节点筛选只检查许可证是否空闲，最终选中节点时才占用它，避免纯筛选操作错误消耗探测名额。

这一限制防止后端刚从故障中恢复时同时接收大量请求，形成恢复探测洪峰并立即再次过载。代价是单节点、无备用节点的熔断恢复期最多只有一个在途请求能够验证恢复，其余请求短暂收到 503/UNAVAILABLE；对于保护依赖服务和保持故障域隔离而言，这是刻意的可用性取舍。该行为由 `tests/gateway/test_balancer_unit.py` 覆盖：首个半开请求可取得许可证，第二个请求被拒绝；已占用探测位的节点不会再次被负载均衡器选中。

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

### 4.4 韧性与性能边界

熔断器的状态判断与半开许可证占用均在节点级线程锁内完成，节点池选择过程另由调度锁串行化，因此不会因多个协程同时看到 `Half-Open` 而重复放行恢复探测。正常 `Closed` 状态只增加一次轻量锁检查，不增加代理调用、网络往返或重试次数；只有故障恢复窗口会限制并发。Prometheus 指标 `privacy_gateway_circuit_breaker_state{node}` 可用于观察 `closed=0`、`open=1`、`half_open=2` 的停留时间，持续停留在 `half_open` 或反复在 `open`/`half_open` 间切换应触发后端容量、超时与依赖链路排查。

容量评估应分别压测正常关闭状态、全部节点熔断状态以及单节点半开恢复状态。在半开场景中验证：并发请求不会绕过单探测限制；探测成功后请求恢复正常分流；探测失败后节点重新进入冷却且重试请求能切换到备用节点。指标上应联合观察请求 502/503、`privacy_gateway_retries_total`、健康节点数、熔断器状态停留时间与后端延迟；不要只以单次恢复成功判断网关具备稳定的故障恢复能力。

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

### 5.1 调度引擎职责与数据模型

`engine/gateway/balancer.py` 中的 `LoadBalancer` 只负责**选择可接收新请求的后端节点**，不直接转发 HTTP 或 gRPC 报文。HTTP 代理与 gRPC 代理在每次尝试前调用 `await balancer.select_node()`；选中的 `BackendNode` 随后由代理在 `async with node.track_connection()` 中实际执行回源调用。调用成功或失败后，代理再反馈熔断器、被动健康状态和重试逻辑。因此，调度、转发、故障判定各有独立职责，避免负载均衡器同时承担网络协议处理。

每个 `BackendNode` 持有以下会影响选路的运行时状态：`weight` 是静态配置权重；`current_weight` 是平滑加权轮询的动态累计值；`active_connections` 是代理调用期间的在途连接数；`is_healthy` 是主动健康检查结果；`passive_unhealthy_until` 是请求失败后的单调时钟冷却截止时间；`circuit_breaker` 管理节点的 Closed/Open/Half-Open 状态；`admin_state` 用于 `active`、`isolated`、`drained` 三种运维状态。节点还懒加载并复用 gRPC Channel/Stub，但连接复用不改变调度决策。

节点必须同时满足以下条件，才会进入某次调度候选集：

$$
	ext{routable}(n) = \text{active}(n) \land \text{healthy}(n) \land
(t \ge \text{passive\_unhealthy\_until}(n)) \land
	ext{circuit\_available}(n)
$$

其中 `circuit_available` 对 Closed 节点恒为真；对 Open 节点为假；对 Half-Open 节点只有恢复探测许可证未被占用时为真。候选过滤使用 `CircuitBreaker.is_available()`，不会消耗许可证；最终选中节点才调用 `allow_request()` 原子占用半开许可证。这将“可观察为候选”与“实际允许回源”分开，避免一次未执行的策略比较错误占用恢复机会。

```python
# engine/gateway/balancer.py（语义简化）
healthy = self._get_healthy_nodes_locked(self.nodes)
if not healthy:
  return None

node = choose_by_strategy(healthy)
return node if node.circuit_breaker.allow_request() else None
```

返回 `None` 时，HTTP 代理记录 503 并停止本次转发；gRPC 代理以 `UNAVAILABLE` 中止调用。它不绕过健康检查、熔断或人工排空状态去“强行选择”节点，保持故障隔离的 fail-closed 语义。

### 5.2 并发模型与一致性

调度引擎同时被 HTTP 请求协程、gRPC 请求协程、后台健康检查和管理 API 调用。`_nodes_lock` 是节点池的同步锁，保护 `nodes` 列表、轮询游标 `rr_index` 和 `current_weight`；同步管理方法如 `add_node()`、`remove_node()`、`isolate_node()` 直接持有此锁。异步路径通过 `asyncio.to_thread()` 执行需要同步锁的短临界区，避免在事件循环线程阻塞。

`_selection_lock` 是协程级锁，包围整个 `select_node()`。这使“过滤候选节点 → 更新轮询/平滑权重状态 → 占用半开探测许可证”成为串行可见的调度操作，避免两个协程同时读取同一个 `rr_index`，或同时把同一 Half-Open 节点分配给多个恢复请求。节点自己的 `_state_lock` 则保护健康标记、被动冷却、运维状态和连接计数；`track_connection()` 无论回源成功、失败还是被取消，都会在 `finally` 中减少计数。

这种设计以很短的选路临界区换取一致性。不会在锁内执行 HTTP/gRPC 网络 I/O，也不会在锁内等待后端响应；锁内工作只包括列表过滤、少量整数比较和算法状态更新。代价是单个网关实例的选路操作会串行化：节点数量极大或单实例请求率极高时，`select_node()` 的锁竞争可能成为 CPU 侧瓶颈，应优先横向扩展无状态网关副本，并在目标并发下测量选择延迟，而不是取消一致性保护。

### 5.3 六种选路算法

**Round Robin (`round_robin`)**：以 `rr_index % len(healthy)` 选择节点，再递增游标。对于 $N$ 个同构健康节点，连续 $N$ 次选择会覆盖每个节点一次，时间复杂度为 $O(N)$（健康过滤）加 $O(1)$（选路）。它不考虑连接数、延迟或权重，适合请求耗时和实例性能接近的部署。

```python
node = healthy[self.rr_index % len(healthy)]
self.rr_index = (self.rr_index + 1) % len(healthy)
```

**Smooth Weighted Round Robin (`weighted_round_robin`)**：每次为全部候选节点累加其静态权重，选择 `current_weight` 最大者，然后从该节点扣除候选总权重。设节点权重为 $w_i$，总权重为 $W = \sum_i w_i$，一次选择的更新为：

$$
c_i \leftarrow c_i + w_i, \qquad
k = \arg\max_i c_i, \qquad
c_k \leftarrow c_k - W
$$

与按权重将节点集中连续分配的朴素加权轮询相比，SWRR 会把高权重节点的请求尽量均匀地穿插到时间序列中。例如权重 $5:1$ 的两个节点会得到 `A, A, A, B, A, A`，而不是先连续五个 `A` 再一个 `B`。该算法每次需遍历候选集合，选路部分为 $O(N)$，适用于机器规格或模型推理能力不同、但希望流量平滑的节点池。

**Least Connections (`least_connections`)**：选择 `active_connections` 最小的节点。`track_connection()` 在代理真正开始回源前递增计数，并在 `finally` 中递减，因此计数涵盖 HTTP/gRPC 请求的实际在途时间。选路使用 `min()`，时间复杂度为 $O(N)$。它适用于慢请求、流式传输或不同请求耗时差异明显的场景；但连接数不是 CPU、GPU 显存或请求载荷的完整代理指标，若单个请求资源差异极大，应结合权重拆分节点池或使用外部容量信号。

**Power of Two Choices (`p2c` / `power_of_two_choices`)**：从候选集中随机抽取两个节点，比较归一化负载 $\frac{\text{active\_connections}}{\max(1, \text{weight})}$，选择较小者。其抽样与比较为 $O(1)$，前提仍是前置的 $O(N)$ 候选过滤。相较全量最小连接数，它降低选择成本并在大规模集群中避免所有请求总是向同一最小值节点集中；适合节点较多、长短请求混合的工作负载。

**Random (`random`)**：对健康候选节点均匀随机选择。它没有游标与动态权重状态，适合只需简单概率均衡、实例完全同构且不要求短时间严格公平的场景。

**Weighted Random (`weighted_random`)**：使用 `random.choices(healthy, weights=[node.weight, ...])` 按静态权重随机抽样。长期期望流量比例接近权重比例，但短时间内会有统计波动；相比 SWRR，它无需维护 `current_weight`，但不能保证相邻请求的平滑性。适用于可接受短期波动、重视配置简单性的异构节点池。

### 5.4 节点生命周期与调度联动

`add_node()` 按 `(http_url, grpc_address)` 去重。重复注册不是新增节点，而是原地更新权重、恢复健康状态、清空被动冷却和连接计数，并调用 `record_success()` 复位熔断器；这支持服务发现或控制面重复投递注册事件。新增和更新后都会刷新 `privacy_gateway_healthy_nodes`。

`remove_node()` 先在锁内从候选池移除节点，再在后台守护线程中关闭可能已经建立的 gRPC Channel，因此新请求不会被继续分配，而已经借由 `track_connection()` 发起的请求由其自身调用生命周期完成。`drain_node()` 将 `admin_state` 改为 `drained`，立即停止新分配但不取消在途请求；`isolate_node()` 同时设为 `isolated` 和不健康，用于故障隔离；`activate_node()` 恢复 `active` 并清除被动不健康状态。三者都优先于负载均衡算法，任何策略都不会绕过人工排空或隔离。

### 5.5 代理调用路径与验证

HTTP 与 gRPC 代理共享同一个 `LoadBalancer` 实例，故相同的健康、熔断、排空和连接计数规则同时适用于两个协议。HTTP 对幂等请求及尚未建立 TCP 连接的非幂等请求进行最多三次故障转移；gRPC 对 `UNAVAILABLE` 和未知传输异常进行最多三次故障转移。每一次尝试都会重新调用 `select_node()`，因此已经被动下线、熔断或排空的节点不会在下一次尝试中再次被选中。

核心算法与韧性行为由 `tests/gateway/test_balancer_unit.py` 覆盖：轮询顺序、SWRR 权重分布、最少连接选路、随机加权倾向、健康候选过滤、节点重复注册、动态注销和 Half-Open 单探测限制均有单元测试；`tests/gateway/test_gateway.py` 与 `tests/gateway/test_http_proxy_edge.py` 覆盖 HTTP/gRPC 被动下线、幂等重试和非幂等请求防重复投递。生产变更负载均衡策略或节点权重后，应先在与目标节点数量和请求时长相近的压测环境中观察分布、P99 延迟、重试次数和健康节点数，再逐步发布。

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
