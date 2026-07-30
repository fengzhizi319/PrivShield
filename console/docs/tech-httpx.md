# httpx 技术栈说明 / httpx Technology Stack

## 1. 技术简介 / Introduction

httpx 是一个功能齐全的 Python HTTP 客户端库，同时支持同步与异步调用，API 设计兼容 requests。
httpx is a fully-featured Python HTTP client library supporting both sync and async calls, with a requests-compatible API.

核心特性 / Core Features：
- **异步支持（Async Support）**：原生 `AsyncClient` 支持 asyncio，适合高并发代理场景。
- **连接池（Connection Pooling）**：复用 TCP 连接，减少握手开销。
- **HTTP/2 支持**：可选启用 HTTP/2 协议。
- **超时控制（Timeout Control）**：细粒度超时配置（连接/读取/写入/池）。
- **流式传输（Streaming）**：支持大文件流式上传/下载。
- **requests 兼容 API**：迁移成本极低。

本项目使用版本 / Version Used：`httpx >= 0.27.0`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 异步代理客户端 / Async Proxy Client

文件 / File：`console/backend/app/client.py`

```python
import httpx

class PrivacyAgentClient:
    """转发请求到 privacy-local-agent 的异步客户端。
    Async client forwarding requests to privacy-local-agent."""

    async def _get_client(self) -> httpx.AsyncClient:
        """懒初始化连接池客户端 / Lazy-init pooled client"""
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=60.0,
                follow_redirects=False,  # 安全：不跟随重定向 / Security: no redirect follow
                trust_env=False,         # 直连：不走系统代理 / Direct: bypass system proxy
            )
        return self._client
```

### 2.2 请求转发 / Request Forwarding

```python
async def request(self, method, path, body=None, raw_content=None, content_type=None):
    client = await self._get_client()

    if raw_content is not None:
        # 二进制载荷（如 Arrow IPC）/ Binary payload (e.g. Arrow IPC)
        response = await client.request(method, url, content=raw_content, headers=headers)
    elif body is not None:
        # JSON 载荷 / JSON payload
        response = await client.request(method, url, json=body, headers=headers)
    else:
        # 无请求体（GET）/ No body (GET)
        response = await client.request(method, url, headers=headers)
```

### 2.3 错误处理 / Error Handling

```python
try:
    response = await client.request(...)
except httpx.RequestError as exc:
    # 网络层错误 → 502 Bad Gateway
    # Network error → 502 Bad Gateway
    raise HTTPException(status_code=502, detail=f"Unable to reach agent: {exc}")

try:
    response.raise_for_status()
except httpx.HTTPStatusError as exc:
    # agent 返回非 2xx → 透传状态码
    # Agent non-2xx → passthrough status code
    raise HTTPException(status_code=response.status_code, detail=detail)
```

### 2.4 负载均衡探测 / Load Balancer Probing

文件 / File：`console/backend/app/main.py`

```python
# 临时客户端用于 LB 探测（transport 可注入 MockTransport 供测试）
# Temporary client for LB probing (transport injectable for testing)
async with httpx.AsyncClient(transport=transport, timeout=10.0, trust_env=False) as lb_client:
    await asyncio.gather(*(probe(i) for i in seq))  # 并发探测 / Concurrent probing
```

### 2.5 连接池管理与生命周期 / Connection Pool Management & Lifecycle

```python
# 应用级单例客户端，通过 lifespan 管理生命周期 / App-level singleton client, managed via lifespan
class PrivacyAgentClient:
    def __init__(self):
        self._client: httpx.AsyncClient | None = None  # 懒初始化 / Lazy init

    async def _get_client(self) -> httpx.AsyncClient:
        """懒初始化连接池客户端 / Lazy-init pooled client

        详细逻辑 / Detailed Logic：
          1. 检查客户端是否已初始化且未关闭
          2. 未初始化时创建新的 AsyncClient（复用 TCP 连接）
          3. 已关闭时重新创建（优雅重连）
        """
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=60.0,               # 全局超时 / Global timeout
                follow_redirects=False,     # 安全：不跟随重定向 / Security: no redirect
                trust_env=False,            # 直连：不走系统代理 / Direct: bypass proxy
            )
        return self._client

# lifespan 中的资源管理 / Resource management in lifespan
@asynccontextmanager
async def lifespan(app: FastAPI):
    _ = await agent_client._get_client()  # 启动时预热连接池 / Warm up pool on start
    yield
    if agent_client._client is not None:
        await agent_client._client.aclose()  # 关闭时释放连接 / Release connections on shutdown
```

### 2.6 SSRF 防护与测试 / SSRF Protection & Testing

```python
# 负载均衡探测中的 SSRF 防护 / SSRF protection in load balancer probing
def validate_backends(backends: list[dict], allowed_hosts: list[str] | None) -> None:
    """校验探测目标 URL 的 scheme / host 白名单。
    Validate probe target URL scheme / host whitelist.

    规则 / Rules：
      - 仅允许 http/https scheme / Only http/https scheme allowed
      - 配置了 allowed_hosts 时，主机必须在白名单内 / Host must be in whitelist when configured
      - 禁止内网地址（可配置）/ Forbid internal addresses (configurable)
    """
    ...

# 测试中注入 MockTransport 隔离网络 I/O / Inject MockTransport in tests to isolate network I/O
async def test_lb_probe_mock():
    transport = httpx.MockTransport(lambda req: httpx.Response(200, json={"status": "ok"}))
    async with httpx.AsyncClient(transport=transport) as client:
        resp = await client.get("http://fake/health")
        assert resp.status_code == 200
```

### 2.7 响应内容类型分发 / Response Content-Type Dispatch

```python
# 根据 Content-Type 决定解析策略 / Decide parsing strategy by Content-Type
async def forward(self, method, path, body=None, ...):
    response = await client.request(...)
    ct = response.headers.get("content-type", "")

    if "application/vnd.apache.arrow.stream" in ct:
        # Arrow IPC 二进制流 → PyArrow 解析 / Arrow IPC binary → PyArrow parse
        return self._parse_arrow_response(response)
    elif "application/json" in ct:
        # 标准 JSON → 直接反序列化 / Standard JSON → direct deserialization
        return response.json()
    else:
        # 其他类型 → 返回原始文本 / Other types → return raw text
        return {"raw": response.text}
```

### 2.8 关键配置说明 / Key Configuration Notes

| 配置 / Config | 值 / Value | 原因 / Reason |
|---|---|---|
| `timeout` | 60.0s | 容忍 LLM 等慢端点 / Tolerate slow LLM endpoints |
| `follow_redirects` | False | 防止重定向绕过限制 / Prevent redirect bypass |
| `trust_env` | False | 避免系统代理干扰本地通信 / Avoid system proxy interference |
| 连接池复用 | 应用级单例 | 减少 TCP 握手开销 / Reduce TCP handshake overhead |
| `base_url` | 环境变量 | 支持多环境部署（dev/staging/prod）/ Multi-env deployment |

### 2.9 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| httpx 而非 requests | 原生异步支持，与 FastAPI 完美配合 / Native async, perfect with FastAPI |
| AsyncClient 而非同步 Client | 代理场景为 I/O 密集，异步提升吐量 / Proxy is I/O bound, async improves throughput |
| 懒初始化 + lifespan 关闭 | 避免导入时副作用 + 确保资源释放 / Avoid import side effects + ensure resource release |
| MockTransport 测试 | 无需真实网络，测试快速且确定性 / No real network, fast and deterministic tests |
| trust_env=False | 本地开发不受系统代理影响 / Local dev unaffected by system proxy |

### 2.10 连接池内部机制 / Connection Pool Internals

```text
┌─────────────────────────────────────────────────────────────┐
│  httpx.AsyncClient                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  连接池 / Connection Pool                             │ │
│  │  ┌─────────────────────────────────────────────┐   │ │
│  │  │  空闲连接 / Idle connections                  │   │ │
│  │  │  - TCP conn #1 (keep-alive)                   │   │ │
│  │  │  - TCP conn #2 (keep-alive)                   │   │ │
│  │  └─────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────┐   │ │
│  │  │  活跃连接 / Active connections                │   │ │
│  │  │  - TCP conn #3 (in use)                       │   │ │
│  │  └─────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────┘ │
│  配置 / Config:                                             │
│  - max_connections: 100 (默认) / Default                     │
│  - max_keepalive_connections: 20 (默认) / Default            │
│  - keepalive_expiry: 5s (默认) / Default                     │
└─────────────────────────────────────────────────────────────┘
```

**连接复用流程 / Connection Reuse Flow**：

```text
请求 1: 新建 TCP 连接 → 发送请求 → 接收响应 → 连接归还池
Request 1: New TCP conn → Send → Receive → Return to pool

请求 2: 从池取连接 → 发送请求 → 接收响应 → 连接归还池
Request 2: Get from pool → Send → Receive → Return to pool

优势：省去 TCP 三次握手 + TLS 握手（~50-100ms）
Benefit: Skip TCP handshake + TLS handshake (~50-100ms)
```

### 2.11 超时配置详解 / Timeout Configuration Details

```python
# 细粒度超时配置 / Fine-grained timeout configuration
timeout = httpx.Timeout(
    connect=5.0,    # TCP 连接超时 / TCP connection timeout
    read=60.0,      # 读取响应超时 / Read response timeout
    write=10.0,     # 发送请求体超时 / Send request body timeout
    pool=5.0,       # 等待连接池超时 / Wait for pool connection timeout
)

# 本项目使用简化配置 / This project uses simplified config
httpx.AsyncClient(timeout=60.0)  # 所有阶段统一 60s / All phases 60s
```

**超时场景分析 / Timeout Scenario Analysis**：

| 场景 / Scenario | 触发超时 / Triggered Timeout | 处理 / Handling |
|---|---|---|
| agent 未启动 / Agent not started | connect (5s) | 502 Bad Gateway |
| LLM 推理慢 / Slow LLM inference | read (60s) | 504 Gateway Timeout |
| 连接池耗尽 / Pool exhausted | pool (5s) | 503 Service Unavailable |
| 大文件上传 / Large file upload | write (10s) | 400 Bad Request |

### 2.12 HTTP/2 支持 / HTTP/2 Support

```python
# httpx 支持 HTTP/2（需安装 h2 包）/ httpx supports HTTP/2 (requires h2 package)
# pip install httpx[http2]

# 启用 HTTP/2 / Enable HTTP/2
client = httpx.AsyncClient(http2=True)

# HTTP/2 优势（本项目未启用）/ HTTP/2 benefits (not enabled in this project):
# - 多路复用：单连接并发多请求 / Multiplexing: concurrent requests on single conn
# - 头部压缩：减少重复头传输 / Header compression: reduce redundant headers
# - 服务器推送：主动推送资源 / Server push: proactively push resources
```

**本项目未启用 HTTP/2 的原因 / Why HTTP/2 not enabled**：

| 原因 / Reason | 说明 / Description |
|---|---|
| 本地通信 / Local communication | 127.0.0.1 无网络延迟，HTTP/1.1 已足够 / No network latency, HTTP/1.1 suffices |
| 连接池已足够 / Connection pool suffices | 多连接复用达到类似效果 / Multi-conn reuse achieves similar effect |
| 减少依赖 / Reduce dependencies | 无需安装 h2 包 / No need to install h2 package |

