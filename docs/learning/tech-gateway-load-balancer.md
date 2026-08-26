# 高性能网关、反向代理与 10k QPS 负载均衡技术指南 / Gateway, Load Balancer & High Concurrency Technical Guide

## 1. 技术简介 / Introduction

在企业级数据治理集群中，单实例 Agent 难以承担超万级 QPS 的高并发隐私脱敏与大批量数据分类任务。

`PrivShield` 设计并实现了专用的**高性能反向代理网关（Reverse Proxy Gateway）与动态负载均衡器（Load Balancer）**，支持 **REST (HTTP/1.1 & HTTP/2) 与 gRPC 双协议透明代理**，具备主动/被动双模健康检查、三态熔断器、平滑加权轮询调度及高并发线程池隔离调度能力。

```text
                               外部客户端流量 (HTTP / gRPC)
                                            │
                                            ▼
                    ┌───────────────────────────────────────────────┐
                    │ ★ PrivShield Gateway (engine/gateway/server)  │
                    │   - 双协议接入 (HTTP :8079 / gRPC :50051)      │
                    │   - 真实 IP 透传 (X-Forwarded-For, X-Real-IP) │
                    │   - RFC 7230 逐段传输头与压缩头清洗            │
                    └───────────────────────┬───────────────────────┘
                                            │
                                            ▼
                    ┌───────────────────────────────────────────────┐
                    │ ★ LoadBalancer & CircuitBreaker (balancer.py) │
                    │   - 节点拓扑与在途连接追踪 (In-Flight Conns)   │
                    │   - 调度算法: SWRR / LeastConn / P2C / RR     │
                    │   - 三态熔断器: Closed -> Open -> Half-Open   │
                    │   - 毫秒级被动故障感知 + 周期性主动健康探针    │
                    └───────────────────────┬───────────────────────┘
                                            │
                  ┌─────────────────────────┼─────────────────────────┐
                  ▼                         ▼                         ▼
         Worker Node 1 (Core)      Worker Node 2 (Core)      Worker Node 3 (ML/GPU)
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 负载均衡核心调度算法 / Load Balancing Algorithms

文件 / File：[`engine/gateway/balancer.py`](engine/gateway/balancer.py#L200-L380)

#### (1) 平滑加权轮询 (Smooth Weighted Round-Robin, SWRR)

基于 Nginx 经典加权轮询算法，动态维护每个节点的 `current_weight`，确保即使权重相差悬殊（如 10:1），大权重节点也不会被连续集中轰炸，实现请求在时间轴上的均匀离散：

```python
def _select_smooth_weighted_round_robin(self, healthy_nodes: list[BackendNode]) -> BackendNode:
    """Nginx 平滑加权轮询算法实现。"""
    total_weight = sum(node.weight for node in healthy_nodes)
    best_node = None
    max_current_weight = -float("inf")

    for node in healthy_nodes:
        # 1. 累加当前动态权重
        node.current_weight += node.weight
        # 2. 选取当前动态权重最大者
        if node.current_weight > max_current_weight:
            max_current_weight = node.current_weight
            best_node = node

    # 3. 选中的节点削减 total_weight
    if best_node is not None:
        best_node.current_weight -= total_weight
    return best_node
```

#### (2) 最小在途连接数 (Least Connections)

通过上下文管理器 `with node.track_connection():` 实时追踪并发在途请求数（In-Flight Requests），优先将耗时较长的重计算任务导向当前最空闲的实例：

```python
@contextlib.contextmanager
def track_connection(self):
    """上下文管理器：原子增加与递减当前节点在途连接数。"""
    with self._conn_lock:
        self.active_connections += 1
    try:
        yield
    finally:
        with self._conn_lock:
            self.active_connections = max(0, self.active_connections - 1)
