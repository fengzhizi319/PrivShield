# 反向代理技术指南 / Reverse Proxy Technology Stack & Architectural Guide

## 1. 技术简介 / Introduction

**反向代理（Reverse Proxy）** 是现代化分布式微服务、API 网关及云原生架构中最为核心的网络中间件形态。与代表客户端访问外部网络的主动式“正向代理（Forward Proxy）”不同，反向代理部署在服务端边缘，代表后端服务器集群接收外部客户端请求，并根据路由规则、负载均衡算法、健康状态与安全策略将流量透明转发至最佳后端节点。

### 1.1 正向代理 vs 反向代理对比 / Forward vs Reverse Proxy

```text
1. 正向代理 (Forward Proxy - 代表客户端出海):
   客户端 A ──► [ 正向代理 (翻墙/企业出口网关) ] ──► 互联网目标服务器 B
   (目标服务器不知道真正的客户端是谁，仅感知正向代理 IP)

2. 反向代理 (Reverse Proxy - 代表服务端迎客):
   外部客户端 A ──► [ 反向代理 (PrivShield Gateway / Nginx) ] ──► 后端集群 (Worker 1 / 2 / 3)
   (客户端以为反向代理就是真实服务器，后端拓扑被完全屏蔽)
```

### 1.2 反向代理、API 网关、Ingress 与 Sidecar 的角色分工

| 架构形态 | 部署位置 | 核心职责 | 本项目对应实现 |
|---|---|---|---|
| **反向代理 (Reverse Proxy)** | 服务端网络边界 | 协议转发、连接池复用、头部清洗、故障转移 | [`engine/gateway/http_proxy.py`](engine/gateway/http_proxy.py) & [`grpc_proxy.py`](engine/gateway/grpc_proxy.py) |
| **BFF 协议转换网关** | 前端与核心服务之间 | REST JSON $\leftrightarrow$ gRPC Protobuf 协议转换与静态 SPA 托管 | [`console/bff-go/`](console/bff-go/) (:8081) |
| **服务治理 Sidecar** | 业务容器同 Pod/同机 | 隐私原语计算、动态分类分级、细粒度审计 | [`engine/server.py`](engine/server.py) (:8079 & :50051) |
| **前端开发代理** | 开发者本地电脑 | 解决本地开发跨域（CORS）与 HMR 转发 | [`console/web/vite.config.ts`](console/web/vite.config.ts) (:5173) |

---

## 2. 反向代理的核心底层技术机制 / Core Architectural Mechanisms

反向代理绝非简单的“收到请求后直接请求下游”。在生产级高可用场景中，反向代理需要解决一系列复杂的网络协议规范、连接生命周期与安全边界问题：

```text
                           客户端 HTTP / gRPC 请求
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │  ★ 1. 协议解析与连接保持 (HTTP/1.1, HTTP/2, gRPC)        │
        │     - 长连接复用 (Keep-Alive) 避免频繁 TCP 握手开销       │
        │     - 客户端直连 IP 提取 (Remote-Addr)                  │
        └────────────────────────────┬────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │  ★ 2. 入站头部清洗 (Hop-by-Hop & Security Headers)      │
        │     - 剥离 RFC 7230 逐段传输头 (Connection, Host, ...)  │
        │     - 链式追加 X-Forwarded-For, X-Real-IP, X-Request-ID │
        └────────────────────────────┬────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │  ★ 3. 动态选路与在途追踪 (LoadBalancer & CircuitBreaker)│
        │     - SWRR 平滑加权轮询 / 最小在途连接数 (LeastConn)    │
        │     - 节点熔断器 (Closed / Open / Half-Open) 过滤       │
        └────────────────────────────┬────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │  ★ 4. 代理转发与容错重试 (Idempotent Safe Retries)       │
        │     - 全局长连接池 (HTTPX AsyncClient / gRPC Channel)   │
        │     - ConnectError 安全重试 vs ReadTimeout 读超时中断   │
        └────────────────────────────┬────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │  ★ 5. 出站响应清洗 (Response Processing)                │
        │     - 剥离 content-encoding (杜绝客户端二次解包崩溃)    │
        │     - 透传 Initial/Trailing Metadata 与 Status Headers   │
        └─────────────────────────────────────────────────────────┘
```