### 2.13 事件钩子 / Event Hooks

httpx 支持请求/响应事件钩子（本项目未使用，但可用于日志/监控）：

```python
# 事件钩子示例（可用于调试/监控）/ Event hooks example (for debugging/monitoring)
async def log_request(request: httpx.Request):
    print(f"→ {request.method} {request.url}")

async def log_response(response: httpx.Response):
    print(f"← {response.status_code} ({response.elapsed.total_seconds()*1000:.1f}ms)")

client = httpx.AsyncClient(
    event_hooks={
        "request": [log_request],    # 发送前触发 / Before send
        "response": [log_response],  # 接收后触发 / After receive
    }
)
```

### 2.14 与其他 HTTP 客户端对比 / Comparison with Other HTTP Clients

| 特性 / Feature | httpx | requests | aiohttp | urllib3 |
|---|---|---|---|---|
| 异步支持 / Async | ✅ 原生 / Native | ❌ | ✅ | ❌ |
| HTTP/2 | ✅ | ❌ | ❌ | ❌ |
| 连接池 / Connection pool | ✅ | ✅ | ✅ | ✅ |
| 同步 API | ✅ | ✅ | ❌ | ✅ |
| requests 兼容 / Compatible | ✅ | - | ❌ | 部分 / Partial |
| 类型提示 / Type hints | ✅ | 部分 / Partial | 部分 / Partial | ❌ |
| 适用场景 / Use case | 现代异步 / Modern async | 简单脚本 / Simple scripts | 纯异步 / Pure async | 底层 / Low-level |

## 3. 流式传输详解 / Streaming Details

### 3.1 流式响应处理 / Streaming Response Handling

```python
# 大文件下载场景（本项目未使用，但 httpx 支持）
# Large file download scenario (not used in this project, but httpx supports)
async def download_large_file(client: httpx.AsyncClient, url: str, path: Path):
    """流式下载大文件，避免内存溢出。
    Stream download large files to avoid memory overflow."""
    async with client.stream("GET", url) as response:
        response.raise_for_status()
        with open(path, "wb") as f:
            async for chunk in response.aiter_bytes(chunk_size=8192):
                f.write(chunk)

# 流式上传（大文件）/ Streaming upload (large files)
async def upload_large_file(client: httpx.AsyncClient, url: str, file_path: Path):
    """流式上传，无需将整个文件加载到内存。
    Stream upload without loading entire file into memory."""
    async with aiofiles.open(file_path, "rb") as f:
        response = await client.post(url, content=f)
    return response
```

### 3.2 流式 vs 非流式对比 / Streaming vs Non-streaming

| 特性 / Feature | 非流式 / Non-streaming | 流式 / Streaming |
|---|---|---|
| 内存占用 / Memory | 整个响应体加载 / Entire body loaded | 分块处理 / Chunked processing |
| 适用大小 / Suitable size | < 10MB | > 10MB 或无限流 / Or infinite stream |
| 响应访问 / Response access | response.json() / response.text | 需迭代 / Must iterate |
| 连接释放 / Connection release | 自动 / Automatic | 需关闭流 / Must close stream |
| 本项目使用 / This project | ✅ 主要方式 / Primary | ❌ 未使用 / Not used |

### 3.3 本项目为何不使用流式 / Why This Project Doesn't Stream

| 原因 / Reason | 说明 / Description |
|---|---|
| 响应体较小 / Small response bodies | 隐私计算结果通常 < 1MB / Privacy results usually < 1MB |
| 需要完整解析 / Need full parsing | Arrow IPC 需完整流才能解析 / Arrow IPC needs complete stream |
| 简化错误处理 / Simplify error handling | 非流式可直接 raise_for_status() / Can directly raise_for_status() |

## 4. 重试策略与弹性 / Retry Strategy & Resilience

### 4.1 httpx 原生重试 / httpx Native Retry

httpx 本身**不提供内置重试机制**，需手动实现或使用第三方库：

```python
# 手动重试实现（本项目未使用，但推荐生产环境添加）
# Manual retry implementation (not used, but recommended for production)
import asyncio
from tenacity import retry, stop_after_attempt, wait_exponential

class PrivacyAgentClient:
    @retry(
        stop=stop_after_attempt(3),              # 最多重试 3 次 / Max 3 retries
        wait=wait_exponential(multiplier=0.5),   # 指数退避 / Exponential backoff
        retry_if_exception_type=httpx.ConnectError,  # 仅连接错误重试 / Only retry connect errors
    )
    async def request_with_retry(self, method, url, **kwargs):
        client = await self._get_client()
        return await client.request(method, url, **kwargs)
```

### 4.2 重试策略设计 / Retry Strategy Design

```text
重试决策流程 / Retry decision flow:

请求失败 / Request failed
    │
    ├── 连接错误 (ConnectError) → ✅ 重试（服务可能重启中）/ Retry (service may restart)
    ├── 超时错误 (TimeoutException) → ⚠️ 谨慎重试 / Cautious retry
    ├── 4xx 客户端错误 → ❌ 不重试（请求本身有问题）/ No retry (bad request)
    ├── 5xx 服务端错误 → ✅ 重试（服务可能暂时不可用）/ Retry (temporary unavailable)
    └── 网络错误 (NetworkError) → ✅ 重试 / Retry

指数退避策略 / Exponential backoff:
  尝试 1: 立即 / Immediate
  尝试 2: 等待 0.5s / Wait 0.5s
  尝试 3: 等待 1.0s / Wait 1.0s
  尝试 4: 等待 2.0s / Wait 2.0s
  ...
```

### 4.3 断路器模式 / Circuit Breaker Pattern

```python
# 生产环境推荐的断路器实现（本项目未使用）
# Circuit breaker implementation recommended for production (not used)
class CircuitBreaker:
    """断路器：连续失败 N 次后熔断，避免雪崩。
    Circuit breaker: trip after N consecutive failures to avoid cascade."""

    def __init__(self, failure_threshold: int = 5, recovery_timeout: float = 30.0):
        self.failure_count = 0
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.last_failure_time: float | None = None
        self.state = "closed"  # closed / open / half-open

    async def call(self, func, *args, **kwargs):
        if self.state == "open":
            if time.time() - self.last_failure_time > self.recovery_timeout:
                self.state = "half-open"  # 尝试恢复 / Try recovery
            else:
                raise CircuitOpenError("Circuit breaker is open")

        try:
            result = await func(*args, **kwargs)
            self._on_success()
            return result
        except Exception as e:
            self._on_failure()
            raise
```

## 5. 自定义 Transport / Custom Transport

### 5.1 Transport 架构 / Transport Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  httpx.AsyncClient                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  高层 API / High-level API                            │ │
│  │  client.get() / client.post() / client.stream()       │ │
│  └──────────────────────────┬──────────────────────────┘ │
│                             │                              │
│  ┌──────────────────────────▼──────────────────────────┐ │
│  │  Transport 层 / Transport Layer                       │ │
│  │  ┌───────────────────────────────────────────────┐ │ │
│  │  │  HTTPTransport (default)                        │ │ │
│  │  │  - TCP 连接管理 / TCP connection management    │ │ │
│  │  │  - TLS 握手 / TLS handshake                    │ │ │
│  │  │  - HTTP/1.1 或 HTTP/2 协议 / Protocol         │ │ │
│  │  └───────────────────────────────────────────────┘ │ │
│  │  ┌───────────────────────────────────────────────┐ │ │
│  │  │  MockTransport (testing)                        │ │ │
│  │  │  - 无真实网络 I/O / No real network I/O        │ │ │
│  │  │  - 确定性响应 / Deterministic responses        │ │ │
│  │  └───────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 MockTransport 测试模式 / MockTransport Testing Pattern

```python
# 本项目中的 MockTransport 使用 / MockTransport usage in this project
import httpx

def make_mock_transport(responses: dict[str, httpx.Response]) -> httpx.MockTransport:
    """创建根据 URL 返回不同响应的 Mock Transport。
    Create Mock Transport returning different responses by URL."""
    def handler(request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path in responses:
            return responses[path]
        return httpx.Response(404, json={"detail": "not found"})

    return httpx.MockTransport(handler)

# 测试示例 / Test example
async def test_health_check():
    transport = make_mock_transport({
        "/health": httpx.Response(200, json={"status": "ok"}),
        "/v1/privacy/mask": httpx.Response(200, json={"masked": "138****1234"}),
    })
    async with httpx.AsyncClient(transport=transport) as client:
        resp = await client.get("http://fake/health")
        assert resp.status_code == 200
        assert resp.json()["status"] == "ok"
```

### 5.3 自定义 Transport 场景 / Custom Transport Scenarios

| 场景 / Scenario | Transport | 用途 / Purpose |
|---|---|---|
| 单元测试 / Unit testing | MockTransport | 无网络依赖 / No network dependency |
| 故障注入 / Fault injection | 自定义 / Custom | 模拟超时/错误 / Simulate timeout/error |
| 日志记录 / Logging | 包装 / Wrapper | 记录所有请求 / Log all requests |
| 重定向 / Redirect | 自定义 / Custom | 控制重定向行为 / Control redirect behavior |
| 代理 / Proxy | HTTPTransport(proxy=...) | 通过代理访问 / Access via proxy |

## 6. Cookie 与认证 / Cookie & Authentication

### 6.1 Cookie 管理 / Cookie Management

```python
# httpx 自动管理 Cookie（本项目未使用）/ httpx auto-manages cookies (not used)
client = httpx.AsyncClient(cookies={"session": "abc123"})

# 响应中的 Set-Cookie 自动存储 / Set-Cookie in response auto-stored
response = await client.get("http://example.com/login")
# client.cookies 自动更新 / Auto-updated

# 手动设置 Cookie / Manually set cookies
client.cookies.set("token", "xyz789", domain="example.com")

# 清除所有 Cookie / Clear all cookies
client.cookies.clear()
```

### 6.2 认证模式 / Authentication Patterns

```python
# 模式 1: Bearer Token（本项目使用）/ Bearer Token (this project uses)
headers = {"Authorization": f"Bearer {api_key}"}
response = await client.request(method, url, headers=headers)

# 模式 2: httpx 内置 Auth 机制 / httpx built-in Auth mechanism
class APIKeyAuth(httpx.Auth):
    """自定义认证流 / Custom auth flow"""
    def __init__(self, api_key: str):
        self.api_key = api_key

    def auth_flow(self, request: httpx.Request):
        request.headers["X-API-Key"] = self.api_key
        yield request

client = httpx.AsyncClient(auth=APIKeyAuth("secret-key"))

# 模式 3: Basic Auth / Basic Auth
client = httpx.AsyncClient(auth=("username", "password"))
```

### 6.3 本项目认证实现 / This Project's Auth Implementation

```python
# client.py 中的 API Key 附加逻辑 / API Key attachment logic in client.py
class PrivacyAgentClient:
    def __init__(self, base_url: str, api_key: str | None = None):
        self.api_key = api_key  # 可选：未配置时不附加 / Optional: not attached when unconfigured

    async def request(self, method, path, body=None, ...):
        headers = {}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        # 转发请求 / Forward request
        response = await client.request(method, url, headers=headers, ...)
```

## 7. 性能调优 / Performance Tuning

### 7.1 连接池参数调优 / Connection Pool Parameter Tuning