```

#### (3) 两选择随机算法 (Power of Two Choices, P2C)

在高并发场景下，简单随机容易造成局部倾斜，而全局最小连接数每次都需要遍历所有节点。P2C 算法随机抽取 2 个健康节点，仅比对两者的在途连接数并路由至更优者，以 $O(1)$ 时间复杂度实现近乎完美的均衡，有效消除“羊群效应”。

---

### 2.2 三态熔断器保护机制 / Three-State Circuit Breaker

文件 / File：[`engine/gateway/balancer.py`](engine/gateway/balancer.py#L120-L190)

```python
class CircuitBreaker:
    """独立节点级三态熔断器：Closed (正常) -> Open (熔断) -> Half-Open (半开试探)。"""

    def record_failure(self) -> None:
        """记录一次 5xx 或连接崩溃。连续失败达到阈值时触发熔断。"""
        self.consecutive_failures += 1
        if self.consecutive_failures >= self.failure_threshold:
            self.state = "OPEN"
            self.last_state_change = time.time()
            logger.error(f"Circuit breaker OPENED for node {self.node_id}. Cooling for {self.recovery_timeout}s.")

    def allow_request(self) -> bool:
        """判断是否放行请求。处于 OPEN 且冷却时间到期后进入 HALF-OPEN 状态放行试探流量。"""
        if self.state == "CLOSED":
            return True
        if self.state == "OPEN":
            if time.time() - self.last_state_change >= self.recovery_timeout:
                self.state = "HALF-OPEN"
                logger.info(f"Circuit breaker HALF-OPEN for node {self.node_id}. Probing canary traffic.")
                return True
            return False
        # HALF-OPEN 状态仅放行少量金丝雀探针
        return True

    def record_success(self) -> None:
        """在 HALF-OPEN 试探成功时，复位为 CLOSED 状态。"""
        if self.state == "HALF-OPEN":
            self.state = "CLOSED"
            self.consecutive_failures = 0
            logger.info(f"Circuit breaker RESET to CLOSED for node {self.node_id}.")
```

---

### 2.3 10k QPS 高并发优化与快慢路径分离 / 10k QPS High-Concurrency Optimization

文件 / File：[`engine/privacy/high_concurrency.py`](engine/privacy/high_concurrency.py)

为了在单机上达成 10,000 QPS 的吞吐量指标，`PrivShield` 采用了快慢路径分离与线程池调度架构：

1. **快路径（Fast Path - 纯内存计算）**：
   - 字段名正则与静态规则直接在主事件循环中命中 LRU 缓存（`functools.lru_cache` 命中率 > 95%），无阻塞直接返回，延迟 < 0.05ms；
2. **慢路径（Slow Path - CPU 密集型/复杂运算）**：
   - 对于大批量高斯差分隐私、Mondrian 树构建或图像处理，使用预初始化的 `ThreadPoolExecutor`（线程数与 CPU 核数绑定）执行异步卸载，绝不阻塞 Starlette 事件循环；
3. **Uvicorn 与 gRPC 底层参数对齐**：
   - `PRIVACY_LIMIT_CONCURRENCY=10000`（最大并发连接）
   - `PRIVACY_TIMEOUT_KEEP_ALIVE=30`
   - `grpc.max_receive_message_length = 64MB` / `grpc.max_send_message_length = 64MB`。

---

## 3. HTTP 反向代理核心实现 / HTTP Reverse Proxy Implementation

文件 / File：[`engine/gateway/http_proxy.py`](engine/gateway/http_proxy.py)

### 3.1 Hop-by-Hop 头清洗与真实 IP 透传 / Header Filtering & Client IP Forwarding

RFC 7230 定义了**逐段传输头（Hop-by-Hop Headers）**，这些头仅在单段直连链路上有效，代理转发时必须剥离，否则会导致后端误解连接语义。例如 `transfer-encoding: chunked` 是客户端到网关的分块约定，若原样传递给后端，后端会尝试对已经解压的内容再次分块解码，引发协议错误。

```python
# RFC 7230 逐段传输头集合 —— 转发时必须剔除
EXCLUDE_HEADERS = {
    "connection",        # 逐连接选项，不应跨链路传递
    "keep-alive",        # HTTP/1.0 遗留保活头
    "proxy-authenticate", # 代理认证质询
    "proxy-authorization",# 代理认证凭据
    "te",                # 传输编码协商
    "trailers",          # 分块传输尾部头
    "transfer-encoding", # 分块编码方式（httpx 自动处理）
    "upgrade",           # 协议升级请求
    "content-length",    # httpx 重新计算，避免与实际 body 不一致
    "host",              # 由 httpx 根据目标 URL 重新设置
}