---

### 2.1 RFC 7230 逐段传输头（Hop-by-Hop Headers）过滤

根据 HTTP/1.1 规范（RFC 7230 §6.1），HTTP 头部被划分为两类：
- **端到端头（End-to-End Headers）**：必须向最终接收方透传（如 `Authorization`, `Content-Type`, `User-Agent`）；
- **逐段传输头（Hop-by-Hop Headers）**：仅在当前单条传输链路生效，**严禁代理向下游转发**。

若反向代理盲目转发 `Connection: close` 或 `Transfer-Encoding: chunked`，会导致上游与下游协议状态机错位，引发连接提前中断或**HTTP 请求走私（Request Smuggling）**漏洞。

文件 / File：[`engine/gateway/http_proxy.py`](engine/gateway/http_proxy.py#L45-L65)

```python
# RFC 7230 规定的逐段传输头集合
EXCLUDE_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "content-length",
    "host",
}
```

---

### 2.2 响应端 `content-encoding` 剥离与二次解压崩溃陷阱

当反向代理使用高性能 HTTP 客户端（如 `httpx.AsyncClient`）向上游节点请求数据时，`httpx` 会根据上游返回的 `Content-Encoding: gzip` 自动在内存中完成流式解压并还原为原始明文字节流。

如果反向代理在向客户端构造响应时，将上游原始的 `content-encoding: gzip` 响应头原样传回客户端：
- **灾难后果**：下游客户端（浏览器或 Go 服务）看到 `content-encoding: gzip`，会尝试对**已经被代理层解压过的明文数据**再次执行 gzip 解压缩，立即抛出 `zlib.error: Error -3 while decompressing data: incorrect header check`，导致整个请求解析崩溃！
- **治理方案**：网关在出站响应头中显式剔除 `content-encoding`：

```python
RESPONSE_EXCLUDE_HEADERS = EXCLUDE_HEADERS | {"content-encoding"}

resp_headers = {
    k: v for k, v in backend_resp.headers.items() 
    if k.lower() not in RESPONSE_EXCLUDE_HEADERS
}
return Response(content=backend_resp.content, status_code=backend_resp.status_code, headers=resp_headers)
```

---

### 2.3 客户端真实 IP 透传标准 (`X-Forwarded-For` / `X-Real-IP`)

当请求经过反向代理后，后端服务器看到的 TCP 连接源 IP 会变成反向代理网关自身的 IP。为了让后端业务准确记录审计日志并执行 IP 级限流，反向代理必须规范化注入客户端真实 IP：

```python
# 提取客户端直连 IP
client_ip = request.client.host if request.client else "127.0.0.1"

# 规范化追加 X-Forwarded-For 链条
forwarded_for = request.headers.get("x-forwarded-for")
if forwarded_for:
    headers["x-forwarded-for"] = f"{forwarded_for}, {client_ip}"
else:
    headers["x-forwarded-for"] = client_ip

# 注入 X-Real-IP 与 X-Request-ID
headers["x-real-ip"] = client_ip
if "x-request-id" in request.headers:
    headers["x-request-id"] = request.headers["x-request-id"]
```

---

### 2.4 长连接池复用与事件循环漂移自愈（Event Loop Drift）

在高吞吐（>5000 QPS）代理场景下，若每次代理请求都临时新建 HTTP 客户端（`async with httpx.AsyncClient()`），会导致每秒产生数千个 TCP 握手与挥手。操作系统中将堆积海量处于 `TIME_WAIT` 状态的套接字，最终导致源端口耗尽抛出 `OSError: Cannot assign requested address`。

#### (1) 应用级长连接池单例
网关在应用启动阶段初始化全局连接池，配置 Keep-Alive 复用与连接数上限：
```python
app.state.http_client = httpx.AsyncClient(
    timeout=httpx.Timeout(30.0),
    limits=httpx.Limits(
        max_keepalive_connections=100,
        max_connections=500,
        keepalive_expiry=30.0,
    ),
    verify=backend_tls_verify(),
)
```

#### (2) 事件循环漂移自愈机制 (Event Loop Drift Recovery)
在 Python 异步运行环境中（尤其是结合 Pytest 测试夹具或 ASGI 进程热重载时），全局单例客户端绑定的事件循环可能与当前处理请求的协程事件循环不一致，触发致命的 `RuntimeError: Event loop is closed`。

`PrivShield` 网关实现了自愈式检测器：
```python
def _get_http_client(request: Request) -> httpx.AsyncClient:
    current_loop = asyncio.get_running_loop()
    client = getattr(request.app.state, "http_client", None)
    cached_loop = getattr(request.app.state, "http_client_loop", None)

    # 检测是否发生事件循环漂移或客户端异常关闭
    if client is None or client.is_closed or cached_loop is not current_loop:
        if client is not None and not client.is_closed:
            # 异步安全释放旧循环中的客户端
            asyncio.create_task(client.aclose())
        # 在当前活动循环中重建客户端
        client = httpx.AsyncClient(
            timeout=httpx.Timeout(30.0),
            limits=httpx.Limits(max_keepalive_connections=100, max_connections=500),
            verify=backend_tls_verify(),
        )
        request.app.state.http_client = client
        request.app.state.http_client_loop = current_loop
    return client
```

---

### 2.5 幂等性与安全重试决策边界 / Idempotent Retry Safety

反向代理在遭遇网络抖动时支持故障转移（Failover），但必须严格遵守 **HTTP 幂等性（Idempotency）安全底线**：

| 操作类型 | 异常场景 | 是否允许故障转移重试 | 决策 rationale |
|---|---|---|---|
| **GET / HEAD / OPTIONS** | 连接失败 (`ConnectError`) 或 读超时 (`ReadTimeout`) | **允许 (最多 3 次)** | 幂等只读查询，无任何后端状态变更副作用。 |
| **POST /v1/privacy/dp/spend** (非幂等扣减) | 握手连接失败 (`ConnectError` / `ConnectTimeout`) | **允许** | TCP 握手尚未建立，请求 100% 未到达后端，无副作用。 |
| **POST /v1/privacy/dp/spend** (非幂等扣减) | 读取响应超时 (`ReadTimeout`) | **严禁重试** | 请求已经送达后端并可能已执行预算扣减，盲目重发会导致**双重扣减**！直接向客户端返回 504 Gateway Timeout。 |

```python
try:
    backend_resp = await client.request(...)
except (httpx.ConnectError, httpx.ConnectTimeout) as exc:
    # 仅在确认网络层尚未建连时，才将该节点标记为故障并重试其他节点
    balancer.mark_unhealthy(node, reason="connect_error")
    retry_node = balancer.select_http_node(exclude={node})
    ...
```

---

## 3. 在 PrivShield 中的多场景反向代理实战 / Real-World Practices in PrivShield

### 3.1 REST 通配反向代理网关实现

文件 / File：[`engine/gateway/http_proxy.py`](engine/gateway/http_proxy.py)

```python
@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"])
async def proxy_http(request: Request, path: str):
    """通配反向代理入口：单次读取 Body，选路并转发。"""
    client = _get_http_client(request)
    body = await request.body()  # 缓存 Body 供重试复用

    node = balancer.select_http_node()
    if not node:
        raise HTTPException(status_code=503, detail="No healthy backend nodes available")

    url = f"{node.http_url.rstrip('/')}/{path}"
    headers = _clean_request_headers(request)

    start_time = time.time()
    with node.track_connection():
        backend_resp = await client.request(
            method=request.method,
            url=url,
            content=body,
            headers=headers,
            params=request.query_params,
        )

    # 记录 Prometheus 代理耗时
    GATEWAY_LATENCY.labels(protocol="http").observe(time.time() - start_time)
    
    # 响应头剥离 content-encoding 后返回
    resp_headers = _clean_response_headers(backend_resp)
    return Response(content=backend_resp.content, status_code=backend_resp.status_code, headers=resp_headers)
```

---

### 3.2 gRPC 泛化动态反射反向代理

文件 / File：[`engine/gateway/grpc_proxy.py`](engine/gateway/grpc_proxy.py)

传统 gRPC 代理需要为每个 RPC 方法手写转发代码。`PrivShield` 网关通过**类级别动态反射（Reflection Binding）**，在启动时自动提取 `PrivacyServiceServicer` 的所有公开 RPC 方法并挂载全双工转发闭包：

```python
class GatewayGrpcServicer(privacy_pb2_grpc.PrivacyServiceServicer):
    """gRPC 网关泛化服务：自动反射绑定所有 RPC 方法。"""

    def _bind_generic_methods(self) -> None:
        base = privacy_pb2_grpc.PrivacyServiceServicer
        for name in dir(base):
            if name.startswith("_") or name in ("__init__", "_forward"):
                continue
            attr = getattr(base, name)
            if callable(attr):
                # 为每个 RPC 动态绑定统一的转发协程
                setattr(self, name, self._make_forwarder(name))

    async def _forward(self, method_name: str, request, context):
        """底层转发逻辑：双向透传 invocation metadata 与 trailing metadata。"""
        node = self.balancer.select_node()
        stub_method = getattr(node.grpc_stub, method_name)

        # 提取入站 metadata
        inv_metadata = context.invocation_metadata()

        # 异步发起 gRPC 调用并透传 metadata
        call = stub_method(request, metadata=inv_metadata, timeout=30.0)
        response = await call

        # 出站回传 initial 与 trailing metadata
        await context.send_initial_metadata(await call.initial_metadata())
        context.set_trailing_metadata(await call.trailing_metadata())
        return response
```

---

### 3.3 Go BFF 协议转换代理网关 (REST $\leftrightarrow$ gRPC)

文件 / File：[`console/bff-go/internal/agent/client.go`](console/bff-go/internal/agent/client.go) & [`console/bff-go/internal/mapper/mapper.go`](console/bff-go/internal/mapper/mapper.go)

在企业控制台与外部微服务调用中，前端发送标准 HTTP/JSON 请求，Go BFF 充当**协议转换反向代理**：
1. 接收 HTTP 请求并解析 JSON 载荷；
2. 反序列化为对应 Protobuf 结构体；
3. 通过 HTTP/2 gRPC 复用长连接发起跨语言调用；
4. 将 Protobuf 响应编码回 JSON 返回给客户端；
5. 同时托管 React 编译后的单页应用（SPA）静态资源。

---

### 3.4 前端 Vite 开发服务器反向代理

文件 / File：[`console/web/vite.config.ts`](console/web/vite.config.ts)

在前端本地开发阶段，前端代码运行在 `http://localhost:5173`，若直接请求后端 `http://localhost:8081` 会受到浏览器同源策略（SOP）限制触发跨域（CORS）报错。

Vite 内置的 Node.js 反向代理在本地充当桥梁：
```typescript
export default defineConfig({
  server: {
    port: 5173,
    proxy: {
      // 捕获所有 /api/* 请求，在后台透明反向代理至 Go BFF
      '/api': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
        rewrite: (path) => path,
      },
    },
  },
})
```

---

## 4. 生产安全防护与避坑指南 / Production Security & Pitfalls

1. **服务端请求伪造（SSRF）与动态注册拦截**：
   - 动态节点注册接口（`/v1/gateway/register`）必须配置高权限 API Key 鉴权（`Fail-Closed`），并校验目标地址是否合法，禁止攻击者将反向代理节点指向云厂商元数据地址（`http://169.254.169.254`）。
2. **大包传输与 413 Payload Too Large**：
   - 反向代理与后端服务的消息体上限必须严格对齐。`PrivShield` 在 REST（32MB）与 gRPC（64MB）上保持双端配置一致，防止在网关层被阶段性截断。
3. **HTTP 协议混淆与走私防护**：
   - 坚决过滤 `Transfer-Encoding` 与不合法的双重 `Content-Length`，交由 Uvicorn / Gin 的严密 HTTP 解析器把关。