```python
# 生产环境连接池配置（本项目使用默认值）
# Production connection pool config (this project uses defaults)
limits = httpx.Limits(
    max_connections=100,          # 最大连接数 / Max connections
    max_keepalive_connections=20, # 最大保持连接 / Max keep-alive connections
    keepalive_expiry=5.0,         # 保持连接过期时间 (s) / Keep-alive expiry
)

client = httpx.AsyncClient(
    limits=limits,
    timeout=httpx.Timeout(60.0),
)
```

### 7.2 性能基准 / Performance Benchmarks

| 操作 / Operation | 延迟 / Latency | 说明 / Description |
|---|---|---|
| 新建连接 / New connection | ~5-10ms | TCP 三次握手 / TCP handshake |
| 复用连接 / Reused connection | ~0.1ms | 从池获取 / Get from pool |
| JSON 解析 (1KB) | ~0.05ms | 小响应 / Small response |
| JSON 解析 (1MB) | ~5ms | 大响应 / Large response |
| Arrow IPC 解析 (1MB) | ~1ms | 二进制更快 / Binary faster |

### 7.3 最佳实践清单 / Best Practices Checklist

| 实践 / Practice | 说明 / Description |
|---|---|
| 复用 AsyncClient / Reuse AsyncClient | 避免每次请求新建客户端 / Avoid new client per request |
| 设置合理超时 / Set reasonable timeout | 防止无限等待 / Prevent infinite wait |
| 使用 base_url / Use base_url | 减少 URL 拼接错误 / Reduce URL concat errors |
| trust_env=False | 避免系统代理干扰 / Avoid system proxy interference |
| follow_redirects=False | 安全：防止重定向攻击 / Security: prevent redirect attack |
| 测试用 MockTransport / MockTransport for tests | 无网络依赖 / No network dependency |
| lifespan 管理生命周期 / lifespan for lifecycle | 确保资源释放 / Ensure resource release |

## 8. HTTP/2 支持详解 / HTTP/2 Support Details

### 8.1 HTTP/2 协议优势 / HTTP/2 Protocol Advantages

| 特性 / Feature | HTTP/1.1 | HTTP/2 | 本项目影响 / Project Impact |
|---|---|---|---|
| 多路复用 / Multiplexing | 每连接单请求 / Single req per conn | 单连接多请求并行 / Multiple parallel reqs | 减少连接数 / Fewer connections |
| 头部压缩 / Header compression | 每次完整发送 / Full headers each time | HPACK 压缩 / HPACK compressed | 减少带宽 / Less bandwidth |
| 服务器推送 / Server push | 不支持 / Not supported | 主动推送资源 / Proactive push | 未使用 / Not used |
| 二进制帧 / Binary framing | 文本协议 / Text protocol | 二进制分帧 / Binary frames | 解析更快 / Faster parsing |

### 8.2 启用 HTTP/2 / Enabling HTTP/2

```python
# httpx 支持 HTTP/2（需安装 h2 依赖）/ httpx supports HTTP/2 (requires h2 dep)
# pip install httpx[http2]

import httpx

# 启用 HTTP/2（同时兼容 HTTP/1.1 回退）
# Enable HTTP/2 (with HTTP/1.1 fallback)
client = httpx.AsyncClient(
    http2=True,               # 启用 HTTP/2 / Enable HTTP/2
    base_url="https://agent:8079",
)

# 检查实际协商的协议版本 / Check negotiated protocol version
response = await client.get("/v1/health")
print(response.http_version)  # "HTTP/2" 或 "HTTP/1.1"
```

### 8.3 本项目未启用 HTTP/2 的原因 / Why HTTP/2 Not Enabled

| 原因 / Reason | 说明 / Description |
|---|---|
| 本地通信 / Local communication | 代理后端与 agent 同机部署，延迟极低 / Same-machine, minimal latency |
| 请求量小 / Low request volume | 控制台单用户，无并发压力 / Single user console, no concurrency pressure |
| 减少依赖 / Fewer dependencies | 避免引入 h2/hpack 额外包 / Avoid extra h2/hpack packages |
| HTTP/1.1 已足够 / HTTP/1.1 sufficient | 代理转发场景无多路复用需求 / No multiplexing need for proxy |

## 9. 代理与连接池管理 / Proxy & Connection Pool Management

### 9.1 连接池架构 / Connection Pool Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  httpx.AsyncClient                                           │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  ConnectionPool                                              │
│  - max_connections: 100（默认）/ Default 100                  │
│  - max_keepalive_connections: 20（默认）/ Default 20          │
│  - keepalive_expiry: 5s（默认）/ Default 5s                   │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌────────────┐    ┌────────────┐    ┌────────────┐
│ Connection1│    │ Connection2│    │ Connection3│
│ (keepalive)│    │ (active)   │    │ (idle)     │
└────────────┘    └────────────┘    └────────────┘
```

### 9.2 连接池参数调优 / Pool Parameter Tuning

```python
# 自定义连接池限制 / Custom pool limits
import httpx

limits = httpx.Limits(
    max_connections=50,            # 最大总连接数 / Max total connections
    max_keepalive_connections=10,  # 最大保活连接 / Max keepalive connections
    keepalive_expiry=30.0,         # 保活过期时间(秒) / Keepalive expiry (seconds)
)

client = httpx.AsyncClient(
    limits=limits,
    base_url="http://127.0.0.1:8079",
)
```

**本项目的连接池策略 / This project's pool strategy**：

| 参数 / Parameter | 本项目值 / Value | 原因 / Reason |
|---|---|---|
| max_connections | 默认 100 / Default | 单用户控制台无压力 / Single user, no pressure |
| max_keepalive | 默认 20 / Default | 保持少量热连接 / Keep few hot connections |
| keepalive_expiry | 默认 5s / Default | 快速回收空闲连接 / Quick idle reclaim |

### 9.3 代理配置 / Proxy Configuration

```python
# httpx 支持 HTTP/SOCKS 代理 / httpx supports HTTP/SOCKS proxy
client = httpx.AsyncClient(
    proxy="http://proxy:8080",          # HTTP 代理 / HTTP proxy
    # proxy="socks5://proxy:1080",      # SOCKS5 代理 / SOCKS5 proxy
)

# 本项目显式禁用代理 / This project explicitly disables proxy
client = httpx.AsyncClient(
    trust_env=False,  # 忽略环境变量中的代理设置 / Ignore env proxy settings
)
```

**禁用代理的原因 / Why proxy is disabled**：
- 代理后端与 agent 同机通信，无需代理 / Same-machine communication, no proxy needed
- 避免企业网络环境变量干扰 / Avoid corporate network env interference
- 确保请求直达目标，减少延迟 / Ensure direct requests, reduce latency

## 10. 监控与可观测性 / Monitoring & Observability

### 10.1 请求事件钩子 / Request Event Hooks

```python
# httpx 支持请求/响应事件钩子 / httpx supports request/response event hooks
import logging
import time

logger = logging.getLogger("console.proxy")

async def log_request(request: httpx.Request):
    """请求发出前记录 / Log before request sent."""
    request.extensions["start_time"] = time.perf_counter()
    logger.debug(f"→ {request.method} {request.url}")

async def log_response(response: httpx.Response):
    """响应到达后记录 / Log after response received."""
    start = response.request.extensions.get("start_time", 0)
    duration_ms = (time.perf_counter() - start) * 1000
    logger.info(
        f"← {response.status_code} {response.request.url.path} "
        f"({duration_ms:.1f}ms)"
    )

client = httpx.AsyncClient(
    event_hooks={
        "request": [log_request],
        "response": [log_response],
    },
    base_url="http://127.0.0.1:8079",
)
```

### 10.2 与 OpenTelemetry 集成 / OpenTelemetry Integration

```python
# 生产环境可集成 OTel 追踪 / Production can integrate OTel tracing
# pip install opentelemetry-instrumentation-httpx

from opentelemetry.instrumentation.httpx import HTTPXClientInstrumentor

# 自动为所有 httpx 请求创建 span / Auto-create spans for all httpx requests
HTTPXClientInstrumentor().instrument()

# 每个请求自动生成：/ Each request auto-generates:
# - span: "HTTP {method}" with attributes:
#   - http.method, http.url, http.status_code
#   - http.request_content_length, http.response_content_length
```

### 10.3 本项目监控策略 / This Project's Monitoring Strategy

| 层级 / Layer | 实现 / Implementation | 指标 / Metrics |
|---|---|---|
| 代理层 / Proxy layer | 响应中 `duration_ms` 字段 / `duration_ms` in response | 请求耗时 / Request duration |
| 应用层 / App layer | FastAPI 中间件日志 / FastAPI middleware logs | 状态码分布 / Status distribution |
| 传输层 / Transport layer | httpx event_hooks（可选）/ httpx event_hooks (optional) | 连接复用率 / Connection reuse |
| 基础设施 / Infrastructure | Prometheus `/metrics`（agent 端）/ Agent-side | 全局吞吐 / Global throughput |

## 11. 错误处理模式 / Error Handling Patterns

### 11.1 httpx 异常层次 / httpx Exception Hierarchy

```text
httpx 异常体系 / httpx exception hierarchy:

HTTPError (基类 / Base)
├── HTTPStatusError          # 4xx/5xx 响应（raise_for_status）/ 4xx/5xx response
├── RequestError             # 请求发送失败 / Request send failure
│   ├── TransportError       # 传输层错误 / Transport layer error
│   │   ├── ConnectError     # 连接失败 / Connection failed
│   │   ├── ConnectTimeout   # 连接超时 / Connection timeout
│   │   ├── ReadTimeout      # 读取超时 / Read timeout
│   │   ├── WriteTimeout     # 写入超时 / Write timeout
│   │   ├── PoolTimeout      # 连接池超时 / Pool timeout
│   │   └── NetworkError     # 网络错误 / Network error
│   ├── DecodingError        # 响应解码失败 / Response decode failure
│   └── TooManyRedirects     # 重定向过多 / Too many redirects
└── StreamError              # 流式传输错误 / Streaming error
    ├── StreamConsumed       # 流已消费 / Stream consumed
    └── StreamClosed         # 流已关闭 / Stream closed
```

### 11.2 本项目错误处理实践 / This Project's Error Handling Practice

```python
# console/backend/app/client.py — 代理请求错误处理
# Proxy request error handling
import httpx
from fastapi import HTTPException

async def proxy_to_agent(request: ProxyRequest) -> ProxyResponse:
    """转发请求到 Agent，统一错误处理 / Forward to Agent with unified error handling"""
    client: httpx.AsyncClient = app.state.client

    try:
        response = await client.request(
            method=request.method,
            url=request.path,
            json=request.body,
            timeout=request.timeout,
        )
        return ProxyResponse(
            status=response.status_code,
            body=response.json(),
            duration_ms=response.elapsed.total_seconds() * 1000,
        )

    except httpx.ConnectError as e:
        # Agent 不可达 / Agent unreachable
        raise HTTPException(502, f"Agent 连接失败 / Agent connect failed: {e}")

    except httpx.TimeoutException as e:
        # 请求超时 / Request timeout
        raise HTTPException(504, f"Agent 响应超时 / Agent timeout: {type(e).__name__}")

    except httpx.DecodingError:
        # 响应非 JSON / Response not JSON
        raise HTTPException(502, "Agent 返回无效 JSON / Agent returned invalid JSON")

    except httpx.HTTPError as e:
        # 其他 HTTP 错误 / Other HTTP errors
        raise HTTPException(502, f"代理错误 / Proxy error: {e}")