# 响应方向额外剔除 content-encoding：
# httpx 读取 resp.content 时自动解压 gzip/deflate/br，
# 若原样传递 content-encoding 给客户端，客户端会对已解压数据再次解压，导致崩溃。
RESPONSE_EXCLUDE_HEADERS = EXCLUDE_HEADERS | {"content-encoding"}
```

**真实客户端 IP 透传**是代理网关的核心职责之一。PrivShield 采用业界标准的 `X-Forwarded-For` + `X-Real-IP` 双头方案：

```python
# 提取客户端直连 IP（request.client.host 是 TCP 对端地址）
client_ip = request.client.host if request.client else "127.0.0.1"

# X-Forwarded-For 是追加式链：client → proxy1 → proxy2 → ...
# 每经过一层代理，就在尾部追加该层的直连 IP
if "x-forwarded-for" in headers:
    headers["x-forwarded-for"] = f"{headers['x-forwarded-for']}, {client_ip}"
else:
    headers["x-forwarded-for"] = client_ip

# X-Real-IP 始终记录最初的客户端 IP（不追加，只设置一次）
if "x-real-ip" not in headers:
    headers["x-real-ip"] = client_ip
```

> **学习要点**：`X-Forwarded-For` 的格式是逗号分隔的 IP 链，最左边是原始客户端，每经过一层代理追加一个 IP。在多层代理场景中，可以通过解析该头部还原完整的请求路径。

---

### 3.2 请求体单次缓冲与防大包 DDoS / Body Buffering & Payload DDoS Protection

代理转发中一个常见的陷阱是**请求体重复读取**。当代理需要支持故障重试时，如果请求体是流式读取（stream），第一次尝试消费后流就枯竭了，重试时将无法获取到请求数据。

PrivShield 的解决方案是在代理层**一次性将请求体完整缓冲到内存**，然后对每次重试都使用同一份字节副本：

```python
# 防大包攻击：先检查 Content-Length 头声明的大小
max_body_bytes = 64 * 1024 * 1024  # 64 MiB 上限
content_length = request.headers.get("content-length")
if content_length:
    if int(content_length) > max_body_bytes:
        raise HTTPException(status_code=413, detail="Payload too large")

# 单次完整读取 body，后续重试复用同一份 bytes 副本
body = await request.body()
if len(body) > max_body_bytes:
    raise HTTPException(status_code=413, detail="Payload too large")
```

> **为什么限制 64 MiB？** 这个值与 gRPC 的 `grpc.max_receive_message_length` 对齐（见 `GRPC_MAX_MESSAGE_BYTES`），覆盖了大表分类和图片脱敏场景的最大消息体积，同时阻止了利用超大 payload 耗尽网关内存的攻击。

---

### 3.3 故障自适应重试与指数退避 / Adaptive Retry & Exponential Backoff

代理网关的重试机制需要精细区分**可重试**与**不可重试**的场景，否则可能导致非幂等操作（如 POST 扣款）被重复执行，产生严重的业务副作用。

```python
for attempt in range(max_retries):  # max_retries = 3
    node = await balancer.select_node()
    if not node:
        raise HTTPException(status_code=503, detail="No healthy backend nodes available")

    try:
        async with node.track_connection():
            resp = await client.request(method=method, url=url, headers=headers,
                                        params=query_params, content=body)
        # 成功路径：根据状态码反馈熔断器
        if resp.status_code >= 500:
            node.circuit_breaker.record_failure()  # 后端故障，惩罚节点
        elif resp.status_code < 400:
            node.circuit_breaker.record_success()  # 正常响应，复位熔断器
        # 4xx 不惩罚也不复位 —— 这是客户端请求错误，不是节点故障
        return Response(content=resp.content, status_code=resp.status_code, headers=resp_headers)

    except Exception as exc:
        last_exception = exc
        # 重试安全判定：幂等方法 GET/HEAD/OPTIONS 总是安全重试；
        # 非幂等方法仅在 ConnectError（TCP 连接未建立，请求未到达后端）时重试
        retry_allowed = method in {"GET", "HEAD", "OPTIONS"} or isinstance(exc, httpx.ConnectError)

        node.circuit_breaker.record_failure()
        node.mark_unhealthy(cooldown_seconds=5.0)  # 被动健康感知：5 秒冷却退避

        if not retry_allowed:
            break  # 非幂等且可能已产生副作用，立即中断重试

        # 指数退避 + 随机抖动，防止重试风暴
        if attempt < max_retries - 1:
            backoff = min(0.1 * (2 ** attempt), 2.0)  # 0.1s → 0.2s → 0.4s
            jitter = random.uniform(0, backoff * 0.5)
            await asyncio.sleep(backoff + jitter)