```

### 11.3 错误处理策略对比 / Error Handling Strategy Comparison

| 策略 / Strategy | 适用场景 / Use Case | 优点 / Pros | 缺点 / Cons |
|---|---|---|---|
| 逐层捕获 / Layered catch | 不同错误不同处理 / Different handling | 精确控制 / Precise | 代码冗长 / Verbose |
| 统一捕获 / Unified catch | 简单代理 / Simple proxy | 简洁 / Concise | 丢失细节 / Loses detail |
| 重试 + 捕获 / Retry + catch | 不稳定网络 / Unstable network | 弹性 / Resilient | 延迟增加 / Added latency |
| 熔断器 / Circuit breaker | 高频调用 / High-frequency | 快速失败 / Fast fail | 实现复杂 / Complex |

## 12. 异步并发模式 / Async Concurrency Patterns

### 12.1 并发请求 / Concurrent Requests

```python
import asyncio
import httpx

async def batch_proxy(requests: list[ProxyRequest]) -> list[ProxyResponse]:
    """并发执行多个代理请求 / Execute multiple proxy requests concurrently"""
    async with httpx.AsyncClient(base_url=AGENT_URL) as client:
        # 创建所有请求任务 / Create all request tasks
        tasks = [
            client.request(
                method=req.method,
                url=req.path,
                json=req.body,
            )
            for req in requests
        ]
        # 并发执行 / Execute concurrently
        responses = await asyncio.gather(*tasks, return_exceptions=True)

        # 处理结果 / Process results
        results = []
        for resp in responses:
            if isinstance(resp, Exception):
                results.append(ProxyResponse(status=502, body={"error": str(resp)}))
            else:
                results.append(ProxyResponse(
                    status=resp.status_code,
                    body=resp.json(),
                ))
        return results
```

### 12.2 信号量控制并发度 / Semaphore for Concurrency Control

```python
# 限制并发连接数 / Limit concurrent connections
MAX_CONCURRENT = 10
_semaphore = asyncio.Semaphore(MAX_CONCURRENT)

async def limited_request(client: httpx.AsyncClient, url: str):
    """带并发限制的请求 / Request with concurrency limit"""
    async with _semaphore:
        return await client.get(url)

# 使用 / Usage:
async def health_check_all(endpoints: list[str]):
    """并发健康检查所有端点 / Concurrently health-check all endpoints"""
    async with httpx.AsyncClient(timeout=5.0) as client:
        tasks = [limited_request(client, ep) for ep in endpoints]
        return await asyncio.gather(*tasks, return_exceptions=True)
```

### 12.3 异步上下文管理器 / Async Context Manager

```python
# 本项目客户端生命周期管理 / This project's client lifecycle management
from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app):
    """应用生命周期 / Application lifecycle"""
    # 启动：创建客户端 / Startup: create client
    app.state.client = httpx.AsyncClient(
        base_url=settings.agent_url,
        timeout=httpx.Timeout(30.0, connect=5.0),
        limits=httpx.Limits(
            max_connections=100,       # 最大连接数 / Max connections
            max_keepalive_connections=20,  # 保持活跃连接 / Keepalive connections
        ),
    )
    yield
    # 关闭：释放资源 / Shutdown: release resources
    await app.state.client.aclose()
```

## 13. 测试集成 / Testing Integration

### 13.1 MockTransport 测试 / MockTransport Testing

```python
# 使用 MockTransport 进行无网络测试 / Network-free testing with MockTransport
import httpx
import pytest

def create_mock_client(handler) -> httpx.AsyncClient:
    """创建 mock 客户端 / Create mock client"""
    transport = httpx.MockTransport(handler)
    return httpx.AsyncClient(transport=transport, base_url="http://test")

# 测试用例 / Test cases:
@pytest.mark.asyncio
async def test_proxy_success():
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/v1/mask"
        return httpx.Response(200, json={"masked": {"email": "j***"}})

    async with create_mock_client(handler) as client:
        resp = await client.post("/api/v1/mask", json={"data": {"email": "john@x.com"}})
        assert resp.status_code == 200
        assert resp.json()["masked"]["email"] == "j***"

@pytest.mark.asyncio
async def test_proxy_timeout():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectTimeout("连接超时 / Connect timeout")

    async with create_mock_client(handler) as client:
        with pytest.raises(httpx.ConnectTimeout):
            await client.get("/slow-endpoint")
```

### 13.2 pytest fixture 集成 / pytest fixture Integration

```python
# conftest.py — 测试固件 / Test fixtures
import pytest
import httpx
from app.main import app

@pytest.fixture
async def client():
    """提供测试用 HTTP 客户端 / Provide test HTTP client"""
    from httpx import ASGITransport, AsyncClient
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        yield c

# 使用 / Usage:
@pytest.mark.asyncio
async def test_health(client):
    resp = await client.get("/health")
    assert resp.status_code == 200

@pytest.mark.asyncio
async def test_proxy_endpoint(client):
    resp = await client.post("/api/proxy", json={
        "method": "POST",
        "path": "/api/v1/mask",
        "body": {"data": {"email": "test@example.com"}},
    })
    assert resp.status_code in (200, 502)  # Agent 可能未启动 / Agent may not be running
```

### 13.3 测试策略总结 / Testing Strategy Summary

| 测试类型 / Test Type | 工具 / Tool | 网络 / Network | 本项目 / Project |
|---|---|---|---|
| 单元测试 / Unit test | MockTransport | 无 / None | ✅ 主要 / Primary |
| 集成测试 / Integration test | ASGITransport | 无 / None | ✅ 使用 / Used |
| 端到端 / End-to-end | 真实服务器 / Real server | 有 / Yes | ✅ smoke_test.py |
| 性能测试 / Performance test | locust / k6 | 有 / Yes | ❌ 未实现 / Not yet |

## 14. 连接池内部机制 / Connection Pool Internals

### 14.1 httpx 连接池架构 / httpx Connection Pool Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  httpx 连接池内部结构 / httpx Connection Pool Internals         │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  AsyncClient                                                   │
│    │                                                           │
│    └── AsyncHTTPTransport                                      │
│        │                                                       │
│        └── httpcore.AsyncConnectionPool                        │
│            │                                                   │
│            ├── 连接池配置 / Pool config                         │
│            │   ├── max_connections: 100 (默认 / default)       │
│            │   ├── max_keepalive_connections: 20               │
│            │   └── keepalive_expiry: 5.0s                      │
│            │                                                   │
│            └── 连接存储 / Connection storage                    │
│                ├── {("https", "host", 443): [conn1, conn2]}   │
│                ├── {("http", "127.0.0.1", 8079): [conn3]}     │
│                └── ...                                         │
│                                                                │
│  连接生命周期 / Connection lifecycle:                           │
│  创建 / Create → 使用 / Use → 归还 / Return → 复用 / Reuse    │
│                                         └→ 过期 / Expire → 关闭 / Close │
└────────────────────────────────────────────────────────────────┘
```

### 14.2 连接池配置详解 / Connection Pool Configuration Details

```python
import httpx

# 连接池配置 / Connection pool configuration
client = httpx.AsyncClient(
    limits=httpx.Limits(
        # 最大连接数（所有主机共享）
        # Max connections (shared across all hosts)
        max_connections=100,

        # 最大保持活跃连接数
        # Max keep-alive connections
        max_keepalive_connections=20,

        # 保持活跃过期时间（秒）
        # Keep-alive expiry (seconds)
        keepalive_expiry=5.0,
    ),
    # 连接超时 / Connection timeout
    timeout=httpx.Timeout(
        connect=5.0,     # TCP 连接超时 / TCP connect timeout
        read=30.0,       # 读取超时 / Read timeout
        write=10.0,      # 写入超时 / Write timeout
        pool=5.0,        # 等待连接池超时 / Pool wait timeout
    ),
)

# 本项目配置 / This project's config:
# 代理场景，连接数需求低 / Proxy scenario, low connection needs
# 默认配置已足够 / Default config sufficient
```

### 14.3 连接复用与 HTTP Keep-Alive / Connection Reuse & HTTP Keep-Alive

```python
# 连接复用示例 / Connection reuse example
import httpx
import asyncio
import time

async def demonstrate_reuse():
    """Demonstrate connection reuse benefit.
    展示连接复用的优势 / Show connection reuse advantage.
    """
    async with httpx.AsyncClient() as client:
        # 第一次请求：建立 TCP 连接 / First request: establish TCP
        start = time.perf_counter()
        await client.get("http://127.0.0.1:8079/health")
        first = time.perf_counter() - start

        # 第二次请求：复用连接 / Second request: reuse connection
        start = time.perf_counter()
        await client.get("http://127.0.0.1:8079/health")
        second = time.perf_counter() - start

        print(f"First (new conn): {first*1000:.1f}ms")   # ~15ms
        print(f"Second (reused):    {second*1000:.1f}ms")  # ~3ms
        # 复用快 ~5x / Reuse ~5x faster


# 本项目中的连接复用 / Connection reuse in this project:
# Console Backend 使用单个 AsyncClient 实例
# Console Backend uses single AsyncClient instance
# 所有请求共享连接池 / All requests share connection pool
```

### 14.4 本项目连接池实践 / This Project's Connection Pool Practice

| 配置 / Config | 值 / Value | 说明 / Notes |
|---|---|---|
| max_connections | 100 (默认) | 代理场景足够 / Sufficient for proxy |
| max_keepalive | 20 (默认) | 保持少量活跃连接 / Keep few active |
| keepalive_expiry | 5s (默认) | 快速回收空闲连接 / Quick reclaim idle |
| Client 实例 | 单例 / Singleton | 应用生命周期共享 / Shared app lifecycle |
| 连接目标 / Target | 127.0.0.1:8079 | 单一 Agent / Single Agent |

## 15. 中间件与拦截器模式 / Middleware & Interceptor Patterns

### 15.1 Event Hooks 机制 / Event Hooks Mechanism

```python
import httpx
import logging
import time

logger = logging.getLogger(__name__)

# 请求拦截器 / Request interceptor
async def log_request(request: httpx.Request):
    """记录请求信息 / Log request info."""
    request.extensions["start_time"] = time.perf_counter()
    logger.debug(f"→ {request.method} {request.url}")


# 响应拦截器 / Response interceptor
async def log_response(response: httpx.Response):
    """记录响应信息 / Log response info."""
    start = response.request.extensions.get("start_time", 0)
    duration = (time.perf_counter() - start) * 1000
    logger.info(
        f"← {response.status_code} {response.request.url} "
        f"({duration:.1f}ms)"
    )


# 配置 event hooks / Configure event hooks
client = httpx.AsyncClient(
    event_hooks={
        "request": [log_request],
        "response": [log_response],
    }
)

# 本项目使用 / This project's usage:
# Console Backend 中可选启用日志 hooks
# Optional logging hooks in Console Backend
```

### 15.2 自定义中间件模式 / Custom Middleware Pattern

```python
import httpx
from typing import Callable, Awaitable

class RetryMiddleware:
    """重试中间件 / Retry middleware.

    包装 httpx 客户端，添加自动重试逻辑 /
    Wraps httpx client with auto-retry logic.
    """

    def __init__(
        self,
        client: httpx.AsyncClient,
        max_retries: int = 3,
        backoff_factor: float = 0.5,
    ):
        self.client = client
        self.max_retries = max_retries
        self.backoff_factor = backoff_factor

    async def request(self, method: str, url: str, **kwargs) -> httpx.Response:
        last_error = None
        for attempt in range(self.max_retries + 1):
            try:
                response = await self.client.request(method, url, **kwargs)
                # 5xx 重试 / Retry on 5xx
                if response.status_code >= 500 and attempt < self.max_retries:
                    await asyncio.sleep(self.backoff_factor * (2 ** attempt))
                    continue
                return response
            except httpx.TransportError as e:
                last_error = e
                if attempt < self.max_retries:
                    await asyncio.sleep(self.backoff_factor * (2 ** attempt))
        raise last_error  # type: ignore


# 使用示例 / Usage example
# middleware = RetryMiddleware(client, max_retries=2)
# response = await middleware.request("POST", "/v1/mask", json=payload)
```

### 15.3 本项目拦截器实践 / This Project's Interceptor Practice

| 拦截器 / Interceptor | 状态 / Status | 说明 / Notes |
|---|---|---|
| 请求日志 / Request logging | ✅ 可选 / Optional | event_hooks 实现 / Via event_hooks |
| 响应计时 / Response timing | ✅ 使用 / Used | duration_ms 字段 / duration_ms field |
| 重试逻辑 / Retry logic | ✅ 内置 / Built-in | httpx 原生重试 / httpx native retry |
| 认证注入 / Auth injection | ✅ 可选 / Optional | headers 配置 / headers config |
| 指标采集 / Metrics collection | ❌ 未实现 / Not implemented | Prometheus 在 Agent 端 / On Agent side |

## 16. 与 aiohttp 对比 / Comparison with aiohttp

### 16.1 架构对比 / Architecture Comparison

```text
┌────────────────────────────────────────────────────────────────┐
│  httpx vs aiohttp 架构对比 / Architecture Comparison           │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  httpx:                                                        │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  httpx (API 层 / API layer)                             │  │
│  │    └── httpcore (传输层 / Transport layer)              │  │
│  │        └── h11 / h2 (协议层 / Protocol layer)           │  │
│  │            └── asyncio / trio (事件循环 / Event loop)   │  │
│  └────────────────────────────────────────────────────────┘  │
│  特点：同步+异步统一 API / Unified sync+async API              │
│                                                                │
│  aiohttp:                                                      │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  aiohttp (API + 传输 + 协议 一体 / All-in-one)          │  │
│  │    └── asyncio (事件循环 / Event loop)                  │  │
│  └────────────────────────────────────────────────────────┘  │
│  特点：仅异步，内置服务器 / Async-only, built-in server        │
└────────────────────────────────────────────────────────────────┘
```

### 16.2 详细功能对比 / Detailed Feature Comparison

| 维度 / Dimension | httpx | aiohttp | 本项目选择 / Project Choice |
|---|---|---|---|
| API 风格 / API style | requests 兼容 / requests-compatible | 独特 / Unique | httpx ✅ |
| 同步支持 / Sync support | ✅ 有 / Yes | ❌ 仅异步 / Async-only | httpx ✅ |
| HTTP/2 | ✅ 原生 / Native | ❌ 需插件 / Needs plugin | httpx ✅ |
| 类型提示 / Type hints | ✅ 完整 / Complete | 部分 / Partial | httpx ✅ |
| 服务器功能 / Server | ❌ 无 / None | ✅ 内置 / Built-in | 无关 / N/A |
| 性能 / Performance | 良好 / Good | 略快 / Slightly faster | 差异微小 / Negligible |
| 生态 / Ecosystem | 增长中 / Growing | 成熟 / Mature | httpx 足够 / Sufficient |
| FastAPI 集成 / Integration | ✅ 官方推荐 / Official rec | 可用 / Usable | httpx ✅ |
| 测试支持 / Testing | ✅ MockTransport | 需第三方 / Needs 3rd party | httpx ✅ |

### 16.3 代码风格对比 / Code Style Comparison

```python
# httpx 风格 / httpx style (本项目使用 / This project uses)
import httpx

async def mask_via_httpx(payload: dict) -> dict:
    async with httpx.AsyncClient(base_url="http://127.0.0.1:8079") as client:
        resp = await client.post("/v1/mask", json=payload, timeout=30.0)
        resp.raise_for_status()
        return resp.json()


# aiohttp 风格 / aiohttp style (未使用 / Not used)
import aiohttp

async def mask_via_aiohttp(payload: dict) -> dict:
    async with aiohttp.ClientSession() as session:
        async with session.post(
            "http://127.0.0.1:8079/v1/mask",
            json=payload,
            timeout=aiohttp.ClientTimeout(total=30),
        ) as resp:
            resp.raise_for_status()
            return await resp.json()

# httpx 优势 / httpx advantages:
# 1. API 与 requests 一致，学习成本低 / Same API as requests, low learning cost
# 2. 同步/异步无缝切换 / Seamless sync/async switch
# 3. 更好的类型提示 / Better type hints
# 4. FastAPI 官方推荐测试客户端 / FastAPI official test client
```

### 16.4 本项目选择 httpx 的原因 / Why This Project Chose httpx

| 原因 / Reason | 详细说明 / Details |
|---|---|
| FastAPI 生态 / FastAPI ecosystem | 官方推荐的 HTTP 客户端 / Official recommended client |
| 测试便利 / Testing convenience | MockTransport 无需网络 / No network needed |
| API 熟悉度 / API familiarity | requests 风格，团队熟悉 / requests-style, team familiar |
| 同步回退 / Sync fallback | 必要时可用同步模式 / Can use sync mode if needed |
| HTTP/2 就绪 / HTTP/2 ready | 未来升级无需换库 / Future upgrade no lib change |
| 类型安全 / Type safety | 完整的类型提示 / Complete type hints |

## 17. 服务网格与 Sidecar 集成 / Service Mesh & Sidecar Integration

### 17.1 Sidecar 代理模式 / Sidecar Proxy Pattern

本项目中 Console Backend 作为 Sidecar 代理转发请求到 Privacy Agent：

```
┌──────────────────────────────────────────────────────────┐
│  Pod / Local Machine                                      │
│                                                          │
│  ┌────────────┐    HTTP     ┌────────────────┐   gRPC   │
│  │  Browser   │──────────▶│ Console Backend │────────▶  │
│  │  (React)   │  :3000     │  (httpx proxy) │  :50051   │
│  └────────────┘            │                │           │
│                            │  • 路由转换    │           │
│                            │  • 认证注入    │           │
│                            │  • 超时控制    │           │
│                            └────────────────┘           │
│                                     │                    │
│                                     │ REST :8079         │
│                                     ▼                    │
│                            ┌────────────────┐           │
│                            │ Privacy Agent  │           │
│                            │  (FastAPI)     │           │
│                            └────────────────┘           │
└──────────────────────────────────────────────────────────┘
```

### 17.2 httpx 作为反向代理客户端 / httpx as Reverse Proxy Client

```python
# console/backend/app/client.py
import httpx
from contextlib import asynccontextmanager

# 全局异步客户端（连接池复用）
# Global async client (connection pool reuse)
_client: httpx.AsyncClient | None = None

@asynccontextmanager
async def get_client():
    """ 获取共享的 httpx 客户端实例 """
    """ Get shared httpx client instance """
    global _client
    if _client is None:
        _client = httpx.AsyncClient(
            base_url="http://127.0.0.1:8079",
            timeout=httpx.Timeout(
                connect=5.0,    # 连接超时 / Connect timeout
                read=30.0,      # 读取超时 / Read timeout
                write=10.0,     # 写入超时 / Write timeout
                pool=5.0,       # 连接池等待 / Pool wait
            ),
            limits=httpx.Limits(
                max_connections=100,       # 最大连接数 / Max connections
                max_keepalive_connections=20,  # 保活连接 / Keepalive
            ),
            headers={"X-Forwarded-By": "console-backend"},
        )
    yield _client


async def proxy_request(
    method: str,
    path: str,
    body: dict | None = None,
    headers: dict | None = None,
) -> httpx.Response:
    """ 代理转发请求到 Privacy Agent """
    """ Proxy forward request to Privacy Agent """
    async with get_client() as client:
        response = await client.request(
            method=method,
            url=path,
            json=body,
            headers=headers,
        )
        return response
```

### 17.3 服务发现与动态路由 / Service Discovery & Dynamic Routing

```python
# 支持多后端实例的动态路由
# Dynamic routing supporting multiple backend instances
import httpx
import asyncio
from dataclasses import dataclass, field

@dataclass
class ServiceInstance:
    """ 服务实例信息 """
    """ Service instance info """
    host: str
    port: int
    healthy: bool = True
    latency_ms: float = 0.0
    
    @property
    def base_url(self) -> str:
        return f"http://{self.host}:{self.port}"


class DynamicRouter:
    """ 动态路由器：健康检查 + 最低延迟路由 """
    """ Dynamic router: health check + lowest latency routing """
    
    def __init__(self, instances: list[ServiceInstance]):
        self.instances = instances
        self._client = httpx.AsyncClient()
    
    async def health_check_loop(self, interval: float = 10.0):
        """ 定期健康检查 """
        """ Periodic health check """
        while True:
            tasks = [
                self._check_instance(inst)
                for inst in self.instances
            ]
            await asyncio.gather(*tasks)
            await asyncio.sleep(interval)
    
    async def _check_instance(self, inst: ServiceInstance):
        try:
            start = asyncio.get_event_loop().time()
            resp = await self._client.get(
                f"{inst.base_url}/health",
                timeout=3.0,
            )
            latency = (asyncio.get_event_loop().time() - start) * 1000
            inst.healthy = resp.status_code == 200
            inst.latency_ms = latency
        except httpx.HTTPError:
            inst.healthy = False
    
    def select_instance(self) -> ServiceInstance | None:
        """ 选择最低延迟的健康实例 """
        """ Select lowest latency healthy instance """
        healthy = [i for i in self.instances if i.healthy]
        if not healthy:
            return None
        return min(healthy, key=lambda i: i.latency_ms)
```

### 17.4 mTLS Sidecar 通信 / mTLS Sidecar Communication

```python
# 生产环境中的 mTLS 配置
# mTLS configuration in production
import ssl
import httpx

def create_mtls_client(
    cert_path: str,
    key_path: str,
    ca_path: str,
    base_url: str,
) -> httpx.AsyncClient:
    """ 创建 mTLS 客户端 """
    """ Create mTLS client """
    ssl_context = ssl.create_default_context(cafile=ca_path)
    ssl_context.load_cert_chain(cert_path, key_path)
    ssl_context.minimum_version = ssl.TLSVersion.TLSv1_3
    
    return httpx.AsyncClient(
        base_url=base_url,
        verify=ssl_context,
        timeout=httpx.Timeout(10.0),
    )
```

## 18. WebSocket 与实时通信 / WebSocket & Real-time Communication

### 18.1 httpx 的 WebSocket 局限 / httpx WebSocket Limitations

httpx 本身不支持 WebSocket，需要配合其他库：