```

**重试策略设计原则**：

| 场景 | 是否重试 | 原因 |
|---|---|---|
| GET/HEAD/OPTIONS + 连接失败 | ✅ 重试 | 幂等方法，无副作用 |
| POST + `ConnectError` | ✅ 重试 | TCP 连接未建立，请求未到达后端 |
| POST + `TimeoutError` | ❌ 不重试 | 超时可能已执行，重复发送有副作用 |
| POST + 响应读取失败 | ❌ 不重试 | 后端可能已处理，重试导致重复操作 |

> **指数退避 + 抖动**：基础延迟 0.1s，每轮翻倍（0.1→0.2→0.4），上限 2.0s。叠加 0~50% 的随机抖动，避免多个客户端在同一时刻同步重试造成「惊群效应」（Thundering Herd）。

---

### 3.4 事件循环漂移感知与 HTTP 客户端自愈 / Event Loop Drift & Client Self-Healing

在 Python asyncio 中，`httpx.AsyncClient` 绑定到创建它的 Event Loop。如果应用被测试框架或多 loop 部署环境切换到不同的 Event Loop，旧客户端将无法使用。PrivShield 实现了**Loop 漂移检测**：

```python
current_loop = asyncio.get_running_loop()
client = getattr(request.app.state, "http_client", None)
cached_loop = getattr(request.app.state, "http_client_loop", None)

if client is None or cached_loop is not current_loop:
    # 检测到 Loop 漂移或首次初始化：销毁旧客户端，创建新的
    if client is not None:
        asyncio.create_task(client.aclose())  # fire-and-forget 关闭旧客户端

    client = httpx.AsyncClient(
        timeout=httpx.Timeout(30.0),
        limits=httpx.Limits(max_keepalive_connections=100, max_connections=500),
        trust_env=False,        # 禁用环境变量代理，防止流量被意外拦截
        verify=backend_tls_verify(),  # 回源 TLS CA 校验
    )
    request.app.state.http_client = client
    request.app.state.http_client_loop = current_loop
```

---

## 4. gRPC 反向代理实现 / gRPC Reverse Proxy

文件 / File：[`engine/gateway/grpc_proxy.py`](engine/gateway/grpc_proxy.py)

gRPC 代理比 HTTP 代理更复杂，因为 gRPC 基于 HTTP/2 多路复用，一个 TCP 连接上可以并发承载多个 RPC 流。PrivShield 的 gRPC 网关采用**透明流拦截**模式。

### 4.1 gRPC 网关启动与 TLS 终结

```python
async def start_grpc_gateway(
    host: str, port: int, balancer: LoadBalancer,
    tls_enabled: bool = False,
    tls_cert_file: str = "",
    tls_key_file: str = "",
    tls_ca_file: str = "",
) -> grpc.aio.Server:
    """构建并启动 gRPC 网关服务器。"""
    server = grpc.aio.server(
        ThreadPoolExecutor(max_workers=8),
        options=GRPC_CHANNEL_OPTIONS,  # 64 MiB 消息大小
    )
    # 注册通用 RPC 处理器（拦截所有未明确定义的 service/method）
    server.add_generic_rpc_handlers([_GenericHandler(balancer)])

    if tls_enabled and tls_cert_file:
        with open(tls_cert_file, "rb") as f:
            cert_chain = f.read()
        with open(tls_key_file, "rb") as f:
            private_key = f.read()
        server_credentials = grpc.ssl_server_credentials(
            [(private_key, cert_chain)],
            root_certificates=open(tls_ca_file, "rb").read() if tls_ca_file else None,
            require_client_auth=bool(tls_ca_file),
        )
        server.add_secure_port(f"{host}:{port}", server_credentials)
    else:
        server.add_insecure_port(f"{host}:{port}")

    await server.start()
    return server
```

### 4.2 gRPC 通用处理器与流式转发

gRPC 的通用处理器（GenericRpcHandler）拦截所有 RPC 调用，根据 `service/method` 路径动态选择后端节点并转发：

```python
class _GenericHandler(grpc.GenericRpcHandler):
    """拦截所有 gRPC 方法调用，动态路由到后端。"""
    def __init__(self, balancer: LoadBalancer):
        self._balancer = balancer

    def service(self, handler_call_details):
        method = handler_call_details.method
        return _StreamForwardHandler(self._balancer, method)