```python
# httpx 不支持 WebSocket，需使用 websockets 库
# httpx doesn't support WebSocket, use websockets library

# ✘ httpx 无法处理 WebSocket
# ✘ httpx cannot handle WebSocket
# await client.get("ws://...")  # 不支持 / Not supported

# ✔ 使用 websockets 库
# ✔ Use websockets library
import websockets
import httpx

class HybridClient:
    """ 混合客户端：HTTP + WebSocket """
    """ Hybrid client: HTTP + WebSocket """
    
    def __init__(self, base_url: str):
        self.http = httpx.AsyncClient(base_url=base_url)
        self.ws_url = base_url.replace("http", "ws")
    
    async def rest_call(self, path: str, data: dict) -> dict:
        """ 普通 REST 调用 """
        """ Regular REST call """
        resp = await self.http.post(path, json=data)
        return resp.json()
    
    async def stream_classification(self, data: dict):
        """ WebSocket 流式分类结果 """
        """ WebSocket streaming classification results """
        async with websockets.connect(f"{self.ws_url}/ws/classify") as ws:
            await ws.send(json.dumps(data))
            async for message in ws:
                yield json.loads(message)
```

### 18.2 SSE (Server-Sent Events) 支持 / SSE Support

```python
# httpx 原生支持 SSE（通过流式响应）
# httpx natively supports SSE (via streaming response)
import httpx
import asyncio

async def listen_sse(url: str, timeout: float = 60.0):
    """ 监听 SSE 事件流 """
    """ Listen to SSE event stream """
    async with httpx.AsyncClient(timeout=timeout) as client:
        async with client.stream("GET", url) as response:
            buffer = ""
            async for chunk in response.aiter_text():
                buffer += chunk
                while "\n\n" in buffer:
                    event_str, buffer = buffer.split("\n\n", 1)
                    event = parse_sse_event(event_str)
                    if event:
                        yield event


def parse_sse_event(raw: str) -> dict | None:
    """ 解析 SSE 事件 """
    """ Parse SSE event """
    event = {}
    for line in raw.strip().split("\n"):
        if line.startswith("data: "):
            event["data"] = line[6:]
        elif line.startswith("event: "):
            event["event"] = line[7:]
        elif line.startswith("id: "):
            event["id"] = line[4:]
    return event if event else None


# 使用示例：监听分类进度
# Usage: Listen to classification progress
async for event in listen_sse("http://localhost:8079/events/classify"):
    if event.get("event") == "progress":
        print(f"进度: {event['data']}")
    elif event.get("event") == "complete":
        print(f"完成: {event['data']}")
        break
```

### 18.3 长轮询模式 / Long Polling Pattern

```python
# 异步任务状态轮询（本项目分类异步 API）
# Async job status polling (project classification async API)
import httpx
import asyncio

async def poll_job_status(
    client: httpx.AsyncClient,
    job_id: str,
    interval: float = 1.0,
    max_wait: float = 300.0,
) -> dict:
    """ 轮询异步任务状态直到完成 """
    """ Poll async job status until complete """
    elapsed = 0.0
    
    while elapsed < max_wait:
        resp = await client.get(f"/api/v1/classify/async/{job_id}")
        result = resp.json()
        
        if result["status"] == "completed":
            return result["result"]
        elif result["status"] == "failed":
            raise RuntimeError(f"Job failed: {result['error']}")
        
        # 指数退避
        # Exponential backoff
        await asyncio.sleep(min(interval, 10.0))
        interval *= 1.5
        elapsed += interval
    
    raise TimeoutError(f"Job {job_id} timed out after {max_wait}s")
```

## 19. 安全最佳实践 / Security Best Practices

### 19.1 TLS 证书验证 / TLS Certificate Verification

```python
import ssl
import httpx
import certifi

# ✔ 生产环境：始终验证证书
# ✔ Production: Always verify certificates
client = httpx.AsyncClient(
    verify=True,  # 默认使用 certifi CA 包 / Uses certifi CA bundle
)

# ✔ 自定义 CA 证书（企业内部 CA）
# ✔ Custom CA certificate (enterprise internal CA)
client = httpx.AsyncClient(
    verify="/path/to/internal-ca-bundle.pem",
)

# ✔ 双向 TLS (mTLS)
# ✔ Mutual TLS (mTLS)
ssl_ctx = ssl.create_default_context(cafile="/path/to/ca.pem")
ssl_ctx.load_cert_chain(
    certfile="/path/to/client-cert.pem",
    keyfile="/path/to/client-key.pem",
)
client = httpx.AsyncClient(verify=ssl_ctx)

# ✘ 绝对不要在生产环境禁用验证
# ✘ NEVER disable verification in production
# client = httpx.AsyncClient(verify=False)  # 危险！/ DANGEROUS!
```

### 19.2 敏感信息保护 / Sensitive Data Protection

```python
# 请求头中的认证信息安全处理
# Secure handling of auth info in headers
import httpx
import os

class SecureClient:
    """ 安全客户端：不在日志中泄露凭证 """
    """ Secure client: no credential leakage in logs """
    
    def __init__(self, base_url: str):
        api_key = os.environ["PRIVACY_API_KEY"]
        
        self._client = httpx.AsyncClient(
            base_url=base_url,
            headers={
                "Authorization": f"Bearer {api_key}",
                "X-API-Key": api_key,
            },
            # 禁止重定向泄露凭证
            # Prevent redirect credential leakage
            follow_redirects=False,
        )
    
    async def request(self, method: str, url: str, **kwargs) -> httpx.Response:
        """ 包装请求，确保日志安全 """
        """ Wrap request, ensure log safety """
        # 日志中不记录完整 URL（可能含 token）
        # Don't log full URL (may contain token)
        logger.info(f"Request: {method} {url.split('?')[0]}")
        
        response = await self._client.request(method, url, **kwargs)
        
        # 日志中不记录响应体（可能含敏感数据）
        # Don't log response body (may contain sensitive data)
        logger.info(f"Response: {response.status_code}")
        
        return response
```

### 19.3 SSRF 防护 / SSRF Prevention

```python
# 防止服务端请求伪造 (SSRF)
# Prevent Server-Side Request Forgery (SSRF)
import ipaddress
import httpx
from urllib.parse import urlparse

BLOCKED_NETWORKS = [
    ipaddress.ip_network("127.0.0.0/8"),
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
    ipaddress.ip_network("169.254.0.0/16"),  # 链路本地 / Link-local
    ipaddress.ip_network("::1/128"),
]

def validate_url(url: str) -> bool:
    """ 验证 URL 不指向内部网络 """
    """ Validate URL doesn't point to internal networks """
    parsed = urlparse(url)
    
    # 仅允许 http/https
    # Only allow http/https
    if parsed.scheme not in ("http", "https"):
        return False
    
    # 解析主机名并检查 IP
    # Resolve hostname and check IP
    import socket
    try:
        ip = socket.gethostbyname(parsed.hostname)
        addr = ipaddress.ip_address(ip)
        for network in BLOCKED_NETWORKS:
            if addr in network:
                return False
    except (socket.gaierror, ValueError):
        return False
    
    return True


async def safe_request(url: str, **kwargs) -> httpx.Response:
    """ 安全请求：先验证 URL 再发送 """
    """ Safe request: validate URL before sending """
    if not validate_url(url):
        raise ValueError(f"URL blocked by SSRF policy: {url}")
    
    async with httpx.AsyncClient(follow_redirects=False) as client:
        return await client.get(url, **kwargs)
```

### 19.4 请求体大小限制 / Request Body Size Limits

```python
# 限制上传大小防止 DoS
# Limit upload size to prevent DoS
import httpx

MAX_BODY_SIZE = 10 * 1024 * 1024  # 10MB

async def safe_upload(client: httpx.AsyncClient, url: str, data: bytes):
    """ 安全上传：检查大小 """
    """ Safe upload: check size """
    if len(data) > MAX_BODY_SIZE:
        raise ValueError(
            f"Body too large: {len(data)} > {MAX_BODY_SIZE}"
        )
    
    return await client.post(
        url,
        content=data,
        headers={"Content-Type": "application/octet-stream"},
    )
```

### 19.5 安全配置检查清单 / Security Configuration Checklist

| 检查项 / Check Item | 状态 / Status | 说明 / Description |
|---|---|---|
| TLS 证书验证 / TLS cert verify | ✅ 始终开启 / Always on | verify=True 或自定义 CA |
| 禁止跟随重定向 / No follow redirects | ✅ 配置 / Configured | 防止凭证泄露 / Prevent cred leak |
| SSRF 防护 / SSRF protection | ✅ URL 验证 / URL validation | 禁止内网 IP / Block internal IPs |
| 超时配置 / Timeout config | ✅ 严格设置 / Strict | 防止慢速攻击 / Prevent slowloris |
| 日志脱敏 / Log sanitization | ✅ 无凭证 / No creds | 不记录 Auth 头 / No Auth header |
| 连接池限制 / Pool limits | ✅ 配置 / Configured | 防止资源耗尽 / Prevent exhaustion |

## 20. gRPC-Web 与协议桥接 / gRPC-Web & Protocol Bridging

在微服务架构中，经常需要在不同协议之间进行桥接。httpx 作为 HTTP 客户端，可以与 gRPC-Web 代理配合，实现浏览器到 gRPC 服务的通信，或者作为 REST-to-gRPC 网关的底层传输。

In microservice architectures, bridging between different protocols is often necessary. httpx, as an HTTP client, can work with gRPC-Web proxies to enable browser-to-gRPC communication, or serve as the underlying transport for REST-to-gRPC gateways.

### 20.1 gRPC-Web 协议桥接 / gRPC-Web Protocol Bridge

```python
"""gRPC-Web 协议桥接器 / gRPC-Web protocol bridge

gRPC-Web 协议特点 / gRPC-Web protocol characteristics:
- 基于 HTTP/1.1 或 HTTP/2 / Based on HTTP/1.1 or HTTP/2
- 使用 application/grpc-web 内容类型 / Uses application/grpc-web content type
- 支持一元和服务器流 / Supports unary and server streaming
- 不支持客户端流和双向流 / No client/bidirectional streaming
"""
import httpx
import struct
import base64
from dataclasses import dataclass
from typing import Any


@dataclass
class GrpcWebFrame:
    """gRPC-Web 帧 / gRPC-Web frame"""
    is_trailer: bool      # 是否为 trailer 帧 / Whether trailer frame
    payload: bytes        # 负载 / Payload


class GrpcWebBridge:
    """通过 httpx 实现 gRPC-Web 调用 / gRPC-Web calls via httpx
    
    架构 / Architecture:
    Browser/Client ──(HTTP)──▶ Envoy/Proxy ──(gRPC)──▶ gRPC Server
    
    本类实现客户端侧 / This class implements client side
    """
    
    def __init__(self, base_url: str, timeout: float = 30.0):
        self.client = httpx.AsyncClient(
            base_url=base_url,
            timeout=timeout,
            headers={
                "Content-Type": "application/grpc-web+proto",
                "Accept": "application/grpc-web+proto",
            },
        )
    
    def _encode_frame(self, data: bytes, is_trailer: bool = False) -> bytes:
        """编码 gRPC-Web 帧 / Encode gRPC-Web frame
        
        帧格式 / Frame format:
        [1 byte flags][4 bytes length][payload]
        - flags: 0x00 = data, 0x80 = trailer
        """
        flags = 0x80 if is_trailer else 0x00
        length = len(data)
        return struct.pack(">BI", flags, length) + data
    
    def _decode_frames(self, data: bytes) -> list[GrpcWebFrame]:
        """解码 gRPC-Web 响应帧 / Decode gRPC-Web response frames"""
        frames = []
        offset = 0
        
        while offset < len(data):
            if offset + 5 > len(data):
                break
            
            flags = data[offset]
            length = struct.unpack(">I", data[offset+1:offset+5])[0]
            offset += 5
            
            payload = data[offset:offset+length]
            offset += length
            
            frames.append(GrpcWebFrame(
                is_trailer=bool(flags & 0x80),
                payload=payload,
            ))
        
        return frames
    
    async def unary_call(self, method: str, request_bytes: bytes) -> bytes:
        """一元调用 / Unary call
        
        Args:
            method: gRPC 方法路径 / gRPC method path (e.g. /privacy.PrivacyService/Mask)
            request_bytes: 序列化的请求 / Serialized request
        
        Returns:
            序列化的响应 / Serialized response
        """
        # 编码请求帧 / Encode request frame
        body = self._encode_frame(request_bytes)
        
        response = await self.client.post(
            method,
            content=body,
        )
        
        # 检查 gRPC 状态 / Check gRPC status
        grpc_status = response.headers.get("grpc-status", "0")
        if grpc_status != "0":
            grpc_message = response.headers.get("grpc-message", "Unknown error")
            raise RuntimeError(f"gRPC error {grpc_status}: {grpc_message}")
        
        # 解码响应帧 / Decode response frames
        frames = self._decode_frames(response.content)
        
        # 第一个 data 帧是响应 / First data frame is the response
        for frame in frames:
            if not frame.is_trailer:
                return frame.payload
        
        raise RuntimeError("No response frame received")
    
    async def close(self):
        await self.client.aclose()


# 使用示例 / Usage example
async def call_privacy_service():
    """调用隐私服务 gRPC 方法 / Call privacy service gRPC method"""
    bridge = GrpcWebBridge("http://localhost:8080")  # Envoy proxy
    
    try:
        # 序列化请求（实际使用 protobuf）/ Serialize request (use protobuf in practice)
        request_data = b'\x0a\x05hello'  # 简化的 protobuf / Simplified protobuf
        
        response_data = await bridge.unary_call(
            "/privacy.PrivacyService/Mask",
            request_data,
        )
        
        # 反序列化响应 / Deserialize response
        print(f"Response: {response_data}")
    finally:
        await bridge.close()
```

### 20.2 REST-to-gRPC 网关 / REST-to-gRPC Gateway

```python
"""通过 httpx 实现 REST-to-gRPC 网关 / REST-to-gRPC gateway via httpx

场景 / Scenario:
前端发送 REST 请求 → 网关转换为 gRPC → 后端 gRPC 服务
Frontend sends REST → Gateway converts to gRPC → Backend gRPC service

本示例展示网关侧的 httpx 使用 / Shows gateway-side httpx usage
"""
import httpx
import json
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

app = FastAPI(title="REST-gRPC Gateway")

# gRPC-Web 代理客户端 / gRPC-Web proxy client
grpc_client = httpx.AsyncClient(
    base_url="http://grpc-proxy:8080",
    timeout=30.0,
    headers={"Content-Type": "application/grpc-web+proto"},
)


@app.post("/api/v1/mask")
async def rest_to_grpc_mask(request: Request):
    """将 REST 请求转换为 gRPC 调用 / Convert REST request to gRPC call
    
    REST: POST /api/v1/mask {"data": {...}}
    gRPC: /privacy.PrivacyService/Mask
    """
    body = await request.json()
    
    # 1. REST JSON → Protobuf 序列化 / REST JSON → Protobuf serialization
    # 实际使用 protobuf 库 / Use protobuf library in practice
    grpc_request = _json_to_protobuf(body)
    
    # 2. 通过 httpx 发送 gRPC-Web 请求 / Send gRPC-Web request via httpx
    response = await grpc_client.post(
        "/privacy.PrivacyService/Mask",
        content=grpc_request,
    )
    
    # 3. Protobuf → REST JSON / Protobuf → REST JSON
    grpc_response = _protobuf_to_json(response.content)
    
    return JSONResponse(content=grpc_response)


def _json_to_protobuf(data: dict) -> bytes:
    """简化的 JSON 到 Protobuf 转换 / Simplified JSON to Protobuf"""
    # 实际使用 google.protobuf.json_format / Use json_format in practice
    return json.dumps(data).encode()


def _protobuf_to_json(data: bytes) -> dict:
    """简化的 Protobuf 到 JSON 转换 / Simplified Protobuf to JSON"""
    return json.loads(data.decode())
```

### 20.3 协议桥接模式对比 / Protocol Bridging Pattern Comparison

| 模式 / Pattern | 工具 / Tools | 优势 / Pros | 劣势 / Cons | 适用 / Suitable |
|---|---|---|---|---|
| gRPC-Web + Envoy | Envoy proxy | 成熟稳定 / Mature | 额外代理 / Extra proxy | 浏览器 / Browser |
| Connect Protocol | connect-go | 原生 HTTP / Native HTTP | 新生态 / New ecosystem | 新项目 / New projects |
| REST 网关 / REST gateway | grpc-gateway | 简单 / Simple | 性能损失 / Perf loss | 兼容旧系统 / Legacy |
| httpx 桥接 / httpx bridge | httpx | 灵活 / Flexible | 手动编码 / Manual encoding | 自定义 / Custom |
| 双协议服务 / Dual protocol | FastAPI + gRPC | 无代理 / No proxy | 复杂度高 / Complex | 内部服务 / Internal |

## 21. 分布式追踪与请求关联 / Distributed Tracing & Request Correlation

分布式追踪是理解微服务间调用链路的关键技术。httpx 作为 HTTP 客户端，需要正确传播追踪上下文（Trace Context），确保跨服务调用能被关联到同一条追踪链路。

Distributed tracing is key to understanding call chains across microservices. httpx, as an HTTP client, must correctly propagate trace context to ensure cross-service calls are correlated to the same trace.

### 21.1 W3C Trace Context 传播 / W3C Trace Context Propagation

```python
"""分布式追踪上下文传播 / Distributed tracing context propagation

W3C Trace Context 标准 / W3C Trace Context standard:
- traceparent: 00-{trace-id}-{span-id}-{flags}
  - trace-id: 16 字节 (32 hex) / 16 bytes
  - span-id: 8 字节 (16 hex) / 8 bytes
  - flags: 01 = sampled / 01 = sampled
- tracestate: 厂商特定数据 / Vendor-specific data
"""
import httpx
import uuid
import os
from contextvars import ContextVar
from dataclasses import dataclass


@dataclass
class TraceContext:
    """追踪上下文 / Trace context"""
    trace_id: str       # 32 hex chars
    span_id: str        # 16 hex chars
    parent_span_id: str = ""
    sampled: bool = True
    
    @classmethod
    def new(cls) -> "TraceContext":
        """创建新的追踪上下文 / Create new trace context"""
        return cls(
            trace_id=uuid.uuid4().hex,
            span_id=uuid.uuid4().hex[:16],
        )
    
    @classmethod
    def from_headers(cls, headers: dict) -> "TraceContext | None":
        """从 HTTP 头解析 / Parse from HTTP headers"""
        traceparent = headers.get("traceparent", "")
        if not traceparent:
            return None
        
        parts = traceparent.split("-")
        if len(parts) != 4:
            return None
        
        return cls(
            trace_id=parts[1],
            span_id=parts[2],
            sampled=parts[3] == "01",
        )
    
    def child_span(self) -> "TraceContext":
        """创建子 span / Create child span"""
        return TraceContext(
            trace_id=self.trace_id,
            span_id=uuid.uuid4().hex[:16],
            parent_span_id=self.span_id,
            sampled=self.sampled,
        )
    
    def to_headers(self) -> dict[str, str]:
        """转换为 HTTP 头 / Convert to HTTP headers"""
        flags = "01" if self.sampled else "00"
        return {
            "traceparent": f"00-{self.trace_id}-{self.span_id}-{flags}",
        }


# 当前追踪上下文 / Current trace context
current_trace: ContextVar[TraceContext | None] = ContextVar('trace', default=None)


class TracingTransport(httpx.AsyncBaseTransport):
    """追踪传输层：自动注入追踪头 / Tracing transport: auto-inject trace headers
    
    包装底层传输，在每个请求中自动添加 traceparent 头。
    Wraps underlying transport, auto-adds traceparent header to every request.
    """
    
    def __init__(self, transport: httpx.AsyncBaseTransport):
        self._transport = transport
    
    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        # 获取或创建追踪上下文 / Get or create trace context
        ctx = current_trace.get()
        if ctx is None:
            ctx = TraceContext.new()
            current_trace.set(ctx)
        
        # 创建子 span / Create child span
        child = ctx.child_span()
        
        # 注入追踪头 / Inject trace headers
        for key, value in child.to_headers().items():
            request.headers[key] = value
        
        # 添加请求 ID 以便日志关联 / Add request ID for log correlation
        request.headers["X-Request-ID"] = child.span_id
        
        # 发送请求 / Send request
        response = await self._transport.handle_async_request(request)
        
        # 将追踪信息添加到响应扩展 / Add trace info to response extensions
        response.extensions["trace_id"] = child.trace_id
        response.extensions["span_id"] = child.span_id
        
        return response


# 使用示例 / Usage example
async def traced_request():
    """带追踪的请求 / Traced request"""
    transport = TracingTransport(httpx.AsyncHTTPTransport())
    
    async with httpx.AsyncClient(transport=transport) as client:
        # 所有请求自动携带 traceparent / All requests auto-carry traceparent
        response = await client.get("http://privacy-agent:8079/api/v1/health")
        
        trace_id = response.extensions.get("trace_id")
        print(f"Trace ID: {trace_id}")
```

### 21.2 请求关联与日志集成 / Request Correlation & Log Integration

```python
"""请求关联与日志集成 / Request correlation & log integration

目标：通过 trace_id 将分布式系统中的所有日志关联起来。
Goal: Correlate all logs in distributed system via trace_id.
"""
import logging
import httpx
from contextvars import ContextVar

# 请求 ID 上下文 / Request ID context
request_id_ctx: ContextVar[str] = ContextVar('request_id', default='-')
trace_id_ctx: ContextVar[str] = ContextVar('trace_id', default='-')


class CorrelatedLogFormatter(logging.Formatter):
    """带关联 ID 的日志格式器 / Log formatter with correlation IDs"""
    
    def format(self, record: logging.LogRecord) -> str:
        # 注入上下文信息 / Inject context info
        record.request_id = request_id_ctx.get()
        record.trace_id = trace_id_ctx.get()
        
        return super().format(record)


class CorrelationMiddleware:
    """httpx 事件钩子：提取并设置关联 ID / httpx event hooks: extract & set correlation IDs"""
    
    @staticmethod
    async def on_request(request: httpx.Request):
        """请求发出前 / Before request sent"""
        # 确保有 request ID / Ensure request ID exists
        if "X-Request-ID" not in request.headers:
            import uuid
            request.headers["X-Request-ID"] = uuid.uuid4().hex[:12]
        
        request_id_ctx.set(request.headers["X-Request-ID"])
    
    @staticmethod
    async def on_response(response: httpx.Response):
        """响应接收后 / After response received"""
        # 从响应头提取追踪信息 / Extract trace info from response headers
        trace_id = response.headers.get("X-Trace-ID", "-")
        trace_id_ctx.set(trace_id)


# 配置带关联的客户端 / Configure correlated client
def create_correlated_client(**kwargs) -> httpx.AsyncClient:
    """创建带请求关联的客户端 / Create client with request correlation"""
    return httpx.AsyncClient(
        event_hooks={
            "request": [CorrelationMiddleware.on_request],
            "response": [CorrelationMiddleware.on_response],
        },
        **kwargs,
    )
```