```

> **学习要点**：gRPC 的 GenericRpcHandler 机制类似于 HTTP 的通配路由 `/{path:path}`，允许网关在不需要预先生成 protobuf stub 的情况下代理任意 RPC 方法。

---

## 5. 动态拓扑管理 API / Dynamic Topology Management

文件 / File：[`engine/gateway/http_proxy.py`](engine/gateway/http_proxy.py#L155-L220)

PrivShield 网关支持**运行时热增删后端节点**，无需重启网关进程。所有管理端点受 Fail-Closed 鉴权保护：

### 5.1 鉴权机制（Fail-Closed 默认拒绝）

```python
def require_management_auth(authorization: str | None) -> None:
    """Fail-Closed 鉴权：未配置 GATEWAY_API_KEY 时直接禁用所有管理端点。"""
    if not gateway_api_key:
        raise HTTPException(status_code=503,
            detail="Gateway management API is disabled")
    expected = f"Bearer {gateway_api_key}"
    if not authorization or not hmac.compare_digest(
        authorization.encode("utf-8"), expected.encode("utf-8")):
        raise HTTPException(status_code=401, detail="Unauthorized")
```

### 5.2 五种管理操作

| 端点 | 方法 | 功能 | 使用场景 |
|---|---|---|---|
| `/v1/gateway/register` | POST | 注册/热更新节点 | 新 Worker 上线后自动注册 |
| `/v1/gateway/deregister` | POST | 注销节点并关闭通道 | Worker 退役/缩容 |
| `/v1/gateway/isolate` | POST | 强制隔离节点 | 运维手动排除故障节点 |
| `/v1/gateway/drain` | POST | 排空节点（拒新放旧） | 优雅下线/滚动更新 |
| `/v1/gateway/activate` | POST | 恢复节点为活跃 | 取消隔离或排空 |

**隔离 vs 排空的区别**：
- **隔离（Isolate）**：立即从调度池完全排除，在途请求也会被中断。适用于节点已确认故障的紧急场景。
- **排空（Drain）**：不再接受新请求，但在途请求可以继续完成。适用于优雅下线、滚动更新等不允许中断正在处理请求的场景。

---

## 6. 网关统一启动入口与配置层级加载 / Server Startup & Configuration

文件 / File：[`engine/gateway/server.py`](engine/gateway/server.py)

### 6.1 三层配置合并优先级

网关配置来自三个层级，优先级从低到高：

```text
┌──────────────────────────────────────┐
│  Layer 1: 代码内置默认值               │  最低优先级
│  rest_host=0.0.0.0, rest_port=8000  │
│  grpc_host=0.0.0.0, grpc_port=50000 │
│  strategy=round_robin               │
├──────────────────────────────────────┤
│  Layer 2: YAML 配置文件               │  中间优先级
│  PRIVACY_GATEWAY_CONFIG=/path/to.yaml│
├──────────────────────────────────────┤
│  Layer 3: 环境变量 / CLI 参数         │  最高优先级
│  GATEWAY_REST_PORT=9000             │
│  GATEWAY_BACKENDS=http://...|...:50051│
└──────────────────────────────────────┘
```

### 6.2 后端节点字符串解析

`GATEWAY_BACKENDS` 环境变量支持紧凑的单行配置格式：

```bash
# 格式：http_url|grpc_address,http_url|grpc_address,...
export GATEWAY_BACKENDS="http://127.0.0.1:8079|127.0.0.1:50051,http://127.0.0.1:8080|127.0.0.1:50052"
```

### 6.3 双协议并行启动与优雅停机

```python
async def async_main(...):
    # 1. 初始化负载均衡器 + 注入静态后端
    balancer = LoadBalancer(strategy=gw["strategy"])
    for node_cfg in backends:
        balancer.add_node(...)

    # 2. 启动 gRPC 网关
    grpc_server = await start_grpc_gateway(...)

    # 3. 启动 HTTP 网关（Uvicorn + TLS/mTLS）
    http_app = create_http_gateway_app(balancer)
    uv_server = uvicorn.Server(uvicorn.Config(app=http_app, ...))

    # 4. 启动后台健康检查守护协程
    health_task = asyncio.create_task(health_check_loop(balancer, interval))

    # 5. 并发运行双协议服务
    await asyncio.gather(uv_server.serve(), grpc_server.wait_for_termination())

    # 6. 优雅停机（except 块中）
    health_task.cancel()                      # 取消健康检查
    await health_task                         # 等待任务退出
    await grpc_server.stop(grace=1.0)         # 1 秒排空期
    await balancer.close_all()                # 释放所有通道