### 21.3 追踪采样策略 / Trace Sampling Strategy

```python
"""追踪采样策略 / Trace sampling strategy

采样原因 / Why sampling:
- 生产环境流量巨大 / Production traffic is massive
- 全量追踪开销太高 / Full tracing overhead too high
- 大多数请求正常 / Most requests are normal

采样策略 / Sampling strategies:
1. 固定比率 / Fixed rate: 每 N 个请求采一个 / Sample 1 in N
2. 尾部采样 / Tail-based: 只保留异常/慢请求 / Keep only errors/slow
3. 优先级采样 / Priority: 特定用户/端点全采 / Full sample for specific users
"""
import random
import time
from dataclasses import dataclass


@dataclass
class SamplingDecision:
    """采样决策 / Sampling decision"""
    sampled: bool
    reason: str


class AdaptiveSampler:
    """自适应采样器 / Adaptive sampler
    
    根据错误率和延迟动态调整采样率。
    Dynamically adjusts sample rate based on error rate and latency.
    """
    
    def __init__(self, base_rate: float = 0.1, min_rate: float = 0.01, max_rate: float = 1.0):
        self.base_rate = base_rate
        self.min_rate = min_rate
        self.max_rate = max_rate
        self._current_rate = base_rate
        self._recent_errors = 0
        self._recent_total = 0
        self._window_start = time.time()
    
    def should_sample(self, endpoint: str = "", status_code: int = 200) -> SamplingDecision:
        """决定是否采样 / Decide whether to sample"""
        # 规则 1: 错误请求始终采样 / Rule 1: Always sample errors
        if status_code >= 500:
            return SamplingDecision(sampled=True, reason="error")
        
        # 规则 2: 健康检查不采样 / Rule 2: Never sample health checks
        if endpoint in ("/health", "/metrics", "/ready"):
            return SamplingDecision(sampled=False, reason="health_check")
        
        # 规则 3: 自适应采样 / Rule 3: Adaptive sampling
        self._update_window()
        
        if random.random() < self._current_rate:
            return SamplingDecision(sampled=True, reason="rate")
        
        return SamplingDecision(sampled=False, reason="dropped")
    
    def _update_window(self):
        """更新采样窗口 / Update sampling window"""
        now = time.time()
        if now - self._window_start > 60:  # 1分钟窗口 / 1-min window
            if self._recent_total > 0:
                error_rate = self._recent_errors / self._recent_total
                # 错误率高时提高采样率 / Increase rate when error rate high
                if error_rate > 0.05:
                    self._current_rate = min(self.max_rate, self._current_rate * 2)
                elif error_rate < 0.01:
                    self._current_rate = max(self.min_rate, self._current_rate * 0.5)
            
            self._recent_errors = 0
            self._recent_total = 0
            self._window_start = now
    
    def record_result(self, is_error: bool):
        """记录结果用于自适应 / Record result for adaptation"""
        self._recent_total += 1
        if is_error:
            self._recent_errors += 1
```

### 21.4 追踪工具链对比 / Tracing Toolchain Comparison

| 工具 / Tool | 协议 / Protocol | 存储 / Storage | UI | 适用 / Suitable |
|---|---|---|---|---|
| Jaeger | OpenTelemetry | ES/Cassandra | 内置 / Built-in | 通用 / General |
| Zipkin | Brave/OTel | MySQL/ES | 内置 / Built-in | 轻量 / Lightweight |
| Tempo | OTLP | 对象存储 / Object store | Grafana | 大规模 / Large scale |
| Datadog APM | 私有 / Proprietary | SaaS | SaaS | 企业 / Enterprise |
| 自建 / Self-built | W3C TC | 自定义 / Custom | 自定义 / Custom | 特殊需求 / Special |

## 22. API 客户端 SDK 设计模式 / API Client SDK Design Patterns

设计一个优秀的 API 客户端 SDK 需要考虑易用性、类型安全、错误处理、可扩展性等多个维度。httpx 的灵活架构使其成为构建 SDK 的理想底层传输。

Designing an excellent API client SDK requires considering usability, type safety, error handling, and extensibility. httpx's flexible architecture makes it an ideal underlying transport for building SDKs.

### 22.1 分层 SDK 架构 / Layered SDK Architecture

```python
"""分层 SDK 架构设计 / Layered SDK architecture design

架构层次 / Architecture layers:

┌─────────────────────────────────────┐
│  用户接口层 / User Interface Layer      │  ← 简洁 API / Clean API
├─────────────────────────────────────┤
│  业务逻辑层 / Business Logic Layer     │  ← 重试、转换 / Retry, transform
├─────────────────────────────────────┤
│  传输层 / Transport Layer             │  ← httpx / httpx
├─────────────────────────────────────┤
│  序列化层 / Serialization Layer        │  ← Pydantic / Pydantic
└─────────────────────────────────────┘
"""
import httpx
from pydantic import BaseModel
from typing import TypeVar, Generic, Type
from abc import ABC, abstractmethod

T = TypeVar('T', bound=BaseModel)


# === 序列化层 / Serialization Layer ===

class MaskRequest(BaseModel):
    """脱敏请求 / Mask request"""
    data: dict
    profile: str = "default"
    fields: list[str] | None = None


class MaskResponse(BaseModel):
    """脱敏响应 / Mask response"""
    masked_data: dict
    masked_fields: list[str]
    profile_used: str


class DPQueryRequest(BaseModel):
    """差分隐私查询请求 / DP query request"""
    query_type: str  # count | sum | mean
    epsilon: float
    data: list[float]
    clamp_min: float | None = None
    clamp_max: float | None = None


class DPQueryResponse(BaseModel):
    """差分隐私查询响应 / DP query response"""
    result: float
    epsilon: float
    mechanism: str
    noise_added: float
    budget_remaining: float


# === 传输层 / Transport Layer ===

class Transport(ABC):
    """传输抽象 / Transport abstraction"""
    
    @abstractmethod
    async def send(self, method: str, path: str, **kwargs) -> httpx.Response:
        ...
    
    @abstractmethod
    async def close(self):
        ...


class HttpxTransport(Transport):
    """httpx 传输实现 / httpx transport implementation"""
    
    def __init__(self, base_url: str, **client_kwargs):
        self._client = httpx.AsyncClient(
            base_url=base_url,
            timeout=30.0,
            **client_kwargs,
        )
    
    async def send(self, method: str, path: str, **kwargs) -> httpx.Response:
        return await self._client.request(method, path, **kwargs)
    
    async def close(self):
        await self._client.aclose()


# === 业务逻辑层 / Business Logic Layer ===

class PrivacySDKError(Exception):
    """SDK 统一异常 / SDK unified exception"""
    def __init__(self, message: str, status_code: int = 0, code: str = ""):
        super().__init__(message)
        self.status_code = status_code
        self.code = code


class BaseResource:
    """资源基类 / Resource base class"""
    
    def __init__(self, transport: Transport):
        self._transport = transport
    
    async def _request(
        self, method: str, path: str,
        response_model: Type[T], **kwargs
    ) -> T:
        """发送请求并解析响应 / Send request and parse response"""
        response = await self._transport.send(method, path, **kwargs)
        
        if response.status_code >= 400:
            error_data = response.json() if response.content else {}
            raise PrivacySDKError(
                message=error_data.get("detail", f"HTTP {response.status_code}"),
                status_code=response.status_code,
                code=error_data.get("code", ""),
            )
        
        return response_model.model_validate(response.json())


# === 用户接口层 / User Interface Layer ===

class MaskingResource(BaseResource):
    """脱敏资源 / Masking resource"""
    
    async def mask(self, data: dict, profile: str = "default") -> MaskResponse:
        """对数据进行脱敏 / Mask data
        
        Args:
            data: 待脱敏数据 / Data to mask
            profile: 脱敏配置 / Masking profile
        
        Returns:
            脱敏结果 / Masking result
        
        Example:
            >>> result = await client.masking.mask({"name": "张三", "age": 30})
            >>> print(result.masked_data)  # {"name": "张*", "age": 30}
        """
        request = MaskRequest(data=data, profile=profile)
        return await self._request(
            "POST", "/api/v1/mask",
            response_model=MaskResponse,
            json=request.model_dump(),
        )


class DPResource(BaseResource):
    """差分隐私资源 / Differential privacy resource"""
    
    async def query(self, query_type: str, data: list[float],
                    epsilon: float = 1.0) -> DPQueryResponse:
        """执行 DP 查询 / Execute DP query
        
        Args:
            query_type: 查询类型 (count/sum/mean) / Query type
            data: 数据集 / Dataset
            epsilon: 隐私参数 / Privacy parameter
        """
        request = DPQueryRequest(
            query_type=query_type, epsilon=epsilon, data=data
        )
        return await self._request(
            "POST", "/api/v1/dp/query",
            response_model=DPQueryResponse,
            json=request.model_dump(),
        )


class PrivacyClient:
    """隐私服务客户端 / Privacy service client
    
    Usage:
        async with PrivacyClient("http://localhost:8079") as client:
            result = await client.masking.mask({"email": "test@example.com"})
            dp_result = await client.dp.query("count", [1,2,3], epsilon=0.5)
    """
    
    def __init__(self, base_url: str, api_key: str | None = None, **kwargs):
        headers = {}
        if api_key:
            headers["X-API-Key"] = api_key
        
        self._transport = HttpxTransport(base_url, headers=headers, **kwargs)
        
        # 资源实例 / Resource instances
        self.masking = MaskingResource(self._transport)
        self.dp = DPResource(self._transport)
    
    async def __aenter__(self):
        return self
    
    async def __aexit__(self, *args):
        await self.close()
    
    async def close(self):
        await self._transport.close()
```

### 22.2 SDK 设计最佳实践 / SDK Design Best Practices

| 实践 / Practice | 说明 / Description | 示例 / Example |
|---|---|---|
| 上下文管理器 / Context manager | 确保资源释放 / Ensure resource cleanup | `async with Client() as c:` |
| 类型化响应 / Typed responses | Pydantic 模型 / Pydantic models | `-> MaskResponse` |
| 统一异常 / Unified exceptions | 单一异常层次 / Single exception hierarchy | `PrivacySDKError` |
| 合理默认值 / Sensible defaults | 减少配置 / Reduce configuration | `profile="default"` |
| 幂等操作 / Idempotent ops | 安全重试 / Safe retry | GET 请求 / GET requests |
| 分页支持 / Pagination | 自动翻页 / Auto-paging | `async for item in ...` |
| 版本化 / Versioning | API 版本前缀 / API version prefix | `/api/v1/...` |
| 文档字符串 / Docstrings | 内联示例 / Inline examples | `>>> result = ...` |