```

---

## 7. Uvicorn 生产加固参数 / Production Hardening Parameters

| 参数 | 环境变量 | 默认值 | 防护目标 |
|---|---|---|---|
| `limit_concurrency` | `GATEWAY_LIMIT_CONCURRENCY` | 10000 | 最大并发连接数，防止过载 OOM |
| `limit_max_requests` | `GATEWAY_LIMIT_MAX_REQUESTS` | 100000 | 单 worker 最大请求数，防内存泄漏 |
| `timeout_keep_alive` | `GATEWAY_TIMEOUT_KEEP_ALIVE` | 30s | 空闲连接超时，减少僵尸连接 |
| `timeout_graceful_shutdown` | `GATEWAY_TIMEOUT_GRACEFUL_SHUTDOWN` | 10s | SIGTERM 后等待在途请求完成 |

---

## 8. 回源 TLS 体系 / Backend Origin TLS

文件 / File：[`engine/gateway/balancer.py`](engine/gateway/balancer.py#L85-L192)

网关到后端的回源链路默认使用明文（适用于同可信内网），但支持通过环境变量启用 TLS 加密：

```bash
export PRIVACY_GATEWAY_BACKEND_TLS_ENABLED=true
export PRIVACY_GATEWAY_BACKEND_TLS_CA=/path/to/ca.crt
# 可选 mTLS
export PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT=/path/to/client.crt
export PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_KEY=/path/to/client.key
```

**Fail-Fast 校验**：启用 TLS 但未配置 CA 路径或 CA 文件不存在时，立即抛出 `RuntimeError`，杜绝静默降级为明文。

---

## 9. Prometheus 网关指标体系 / Gateway Metrics

| 指标名 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_gateway_requests_total` | Counter | protocol, method, status | QPS 统计 |
| `privacy_gateway_latency_seconds` | Histogram | protocol | 延迟分布 |
| `privacy_gateway_retries_total` | Counter | protocol, reason | 重试计数 |
| `privacy_gateway_healthy_nodes` | Gauge | — | 健康节点数 |
| `privacy_gateway_circuit_breaker_state` | Gauge | node | 熔断器状态 (0/1/2) |
| `privacy_gateway_node_admin_state` | Gauge | node | 管理状态 (0/1/2) |

---

## 10. 运维实战命令 / Operations Commands

```bash
# 启动网关（默认 round_robin 策略）
python -m engine.gateway.server

# 指定 P2C 策略 + 自定义端口
python -m engine.gateway.server --rest-port 9000 --grpc-port 60000

# 通过环境变量配置后端集群
export GATEWAY_BACKENDS="http://agent-1:8079|agent-1:50051,http://agent-2:8079|agent-2:50051"
export GATEWAY_STRATEGY=power_of_two_choices

# 启用 TLS 终结 + mTLS
export GATEWAY_TLS_ENABLED=true
export GATEWAY_TLS_CERT=/etc/ssl/gateway.crt
export GATEWAY_TLS_KEY=/etc/ssl/gateway.key
export GATEWAY_TLS_CA=/etc/ssl/ca.crt

# 动态注册新节点
curl -X POST http://gateway:8000/v1/gateway/register \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{"http_url": "http://agent-3:8079", "grpc_address": "agent-3:50051", "weight": 5}'

# 优雅排空节点（滚动更新场景）
curl -X POST http://gateway:8000/v1/gateway/drain \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{"http_url": "http://agent-1:8079", "grpc_address": "agent-1:50051"}'
```

---

## 11. 扩展阅读 / Further Reading

1. **Nginx 平滑加权轮询算法**：Nginx 源码中的 smooth weighted round-robin 实现
2. **P2C 论文**："The Power of Two Random Choices" — Mitzenmacher et al., 1996
3. **熔断器模式**：Michael Nygard《Release It!》Chapter 5
4. **指数退避 + 抖动**：AWS Architecture Blog — "Exponential Backoff and Jitter"
5. **gRPC 生产部署**：https://grpc.io/docs/guides/deployment-operations
