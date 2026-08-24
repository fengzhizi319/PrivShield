# FastAPI 技术栈说明 / FastAPI Technology Stack

## 1. 技术简介 / Introduction

FastAPI 是一个现代、高性能的 Python Web 框架，用于构建 API。基于 Python 类型提示与 Pydantic 数据校验。
FastAPI is a modern, high-performance Python web framework for building APIs, based on Python type hints and Pydantic data validation.

核心特性 / Core Features：
- **高性能（High Performance）**：基于 Starlette（ASGI）+ Uvicorn，性能媲美 Node.js 与 Go。
- **自动数据校验（Auto Validation）**：请求/响应自动经 Pydantic 校验，减少 40%+ 的 bug。
- **自动文档（Auto Docs）**：内置 Swagger UI（/docs）与 ReDoc（/redoc）交互式 API 文档。
- **异步优先（Async First）**：原生支持 async/await，适合 I/O 密集型代理场景。
- **类型驱动（Type Driven）**：利用 Python 类型提示实现序列化、校验与文档生成。

本项目使用版本 / Version Used：`fastapi >= 0.110.0`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 应用创建与生命周期 / App Creation & Lifespan

文件 / File：`console/bff-py/app/main.py`

```python
from contextlib import asynccontextmanager
from fastapi import FastAPI

@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期：启动时预热连接池，关闭时释放资源。
    App lifespan: warm up connection pool on start, release on shutdown."""
    _ = await agent_client._get_client()  # 预热 httpx 连接池 / Warm up httpx pool
    yield
    if agent_client._client is not None:
        await agent_client._client.aclose()  # 优雅关闭 / Graceful close

app = FastAPI(title="Privacy Test Console", lifespan=lifespan)
```

### 2.2 请求/响应模型 / Request/Response Models

```python
from pydantic import BaseModel, Field

class ProxyRequest(BaseModel):
    """通用代理请求体 / Generic proxy request body"""
    method: str = Field(..., examples=["POST"])
    path: str = Field(..., examples=["/v1/privacy/mask"])
    body: dict[str, Any] | None = Field(default=None)
    raw_payload_b64: str | None = Field(default=None)
    content_type: str | None = Field(default=None)

class ProxyResponse(BaseModel):
    """统一响应包装 / Unified response wrapper"""
    status: int
    duration_ms: float
    data: Any
    via: str = Field(default="python-rest")
    protocol: str = Field(default="REST")
```

### 2.3 路由定义 / Route Definitions

```python
@app.get("/api/health")
async def health():
    """健康检查：返回后端与 agent 的连通性。
    Health check: returns backend and agent connectivity."""
    ...

@app.post("/api/proxy")
async def proxy(req: ProxyRequest):
    """通用代理：转发请求到 PrivShield。
    Generic proxy: forward request to PrivShield."""
    ...

@app.post("/api/batch")
async def batch(req: BatchRequest):
    """批量测试：逐个转发并汇总统计。
    Batch test: forward sequentially and aggregate stats."""
    ...
```

### 2.4 中间件 / Middleware

```python
# CORS 中间件：允许 Vite 开发服务器跨域调用
# CORS middleware: allow Vite dev server cross-origin calls
app.add_middleware(CORSMiddleware, allow_origins=["*"], ...)

# 安全中间件：可选 API Key 鉴权 + 限流
# Security middleware: optional API Key auth + rate limiting
app.add_middleware(ConsoleSecurityMiddleware, api_key=..., rate_limit=...)
```

### 2.5 静态文件托管 / Static File Serving

```python
# SPA 托管：/assets 静态资源 + 其余路径回退 index.html
# SPA hosting: /assets static files + fallback to index.html
app.mount("/assets", StaticFiles(directory="..."), name="assets")

@app.get("/{full_path:path}")
async def serve_spa(request: Request, full_path: str):
    return FileResponse(index_file, headers={"Cache-Control": "no-cache"})
```

### 2.6 异常处理 / Exception Handling

```python
@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    """统一错误格式：{"detail": ..., "status": ...}
    Unified error format for frontend parsing."""
    return JSONResponse(status_code=exc.status_code, content={...})
```

### 2.7 安全中间件详解 / Security Middleware Details

文件 / File：`console/bff-py/app/security.py`

本项目实现了基于 Starlette `BaseHTTPMiddleware` 的安全中间件：
This project implements a security middleware based on Starlette `BaseHTTPMiddleware`:

```python
class ConsoleSecurityMiddleware(BaseHTTPMiddleware):
    """可选的 API Key 鉴权 + 限流中间件（默认关闭 / 宽松）。
    Optional API Key auth + rate limiting middleware (disabled/relaxed by default)."""

    async def dispatch(self, request: Request, call_next) -> Response:
        # 1. CORS 预检直接放行 / CORS preflight passes through
        if request.method == "OPTIONS":
            return await call_next(request)

        # 2. 仅对 /api/* 生效；/api/health 豁免 / Only /api/*; health exempt
        if not path.startswith("/api/") or path == "/api/health":
            return await call_next(request)

        # 3. API Key 鉴权（常量时间比较防时序攻击）/ API Key auth (constant-time compare)
        if self._api_key is not None:
            token = _extract_bearer(request.headers.get("authorization"))
            if token is None or not hmac.compare_digest(token, self._api_key):
                return JSONResponse(status_code=401, content={"detail": "Unauthorized"})

        # 4. 滑动窗口限流 / Sliding window rate limiting
        if self._rate_limited(self._client_ip(request)):
            return JSONResponse(status_code=429, content={"detail": "Too many requests"})

        return await call_next(request)
```

安全设计要点 / Security Design Highlights：

| 特性 / Feature | 实现 / Implementation |
|---|---|
| 时序攻击防护 / Timing attack protection | `hmac.compare_digest()` 常量时间比较 / Constant-time comparison |
| 滑动窗口限流 / Sliding window rate limit | `deque` 记录 60s 内时间戳 / Timestamps in deque within 60s |
| 健康检查豁免 / Health check exemption | 便于 K8s/负载均衡器探测 / Facilitate K8s/LB probing |
| 默认关闭 / Disabled by default | 未配置环境变量时完全放行 / Fully permissive when unconfigured |

### 2.8 依赖注入与测试 / Dependency Injection & Testing

```python
# 测试中使用 AsyncMock 打桩客户端，无需真实 agent / Mock client in tests, no real agent needed
from unittest.mock import AsyncMock, patch

@pytest.fixture
def mock_client():
    with patch("app.main.agent_client") as m:
        m.request = AsyncMock(return_value={"result": "ok"})
        yield m

# TestClient 同步测试异步路由 / TestClient tests async routes synchronously
from fastapi.testclient import TestClient
client = TestClient(app)
resp = client.get("/api/health")
assert resp.status_code == 200
```

### 2.9 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 异步路由（async def）| 代理转发为 I/O 密集，异步提升并发 / Proxy is I/O bound, async improves concurrency |
| Pydantic v2 模型 | 自动校验 + 序列化 + 文档生成 / Auto validation + serialization + docs |
| lifespan 管理连接池 | 避免首请求延迟 + 优雅释放资源 / Avoid first-request latency + graceful release |
| 统一异常处理器 | 前端可依赖固定错误结构解析 / Frontend can rely on fixed error structure |
| hmac.compare_digest | 防止时序攻击泄露 API Key 前缀 / Prevent timing attack leaking API Key prefix |
| 进程内 deque 限流 | 单进程场景无需 Redis，简化部署 / Single process needs no Redis |
| BaseHTTPMiddleware | Starlette 原生支持，无需第三方库 / Starlette native, no third-party lib |

### 2.10 请求生命周期 / Request Lifecycle

```text
┌─────────────────────────────────────────────────────────────┐
│  1. HTTP 请求到达 / HTTP Request arrives                     │
│     Uvicorn 接收并解析 / Uvicorn receives and parses          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. 中间件链（洋葱模型）/ Middleware chain (onion model)      │
│     CORSMiddleware → SecurityMiddleware → ...                │
│     请求向内传递，响应向外返回                            │
│     Request passes inward, response returns outward           │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 路由匹配 / Route matching                                │
│     FastAPI 根据 method + path 查找处理器                  │
│     FastAPI finds handler by method + path                    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  4. 依赖解析 / Dependency resolution                         │
│     - 请求体解析 + Pydantic 校验 / Body parse + validation   │
│     - 路径/查询参数提取 / Path/query param extraction        │
│     - 依赖注入 / Dependency injection                        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  5. 处理器执行 / Handler execution                           │
│     async def proxy(req: ProxyRequest): ...                  │
│     - 业务逻辑 / Business logic                              │
│     - httpx 转发 / httpx forwarding                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  6. 响应序列化 / Response serialization                      │
│     - Pydantic 模型 → JSON / Pydantic model → JSON           │
│     - 状态码设置 / Status code setting                       │
│     - 响应头添加 / Response headers                          │
└─────────────────────────────────────────────────────────────┘
```

### 2.11 中间件执行顺序 / Middleware Execution Order

```python
# 中间件注册顺序（后注册的先执行）/ Registration order (last registered executes first)
app.add_middleware(CORSMiddleware, ...)        # 2️⃣ 第二执行 / Executes second
app.add_middleware(ConsoleSecurityMiddleware)  # 1️⃣ 第一执行 / Executes first
```

```text
请求流向 / Request flow:
Client → SecurityMiddleware → CORSMiddleware → Router → Handler
响应流向 / Response flow:
Handler → Router → CORSMiddleware → SecurityMiddleware → Client

注意：FastAPI/Starlette 中间件是“后注册先执行”（洋葱模型）
Note: FastAPI/Starlette middleware is "last registered, first executed" (onion model)
```

### 2.12 OpenAPI 自动文档 / OpenAPI Auto Documentation

FastAPI 自动生成符合 OpenAPI 3.1.0 规范的 API 文档：

```python
# 访问自动文档 / Access auto-generated docs
# http://127.0.0.1:8080/docs   → Swagger UI（交互式）/ Interactive
# http://127.0.0.1:8080/redoc  → ReDoc（只读）/ Read-only
# http://127.0.0.1:8080/openapi.json → 原始 JSON / Raw JSON

# 文档信息来源 / Documentation sources:
app = FastAPI(
    title="Privacy Test Console",     # API 标题 / API title
    description="测试控制台后端",      # 描述 / Description
    version="0.1.0",                  # 版本 / Version
)

# 每个路由的文档 / Per-route documentation:
@app.post("/api/proxy", summary="代理转发", description="转发请求到 agent")
async def proxy(req: ProxyRequest):
    """Docstring 也会显示在文档中 / Docstring also shows in docs"""
    ...
```

**文档生成流程 / Documentation Generation Flow**：

```text
Pydantic 模型 / Pydantic Models      路由装饰器 / Route Decorators
       │                                      │
       └──────────────┬──────────────────┘
                      │
                      ▼
        FastAPI OpenAPI Generator
                      │
                      ▼
        openapi.json (OpenAPI 3.1.0)
                      │
          ┌───────────┴───────────┐
          ▼                       ▼
    /docs (Swagger UI)      /redoc (ReDoc)
```

### 2.13 异步与同步路由 / Async vs Sync Routes

| 路由类型 / Route Type | 适用场景 / Use Case | 执行方式 / Execution |
|---|---|---|
| `async def` | I/O 密集（HTTP 调用、DB 查询）/ I/O bound | 事件循环直接执行 / Event loop direct |
| `def` (sync) | CPU 密集（计算、图像处理）/ CPU bound | 线程池执行 / Thread pool execution |

```python
# 本项目所有路由均为 async（代理场景为 I/O 密集）
# All routes in this project are async (proxy is I/O bound)
@app.post("/api/proxy")
async def proxy(req: ProxyRequest):  # async 路由 / async route
    response = await agent_client.request(...)  # 非阻塞 I/O / Non-blocking I/O
    return response
```

### 2.14 性能特征 / Performance Characteristics

| 指标 / Metric | 值 / Value | 说明 / Description |
|---|---|---|
| 空路由延迟 / Empty route latency | ~0.1ms | 仅框架开销 / Framework overhead only |
| JSON 序列化 / JSON serialization | ~0.05ms | Pydantic v2 Rust 核心 / Rust core |
| 代理转发延迟 / Proxy forwarding latency | ~5-50ms | 取决于 agent 响应 / Depends on agent |
| 并发连接数 / Concurrent connections | 1000+ | 单进程异步 / Single async process |
| 内存占用 / Memory footprint | ~50MB | 含 Python 运行时 / Including Python runtime |

## 3. 后台任务与异步模式 / Background Tasks & Async Patterns

### 3.1 BackgroundTasks 机制 / BackgroundTasks Mechanism

```python
from fastapi import BackgroundTasks, FastAPI

app = FastAPI()

async def log_request(method: str, path: str, duration_ms: float):
    """后台记录请求日志（不阻塞响应返回）。
    Log request in background (doesn't block response)."""
    # 异步写入日志/数据库 / Async write to log/database
    print(f"[{method}] {path} - {duration_ms:.1f}ms")

@app.post("/api/proxy")
async def proxy(req: ProxyRequest, background_tasks: BackgroundTasks):
    start = time.time()
    result = await agent_client.request(req.method, req.path, req.body)
    duration = (time.time() - start) * 1000

    # 添加后台任务：响应返回后执行 / Add background task: runs after response
    background_tasks.add_task(log_request, req.method, req.path, duration)

    return result  # 立即返回响应，后台任务稍后执行 / Return immediately
```

**执行时序 / Execution Timeline**：

```text
客户端请求 / Client request
    │
    ▼
处理器执行 / Handler execution
    │
    ▼
响应发送给客户端 / Response sent to client  ← 客户端已收到响应
    │
    ▼
后台任务执行 / Background task executes  ← 不阻塞客户端
```

### 3.2 并发请求模式 / Concurrent Request Patterns

```python
import asyncio

@app.post("/api/batch")
async def batch(req: BatchRequest):
    """批量测试：并发转发多个请求。
    Batch test: forward multiple requests concurrently."""

    async def forward_one(item):
        start = time.time()
        try:
            data = await agent_client.request(item.method, item.path, item.body)
            return {"status": 200, "data": data, "duration_ms": (time.time()-start)*1000}
        except Exception as e:
            return {"status": 500, "error": str(e), "duration_ms": (time.time()-start)*1000}

    # asyncio.gather 并发执行所有请求 / Execute all requests concurrently
    results = await asyncio.gather(*[forward_one(item) for item in req.requests])

    return {
        "total": len(results),
        "succeeded": sum(1 for r in results if r["status"] == 200),
        "failed": sum(1 for r in results if r["status"] != 200),
        "results": results,
    }
```

## 4. 测试模式详解 / Testing Patterns Details

### 4.1 TestClient 同步测试 / TestClient Synchronous Testing

```python
import pytest
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, patch

from app.main import app

# 同步 TestClient 测试异步路由 / Sync TestClient tests async routes
client = TestClient(app)

def test_health_check():
    """健康检查端点测试 / Health check endpoint test"""
    response = client.get("/api/health")
    assert response.status_code == 200
    data = response.json()
    assert "backend" in data
    assert data["backend"] == "ok"

def test_proxy_validation_error():
    """请求体校验失败返回 422 / Validation error returns 422"""
    response = client.post("/api/proxy", json={})
    assert response.status_code == 422  # 缺少必填字段 / Missing required fields
```

### 4.2 异步测试与 Mock / Async Testing & Mock

```python
import pytest
from httpx import AsyncClient, ASGITransport

@pytest.fixture
def mock_agent():
    """模拟 agent 客户端，无需真实后端。
    Mock agent client, no real backend needed."""
    with patch("app.main.agent_client") as mock:
        mock.request = AsyncMock(return_value={"masked": "138****1234"})
        yield mock

@pytest.mark.asyncio
async def test_proxy_forward(mock_agent):
    """代理转发测试（模拟 agent 响应）/ Proxy forwarding test"""
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        response = await ac.post("/api/proxy", json={
            "method": "POST",
            "path": "/v1/privacy/mask",
            "body": {"data": {"phone": "13812341234"}},
        })
    assert response.status_code == 200
    mock_agent.request.assert_called_once()
```

### 4.3 测试策略对比 / Testing Strategy Comparison

| 方式 / Approach | 工具 / Tool | 适用 / Suitable | 速度 / Speed |
|---|---|---|---|
| TestClient (sync) | fastapi.testclient | 简单端点测试 / Simple endpoint | 快 / Fast |
| AsyncClient + ASGI | httpx | 异步路由测试 / Async routes | 快 / Fast |
| MockTransport | httpx.MockTransport | 隔离外部服务 / Isolate external | 极快 / Very fast |
| 真实服务器 / Real server | uvicorn + httpx | 集成测试 / Integration | 慢 / Slow |

## 5. 响应模型与序列化 / Response Model & Serialization

### 5.1 响应模型继承 / Response Model Inheritance

```python
from pydantic import BaseModel, Field
from typing import Any

# 基础响应 / Base response
class BaseResponse(BaseModel):
    """所有 API 响应的公共字段 / Common fields for all API responses"""
    status: int = 200
    duration_ms: float = Field(description="请求耗时 / Request duration")

# 代理响应（继承 + 扩展）/ Proxy response (inherit + extend)
class ProxyResponse(BaseResponse):
    """代理转发响应 / Proxy forwarding response"""
    data: Any = None
    via: str = "python-rest"       # 经由哪个后端 / Via which backend
    protocol: str = "REST"         # 使用的协议 / Protocol used

# 批量响应 / Batch response
class BatchResponse(BaseResponse):
    """批量测试响应 / Batch test response"""
    total: int
    succeeded: int
    failed: int
    results: list[dict]
```

### 5.2 条件响应格式 / Conditional Response Format

```python
from fastapi.responses import JSONResponse, Response

@app.post("/api/proxy")
async def proxy(req: ProxyRequest):
    result = await agent_client.request(...)

    # 根据内容类型返回不同格式 / Return different format by content type
    if isinstance(result, dict) and result.get("_content_type") == "application/vnd.apache.arrow.stream":
        # Arrow 解析结果：返回 JSON 记录 / Arrow parsed: return JSON records
        return JSONResponse(content={
            "status": 200,
            "data": result["records"],
            "schema": result["schema"],
        })
    else:
        # 标准 JSON 响应 / Standard JSON response
        return {"status": 200, "data": result}
```

## 6. 错误处理与弹性 / Error Handling & Resilience

### 6.1 分层错误处理 / Layered Error Handling

```text
┌─────────────────────────────────────────────────────────────┐
│  第 1 层：Pydantic 校验 / Layer 1: Pydantic validation     │
│  → 422 Unprocessable Entity (自动) / Automatic             │
├─────────────────────────────────────────────────────────────┤
│  第 2 层：业务逻辑错误 / Layer 2: Business logic errors    │
│  → HTTPException(400/404/409) (手动抛出) / Manual raise    │
├─────────────────────────────────────────────────────────────┤
│  第 3 层：上游服务错误 / Layer 3: Upstream service errors  │
│  → 502 Bad Gateway (httpx 异常捕获) / httpx exception      │
├─────────────────────────────────────────────────────────────┤
│  第 4 层：未预期异常 / Layer 4: Unexpected exceptions      │
│  → 500 Internal Server Error (全局异常处理器) / Global     │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 全局异常处理器 / Global Exception Handler

```python
from fastapi import Request
from fastapi.responses import JSONResponse
import traceback

@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    """全局异常捕获：防止未处理异常导致服务崩溃。
    Global exception catch: prevent unhandled exceptions from crashing service."""
    # 生产环境不返回堆栈跟踪 / Don't return stack trace in production
    detail = str(exc) if len(str(exc)) < 500 else "Internal server error"
    return JSONResponse(
        status_code=500,
        content={"detail": detail, "status": 500},
    )

@app.exception_handler(httpx.RequestError)
async def httpx_error_handler(request: Request, exc: httpx.RequestError):
    """上游连接错误：返回 502 Bad Gateway。
    Upstream connection error: return 502 Bad Gateway."""
    return JSONResponse(
        status_code=502,
        content={"detail": f"Unable to reach agent: {exc}", "status": 502},
    )
```

## 7. FastAPI 与 Starlette 的关系 / FastAPI & Starlette Relationship

### 7.1 架构层次 / Architecture Layers

```text
┌─────────────────────────────────────────────────────────────┐
│  FastAPI (高层 API) / High-level API                         │
│  - 路由装饰器 / Route decorators                            │
│  - Pydantic 集成 / Pydantic integration                     │
│  - 自动文档 / Auto documentation                            │
│  - 依赖注入 / Dependency injection                          │
└──────────────────────────┬──────────────────────────────────┘
                           │ 基于 / Built on
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Starlette (中层框架) / Mid-level framework                  │
│  - 路由匹配 / Route matching                                │
│  - 中间件 / Middleware                                      │
│  - 请求/响应对象 / Request/Response objects                 │
│  - WebSocket 支持 / WebSocket support                       │
│  - 静态文件 / Static files                                  │
└──────────────────────────┬──────────────────────────────────┘
                           │ 基于 / Built on
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Uvicorn (ASGI 服务器) / ASGI Server                         │
│  - HTTP 协议解析 / HTTP protocol parsing                    │
│  - 事件循环 / Event loop                                    │
│  - 并发连接管理 / Concurrent connection management          │
└─────────────────────────────────────────────────────────────┘
```

### 7.2 本项目使用的 Starlette 组件 / Starlette Components Used

| 组件 / Component | 用途 / Purpose | 本项目使用 / Usage |
|---|---|---|
| `CORSMiddleware` | 跨域资源共享 / Cross-origin resource sharing | ✅ 允许 Vite dev server |
| `BaseHTTPMiddleware` | 自定义中间件基类 / Custom middleware base | ✅ 安全中间件 |
| `StaticFiles` | 静态文件服务 / Static file serving | ✅ 托管 dist/assets |
| `FileResponse` | 文件响应 / File response | ✅ SPA 回退 index.html |
| `JSONResponse` | JSON 响应 / JSON response | ✅ 错误响应 |
| `TestClient` | 测试客户端 / Test client | ✅ 单元测试 |

## 8. 依赖注入系统详解 / Dependency Injection System Details

### 8.1 DI 架构原理 / DI Architecture Principles

FastAPI 的依赖注入是基于函数参数的声明式系统：
FastAPI's DI is a declarative system based on function parameters:

```python
from fastapi import Depends, Request

# 定义依赖 / Define dependency
async def get_agent_client(request: Request) -> AgentClient:
    """From request state, get the shared agent client.
    从请求状态中获取共享的 agent 客户端。"""
    return request.app.state.agent_client

# 使用依赖 / Use dependency
@app.post("/api/proxy")
async def proxy(
    body: ProxyRequest,
    client: AgentClient = Depends(get_agent_client),  # 自动注入 / Auto-injected
):
    return await client.forward(body)
```

### 8.2 依赖嵌套与组合 / Dependency Nesting & Composition

```python
# 依赖可以嵌套（依赖的依赖）/ Dependencies can nest (deps of deps)
async def get_config() -> Config:
    return Config()

async def get_client(config: Config = Depends(get_config)) -> AgentClient:
    return AgentClient(base_url=config.agent_url)

async def get_auth_client(
    client: AgentClient = Depends(get_client),
    api_key: str = Depends(get_api_key),
) -> AuthenticatedClient:
    return AuthenticatedClient(client, api_key)
```

**解析顺序 / Resolution order**：
```text
get_config() → get_client(config) → get_auth_client(client, api_key)
    ↑ 先解析 / Resolved first          ↓ 最后解析 / Resolved last
```

### 8.3 本项目 DI 使用场景 / DI Usage in This Project

| 场景 / Scenario | 依赖 / Dependency | 作用 / Purpose |
|---|---|---|
| 代理转发 / Proxy forwarding | `get_agent_client` | 获取共享 httpx 客户端 / Get shared httpx client |
| 配置读取 / Config reading | `get_settings` | 单例配置对象 / Singleton config object |
| 请求计时 / Request timing | 中间件 / Middleware | 计算 duration_ms / Calculate duration_ms |

## 9. 中间件机制 / Middleware Mechanism

### 9.1 中间件执行模型 / Middleware Execution Model

```text
请求方向 / Request direction →

┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐
│ CORS      │ → │ Security  │ → │ Timing    │ → │ Route     │
│ Middleware│   │ Middleware│   │ Middleware│   │ Handler   │
└───────────┘   └───────────┘   └───────────┘   └───────────┘
     │                │                │                │
     ▼                ▼                ▼                ▼
响应方向 / Response direction ←
```

### 9.2 本项目中间件实现 / Middleware Implementation

```python
# CORS 中间件（允许 Vite dev server 跨域）
# CORS middleware (allow Vite dev server cross-origin)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],  # Vite dev server
    allow_methods=["*"],
    allow_headers=["*"],
)

# 自定义计时中间件 / Custom timing middleware
@app.middleware("http")
async def add_process_time(request: Request, call_next):
    start = time.perf_counter()
    response = await call_next(request)
    duration_ms = (time.perf_counter() - start) * 1000
    response.headers["X-Process-Time"] = f"{duration_ms:.2f}"
    return response
```

### 9.3 中间件 vs 依赖注入 / Middleware vs DI

| 维度 / Dimension | 中间件 / Middleware | 依赖注入 / DI |
|---|---|---|
| 作用范围 / Scope | 所有请求 / All requests | 特定路由 / Specific routes |
| 访问请求体 / Access body | 不便（流式）/ Inconvenient (stream) | 直接获取 / Direct access |
| 执行时机 / Timing | 路由匹配前 / Before route match | 路由匹配后 / After route match |
| 典型用途 / Typical use | CORS、日志、计时 / CORS, logging, timing | 认证、数据库、客户端 / Auth, DB, client |

## 10. OpenAPI 文档自动生成 / OpenAPI Auto-documentation

### 10.1 文档生成机制 / Documentation Generation

FastAPI 自动从路由定义生成 OpenAPI 3.0 规范：
FastAPI auto-generates OpenAPI 3.0 spec from route definitions:

```python
# 自动生成文档的信息来源 / Sources for auto-generated docs
@app.post(
    "/api/proxy",
    summary="代理转发请求 / Proxy forward request",    # 摘要 / Summary
    description="将前端请求转发到 PrivShield Agent / Forward to agent",  # 描述 / Description
    response_model=ProxyResponse,                   # 响应模型 / Response model
    tags=["proxy"],                                 # 分组标签 / Group tag
)
async def proxy(body: ProxyRequest):  # Pydantic 模型自动生成请求体文档 / Auto request body docs
    ...
```

### 10.2 文档访问端点 / Documentation Endpoints

| 端点 / Endpoint | UI | 用途 / Purpose |
|---|---|---|
| `/docs` | Swagger UI | 交互式 API 测试 / Interactive API testing |
| `/redoc` | ReDoc | 可读性更好的文档 / More readable docs |
| `/openapi.json` | 无 / None | 原始 OpenAPI JSON / Raw OpenAPI JSON |

### 10.3 本项目文档配置 / This Project's Doc Configuration

```python
app = FastAPI(
    title="Privacy Test Console",        # 文档标题 / Doc title
    version="1.0.0",                     # API 版本 / API version
    description="隐私代理测试控制台后端 / Privacy proxy test console backend",
    docs_url="/docs",                    # Swagger UI 路径 / Swagger UI path
    redoc_url="/redoc",                  # ReDoc 路径 / ReDoc path
)
```

**文档与 Pydantic 的协作 / Docs & Pydantic collaboration**：
- 请求体结构从 `ProxyRequest` 模型自动推断 / Request body inferred from `ProxyRequest`
- 响应结构从 `response_model=ProxyResponse` 自动生成 / Response from `response_model`
- 字段约束（`Field(description=...)`）直接映射为文档描述 / Field constraints map to doc descriptions

## 11. 路由系统与 API 组织 / Routing System & API Organization

### 11.1 APIRouter 模块化路由 / Modular Routing with APIRouter

FastAPI 通过 `APIRouter` 实现路由的模块化组织，将不同功能域的路由分离到独立文件中：

```python
# app/routers/proxy.py — 代理路由模块 / Proxy router module
from fastapi import APIRouter, Depends

router = APIRouter(
    prefix="/api",              # 路由前缀 / Route prefix
    tags=["proxy"],             # OpenAPI 分组标签 / OpenAPI group tag
    dependencies=[Depends(verify_api_key)],  # 路由级依赖 / Router-level deps
)

@router.post("/proxy")
async def proxy_request(req: ProxyRequest):
    """转发请求到隐私代理 / Forward request to PrivShield Agent"""
    ...

@router.get("/endpoints")
async def list_endpoints():
    """列出可用端点 / List available endpoints"""
    ...
```

```python
# app/main.py — 路由注册 / Router registration
from fastapi import FastAPI
from app.routers import proxy, health, fixtures

app = FastAPI()
app.include_router(proxy.router)       # 代理路由 / Proxy routes
app.include_router(health.router)      # 健康检查 / Health check
app.include_router(fixtures.router)    # 测试数据 / Test fixtures
```

### 11.2 路由匹配机制 / Route Matching Mechanism

```text
请求到达 / Request arrives
    │
    ▼
┌─────────────────────────────────────────────────┐
│  路由匹配顺序 / Route matching order            │
│  1. 按注册顺序逐一匹配 / Match in registration │
│  2. 路径参数贪婪匹配 / Path params greedy match  │
│  3. 第一个匹配即停止 / First match wins          │
│  4. 无匹配返回 404 / No match → 404             │
└─────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────┐
│  路径参数类型约束 / Path param type constraints  │
│  /items/{item_id}      → str (默认 / default)    │
│  /items/{item_id:int}  → int (显式约束)          │
│  /files/{path:path}    → 含斜杠路径 / slash path │
└─────────────────────────────────────────────────┘
```

### 11.3 本项目的路由设计 / This Project's Route Design

| 路由 / Route | 方法 / Method | 功能 / Purpose | 认证 / Auth |
|---|---|---|---|
| `/api/proxy` | POST | 转发到 Agent REST / Forward to Agent | 可选 / Optional |
| `/api/endpoints` | GET | 获取端点列表 / Get endpoint list | 无 / None |
| `/api/samples/{name}` | GET | 获取示例数据 / Get sample data | 无 / None |
| `/health` | GET | 健康检查 / Health check | 无 / None |
| `/{path:path}` | GET | SPA 回退 / SPA fallback | 无 / None |

**设计决策 / Design decisions**：
- 所有 API 路由使用 `/api` 前缀，与 SPA 静态资源分离 / All API routes use `/api` prefix, separated from SPA
- 通配符路由 `/{path:path}` 放在最后注册，确保不拦截 API / Wildcard route registered last
- 健康检查独立于 `/api` 前缀，便于 K8s 探针访问 / Health check outside `/api` for K8s probes

## 12. 请求验证管道 / Request Validation Pipeline

### 12.1 验证流程全景 / Validation Pipeline Overview

```text
HTTP 请求 / HTTP Request
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 1: 路由匹配 / Stage 1: Route matching             │
│  - 路径 + 方法匹配 / Path + method match                │
│  - 提取路径参数 / Extract path params                    │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 2: 依赖解析 / Stage 2: Dependency resolution      │
│  - 执行 Depends() 链 / Execute Depends() chain          │
│  - 认证、限流等前置检查 / Auth, rate-limit pre-checks    │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 3: 请求体解析 / Stage 3: Body parsing             │
│  - Content-Type 检测 / Content-Type detection           │
│  - JSON 反序列化 / JSON deserialization                 │
│  - Pydantic 模型验证 / Pydantic model validation        │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 4: 参数注入 / Stage 4: Parameter injection        │
│  - 验证后的数据注入处理函数 / Validated data → handler   │
│  - Query/Path/Header 参数绑定 / Param binding           │
└─────────────────────────────────────────────────────────┘
    │
    ▼
  处理函数执行 / Handler execution
```

### 12.2 Pydantic 验证层 / Pydantic Validation Layer

```python
from pydantic import BaseModel, Field, field_validator
from typing import Optional
from enum import Enum

class HTTPMethod(str, Enum):
    """HTTP 方法枚举 / HTTP method enum"""
    GET = "GET"
    POST = "POST"
    PUT = "PUT"
    DELETE = "DELETE"

class ProxyRequest(BaseModel):
    """代理请求模型 — 验证管道核心 / Proxy request — validation core"""
    method: HTTPMethod = Field(
        default=HTTPMethod.POST,
        description="HTTP 方法 / HTTP method"
    )
    path: str = Field(
        ...,
        min_length=1,
        max_length=2048,
        pattern=r"^/",           # 必须以 / 开头 / Must start with /
        description="目标路径 / Target path"
    )
    body: Optional[dict] = Field(
        default=None,
        description="请求体 / Request body"
    )
    timeout: float = Field(
        default=30.0,
        ge=0.1,                  # 最小 0.1s / Min 0.1s
        le=300.0,                # 最大 300s / Max 300s
        description="超时秒数 / Timeout seconds"
    )

    @field_validator("path")
    @classmethod
    def validate_path(cls, v: str) -> str:
        """自定义路径验证 / Custom path validation"""
        if ".." in v:
            raise ValueError("路径不允许包含 .. / Path cannot contain ..")
        return v
```

### 12.3 验证错误响应格式 / Validation Error Response Format

```json
{
  "detail": [
    {
      "type": "string_pattern_mismatch",
      "loc": ["body", "path"],
      "msg": "String should match pattern '^/'",
      "input": "invalid-path"
    },
    {
      "type": "greater_than_equal",
      "loc": ["body", "timeout"],
      "msg": "Input should be >= 0.1",
      "input": -1
    }
  ]
}
```

**验证层次总结 / Validation layers summary**：

| 层次 / Layer | 机制 / Mechanism | 错误码 / Status | 可定制 / Customizable |
|---|---|---|---|
| 类型检查 / Type check | Pydantic 自动 / Auto | 422 | 否 / No |
| 约束验证 / Constraint | Field(ge, le, pattern) | 422 | 是 / Yes |
| 自定义验证器 / Custom validator | @field_validator | 422 | 是 / Yes |
| 业务逻辑 / Business logic | 处理函数内 / In handler | 400/409 | 是 / Yes |

## 13. 生命周期管理与资源清理 / Lifecycle Management & Resource Cleanup

### 13.1 Lifespan 上下文管理器 / Lifespan Context Manager

FastAPI 推荐使用 `lifespan` 替代已弃用的 `on_event` 来管理应用生命周期：

```python
from contextlib import asynccontextmanager
import httpx

@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    应用生命周期管理 / Application lifecycle management
    - yield 之前：启动逻辑 / Before yield: startup logic
    - yield 之后：关闭逻辑 / After yield: shutdown logic
    """
    # === 启动阶段 / Startup phase ===
    app.state.http_client = httpx.AsyncClient(
        base_url="http://127.0.0.1:8079",
        timeout=httpx.Timeout(30.0),
        limits=httpx.Limits(max_connections=100),
    )
    print("✅ HTTP 客户端已初始化 / HTTP client initialized")

    yield  # 应用运行中 / Application running

    # === 关闭阶段 / Shutdown phase ===
    await app.state.http_client.aclose()
    print("🛑 HTTP 客户端已关闭 / HTTP client closed")

app = FastAPI(lifespan=lifespan)
```

### 13.2 生命周期时序图 / Lifecycle Sequence Diagram

```text
uvicorn 启动 / uvicorn starts
    │
    ▼
┌───────────────────────────────────────────────┐
│  lifespan.__aenter__()                        │
│  - 创建 HTTP 客户端 / Create HTTP client      │
│  - 连接数据库 / Connect database              │
│  - 加载配置 / Load configuration              │
│  - 预热缓存 / Warm up cache                   │
└───────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────┐
│  应用就绪，开始接收请求 / App ready, serving   │
│  - 处理 HTTP 请求 / Handle HTTP requests      │
│  - 执行后台任务 / Run background tasks        │
│  ... (持续运行 / running) ...                  │
└───────────────────────────────────────────────┘
    │  ← SIGTERM / SIGINT
    ▼
┌───────────────────────────────────────────────┐
│  lifespan.__aexit__()                         │
│  - 停止接收新请求 / Stop accepting new reqs    │
│  - 等待进行中请求 / Wait in-flight requests    │
│  - 关闭 HTTP 客户端 / Close HTTP client       │
│  - 释放资源 / Release resources               │
└───────────────────────────────────────────────┘
    │
    ▼
进程退出 / Process exits
```

### 13.3 资源管理模式对比 / Resource Management Pattern Comparison

| 模式 / Pattern | 适用场景 / Use Case | 优点 / Pros | 缺点 / Cons |
|---|---|---|---|
| lifespan + app.state | 全局共享资源 / Global shared | 集中管理 / Centralized | 需类型提示 / Needs typing |
| Depends + yield | 请求级资源 / Per-request | 自动清理 / Auto cleanup | 每请求创建 / Per-req create |
| 全局单例 / Global singleton | 无状态客户端 / Stateless client | 简单 / Simple | 测试困难 / Hard to test |
| 后台任务 / BackgroundTasks | 非关键操作 / Non-critical | 不阻塞响应 / Non-blocking | 无保证执行 / No guarantee |

### 13.4 本项目的资源管理 / This Project's Resource Management

```python
# 本项目采用 lifespan + app.state 模式 / This project uses lifespan + app.state

@asynccontextmanager
async def lifespan(app: FastAPI):
    # 1. 创建到 Agent 的 HTTP 客户端 / Create HTTP client to Agent
    app.state.client = httpx.AsyncClient(
        base_url=settings.agent_url,     # 从配置读取 / From config
        timeout=settings.timeout,        # 统一超时 / Unified timeout
        trust_env=False,                 # 忽略系统代理 / Ignore system proxy
    )
    # 2. 加载示例数据 / Load sample fixtures
    app.state.samples = load_samples()

    yield

    # 3. 优雅关闭 / Graceful shutdown
    await app.state.client.aclose()

# 在处理函数中通过 request.app.state 访问 / Access via request.app.state in handlers
@app.post("/api/proxy")
async def proxy(req: ProxyRequest, request: Request):
    client: httpx.AsyncClient = request.app.state.client
    response = await client.request(...)
    return response.json()
```

**关键设计原则 / Key design principles**：
- 所有 I/O 资源在 lifespan 中创建和销毁 / All I/O resources created/destroyed in lifespan
- 使用 `app.state` 而非全局变量，便于测试替换 / Use `app.state` over globals for testability
- 客户端配置从环境变量/配置文件注入 / Client config injected from env/config
- 关闭时先停新请求，再等待进行中请求完成 / Stop new reqs first, then drain in-flight

---

## 14. WebSocket 与实时通信 / WebSocket & Real-time Communication

### 14.1 WebSocket 基础与 FastAPI 集成 / WebSocket Basics & FastAPI Integration

FastAPI 原生支持 WebSocket，基于 Starlette 的 WebSocket 实现：

```python
# ===== WebSocket 端点实现 / WebSocket Endpoint Implementation =====
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from typing import Dict, Set
import asyncio
import json

app = FastAPI()

# 连接管理器 / Connection Manager
class ConnectionManager:
    """WebSocket 连接池管理器 / WebSocket connection pool manager"""

    def __init__(self):
        # 活跃连接: {client_id: websocket}
        self.active: Dict[str, WebSocket] = {}
        # 订阅组: {channel: {client_ids}}
        self.channels: Dict[str, Set[str]] = {}

    async def connect(self, client_id: str, ws: WebSocket):
        await ws.accept()
        self.active[client_id] = ws

    def disconnect(self, client_id: str):
        self.active.pop(client_id, None)
        # 从所有频道移除 / Remove from all channels
        for subscribers in self.channels.values():
            subscribers.discard(client_id)

    def subscribe(self, client_id: str, channel: str):
        if channel not in self.channels:
            self.channels[channel] = set()
        self.channels[channel].add(client_id)

    async def broadcast(self, channel: str, message: dict):
        """向频道内所有订阅者广播 / Broadcast to all channel subscribers"""
        subscribers = self.channels.get(channel, set())
        payload = json.dumps(message)
        # 并发发送 / Concurrent send
        tasks = [
            self.active[cid].send_text(payload)
            for cid in subscribers
            if cid in self.active
        ]
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

manager = ConnectionManager()

@app.websocket("/ws/{client_id}")
async def websocket_endpoint(ws: WebSocket, client_id: str):
    await manager.connect(client_id, ws)
    try:
        while True:
            data = await ws.receive_text()
            msg = json.loads(data)

            if msg.get("action") == "subscribe":
                manager.subscribe(client_id, msg["channel"])
                await ws.send_text(json.dumps({
                    "type": "subscribed",
                    "channel": msg["channel"]
                }))
            elif msg.get("action") == "ping":
                await ws.send_text(json.dumps({"type": "pong"}))

    except WebSocketDisconnect:
        manager.disconnect(client_id)
```

### 14.2 实时日志流 / Real-time Log Streaming

```python
# ===== 实时日志推送（本项目场景）/ Real-time Log Push (Project Scenario) =====
from fastapi import WebSocket
from collections import deque
import asyncio

# 日志缓冲 / Log buffer
log_buffer: deque = deque(maxlen=1000)
log_subscribers: Set[asyncio.Queue] = set()

async def publish_log(entry: dict):
    """发布日志到所有订阅者 / Publish log to all subscribers"""
    log_buffer.append(entry)
    for queue in list(log_subscribers):
        try:
            queue.put_nowait(entry)
        except asyncio.QueueFull:
            pass  # 慢消费者丢弃 / Drop for slow consumers

@app.websocket("/ws/logs")
async def log_stream(ws: WebSocket):
    await ws.accept()
    queue: asyncio.Queue = asyncio.Queue(maxsize=100)
    log_subscribers.add(queue)

    try:
        # 先发送历史日志 / Send history first
        for entry in list(log_buffer)[-50:]:
            await ws.send_json(entry)

        # 实时推送新日志 / Push new logs in real-time
        while True:
            entry = await asyncio.wait_for(queue.get(), timeout=30)
            await ws.send_json(entry)

    except asyncio.TimeoutError:
        # 发送心跳保持连接 / Send heartbeat to keep alive
        await ws.send_json({"type": "heartbeat"})
    except WebSocketDisconnect:
        pass
    finally:
        log_subscribers.discard(queue)
```

### 14.3 WebSocket 与 HTTP 对比 / WebSocket vs HTTP Comparison

| 特性 / Feature | HTTP REST | WebSocket | 本项目选择 / Project Choice |
|---|---|---|---|
| 连接模式 / Connection | 短连接 / Short-lived | 长连接 / Persistent | HTTP 为主 / HTTP primary |
| 通信方向 / Direction | 请求-响应 / Req-Resp | 双向 / Bidirectional | 请求-响应足够 / Req-Resp enough |
| 实时性 / Real-time | 轮询 / Polling | 即时推送 / Instant push | 无需实时 / No real-time need |
| 复杂度 / Complexity | 低 / Low | 高 / High | 保持简单 / Keep simple |
| 适用场景 / Use case | CRUD 操作 / CRUD ops | 实时通知 / Real-time notify | 测试控制台 / Test console |

---

## 15. 安全与认证机制 / Security & Authentication

### 15.1 API Key 认证实现 / API Key Authentication Implementation

```python
# ===== API Key 认证中间件 / API Key Auth Middleware =====
from fastapi import FastAPI, Request, HTTPException, Security
from fastapi.security import APIKeyHeader
from starlette.middleware.base import BaseHTTPMiddleware
import hashlib
import hmac
import time

# 方案 1: 依赖注入式认证 / Approach 1: DI-based auth
api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)

# 存储哈希后的 key / Store hashed keys
VALID_KEY_HASHES = {
    hashlib.sha256(b"secret-key-1").hexdigest(): "admin",
    hashlib.sha256(b"secret-key-2").hexdigest(): "readonly",
}

async def verify_api_key(
    api_key: str = Security(api_key_header)
) -> str:
    """FastAPI 依赖: 验证 API Key / FastAPI dependency: verify API Key"""
    if not api_key:
        raise HTTPException(status_code=401, detail="Missing API key")

    key_hash = hashlib.sha256(api_key.encode()).hexdigest()
    role = VALID_KEY_HASHES.get(key_hash)

    if not role:
        raise HTTPException(status_code=403, detail="Invalid API key")

    return role

# 在路由中使用 / Use in routes
@app.post("/api/mask")
async def mask_data(request: MaskRequest, role: str = Depends(verify_api_key)):
    if role != "admin":
        raise HTTPException(status_code=403, detail="Admin access required")
    return await process_mask(request)


# 方案 2: 中间件式认证 / Approach 2: Middleware-based auth
class APIKeyMiddleware(BaseHTTPMiddleware):
    """全局 API Key 验证中间件 / Global API key validation middleware"""

    EXEMPT_PATHS = {"/health", "/docs", "/openapi.json", "/metrics"}

    async def dispatch(self, request: Request, call_next):
        # 豁免路径跳过验证 / Skip validation for exempt paths
        if request.url.path in self.EXEMPT_PATHS:
            return await call_next(request)

        api_key = request.headers.get("X-API-Key")
        if not api_key:
            return JSONResponse(status_code=401, content={"detail": "Missing API key"})

        key_hash = hashlib.sha256(api_key.encode()).hexdigest()
        if key_hash not in VALID_KEY_HASHES:
            return JSONResponse(status_code=403, content={"detail": "Invalid API key"})

        # 将角色注入 request.state / Inject role into request.state
        request.state.role = VALID_KEY_HASHES[key_hash]
        return await call_next(request)
```

### 15.2 HMAC 请求签名 / HMAC Request Signing

```python
# ===== HMAC 签名验证 / HMAC Signature Verification =====
import hashlib
import hmac
import time

SIGNING_SECRET = os.environ.get("SIGNING_SECRET", "").encode()
MAX_TIMESTAMP_DRIFT = 300  # 5分钟容差 / 5-minute tolerance

async def verify_hmac_signature(request: Request):
    """验证请求签名 / Verify request signature"""
    signature = request.headers.get("X-Signature")
    timestamp = request.headers.get("X-Timestamp")

    if not signature or not timestamp:
        raise HTTPException(401, "Missing signature headers")

    # 防重放: 检查时间戳 / Anti-replay: check timestamp
    try:
        req_time = int(timestamp)
        if abs(time.time() - req_time) > MAX_TIMESTAMP_DRIFT:
            raise HTTPException(401, "Request timestamp expired")
    except ValueError:
        raise HTTPException(401, "Invalid timestamp")

    # 计算签名 / Compute signature
    body = await request.body()
    message = f"{timestamp}.{body.decode()}".encode()
    expected = hmac.new(SIGNING_SECRET, message, hashlib.sha256).hexdigest()

    # 常量时间比较防侧信道 / Constant-time comparison against timing attacks
    if not hmac.compare_digest(signature, expected):
        raise HTTPException(403, "Invalid signature")
```

### 15.3 安全最佳实践 / Security Best Practices

| 实践 / Practice | 实现 / Implementation | 重要性 / Importance |
|---|---|---|
| 传输加密 / Transport encryption | TLS/HTTPS 强制 / Enforce TLS | ★★★ 必须 / Required |
| API Key 认证 / API Key auth | Header 传递 + 哈希存储 / Header + hashed | ★★★ 必须 / Required |
| 速率限制 / Rate limiting | 滑动窗口 / Sliding window | ★★☆ 推荐 / Recommended |
| CORS 配置 / CORS config | 白名单域名 / Whitelist origins | ★★★ 必须 / Required |
| 输入验证 / Input validation | Pydantic 模型 / Pydantic models | ★★★ 必须 / Required |
| 请求签名 / Request signing | HMAC-SHA256 | ★☆☆ 可选 / Optional |
| 审计日志 / Audit log | 记录所有操作 / Log all ops | ★★☆ 推荐 / Recommended |

---

## 16. 部署与性能调优 / Deployment & Performance Tuning

### 16.1 生产部署架构 / Production Deployment Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│           FastAPI 生产部署架构 / Production Deployment           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Internet                                                       │
│     │                                                           │
│     ▼                                                           │
│  ┌──────────────┐                                              │
│  │ Nginx/Traefik │  ← TLS 终结 + 负载均衡 / TLS + LB        │
│  └──────┬───────┘                                              │
│         │                                                       │
│    ┌────┴────┐                                                  │
│    ▼         ▼                                                  │
│  ┌─────┐  ┌─────┐                                             │
│  │Uvicorn│  │Uvicorn│  ← 多 Worker 进程 / Multi-worker        │
│  │ :8001 │  │ :8002 │                                             │
│  └──┬──┘  └──┬──┘                                             │
│     │         │                                                 │
│     └────┬────┘                                                 │
│          ▼                                                      │
│  ┌──────────────┐                                              │
│  │  FastAPI App  │  ← 应用实例 / App instance                  │
│  │  + Middleware │                                              │
│  └──────┬───────┘                                              │
│         │                                                       │
│    ┌────┴────┐                                                  │
│    ▼         ▼                                                  │
│  ┌─────┐  ┌───────┐                                           │
│  │Redis │  │SQLite/PG│  ← 状态存储 / State store              │
│  └─────┘  └───────┘                                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 16.2 Uvicorn 生产配置 / Uvicorn Production Configuration

```python
# ===== 生产环境 Uvicorn 配置 / Production Uvicorn Config =====
# 文件: run_production.py
import uvicorn
import multiprocessing

# Worker 数量: CPU核心数 * 2 + 1 / Workers: CPU cores * 2 + 1
workers = multiprocessing.cpu_count() * 2 + 1

uvicorn.run(
    "app.main:app",
    host="0.0.0.0",
    port=8000,
    workers=workers,           # 多进程 / Multi-process
    loop="uvloop",             # 高性能事件循环 / High-perf event loop
    http="httptools",          # 高性能 HTTP 解析 / High-perf HTTP parser
    log_level="info",
    access_log=True,
    timeout_keep_alive=30,     # Keep-alive 超时 / Keep-alive timeout
    limit_concurrency=1000,    # 最大并发 / Max concurrency
    limit_max_requests=10000,  # 单 worker 最大请求后重启 / Restart after N reqs
)
```

```bash
# ===== Docker 部署命令 / Docker Deployment Commands =====

# 开发环境 / Development
docker run -p 8079:8079 \
  -e PRIVACY_LOG_LEVEL=debug \
  PrivShield:0.1.0

# 生产环境 / Production
docker run -d \
  --name privacy-agent \
  -p 8079:8079 -p 50051:50051 \
  -e PRIVACY_LOG_FORMAT=json \
  -e PRIVACY_AUTH_ENABLED=true \
  -e PRIVACY_RATE_LIMIT_ENABLED=true \
  -e PRIVACY_TLS_ENABLED=true \
  -v /certs:/app/certs:ro \
  --memory=512m \
  --cpus=2 \
  --restart=unless-stopped \
  PrivShield:0.1.0
```

### 16.3 性能调优策略 / Performance Tuning Strategies

| 优化项 / Optimization | 方法 / Method | 效果 / Effect | 复杂度 / Complexity |
|---|---|---|---|
| 事件循环 / Event loop | 使用 uvloop | +30-50% 吞吐 / throughput | 低 / Low |
| HTTP 解析 / HTTP parser | 使用 httptools | +10-20% 解析速度 / parse speed | 低 / Low |
| 连接复用 / Connection reuse | httpx.AsyncClient 单例 | 减少 TCP 握手 / Reduce handshake | 低 / Low |
| 响应压缩 / Response compression | GZipMiddleware | 减少传输体积 / Reduce payload | 低 / Low |
| 缓存 / Caching | Redis / in-memory LRU | 减少重复计算 / Reduce recompute | 中 / Medium |
| Worker 数 / Worker count | CPU * 2 + 1 | 充分利用多核 / Utilize multi-core | 低 / Low |
| 异步 I/O / Async I/O | async/await 全链路 | 避免阻塞 / Avoid blocking | 中 / Medium |
| 连接池 / Connection pool | 数据库/HTTP 连接池 | 减少连接开销 / Reduce conn overhead | 中 / Medium |

### 16.4 本项目部署模式 / This Project's Deployment Modes

```bash
# ===== 本项目支持的部署方式 / Supported Deployment Modes =====

# 1. 本地开发 / Local development
python -m engine.server
# REST: http://127.0.0.1:8079 | gRPC: 127.0.0.1:50051

# 2. Docker 单容器 / Docker single container
docker build --target core -t PrivShield:0.1.0 .
docker run -p 8079:8079 -p 50051:50051 PrivShield:0.1.0

# 3. Docker Compose (含代理) / Docker Compose (with proxy)
cd deploy/docker-compose && docker-compose up -d

# 4. Kubernetes + Helm
helm install privshield ./deploy/helm/PrivShield \
  -f values-production.yaml

# 5. Sidecar 模式 / Sidecar mode
# 与业务 Pod 同部署，通过 localhost 通信
# Deployed with business Pod, communicate via localhost
```

## 17. 流式响应与大文件处理 / Streaming Response & Large File Handling

### 17.1 StreamingResponse 基础 / StreamingResponse Basics

```python
# 流式响应：逐块发送数据，不占用内存
# Streaming response: send data chunk by chunk, no memory hogging
from fastapi import FastAPI
from fastapi.responses import StreamingResponse
import asyncio
import json

app = FastAPI()


# 基本流式下载
# Basic streaming download
@app.get("/api/v1/export")
async def export_data():
    """ 流式导出大量分类结果 """
    """ Stream export large classification results """
    
    async def generate():
        # 逐批生成数据
        # Generate data batch by batch
        for batch in range(100):
            records = await fetch_batch(batch, size=1000)
            for record in records:
                yield json.dumps(record, ensure_ascii=False) + "\n"
            # 让出事件循环
            # Yield event loop
            await asyncio.sleep(0)
    
    return StreamingResponse(
        generate(),
        media_type="application/x-ndjson",
        headers={
            "Content-Disposition": "attachment; filename=export.ndjson",
            "X-Total-Batches": "100",
        },
    )


# SSE (Server-Sent Events) 流
# SSE (Server-Sent Events) stream
@app.get("/api/v1/classify/progress/{job_id}")
async def classify_progress(job_id: str):
    """ 实时分类进度推送 """
    """ Real-time classification progress push """
    
    async def event_stream():
        for stage in ["rule_engine", "ner", "llm", "composite"]:
            progress = await get_stage_progress(job_id, stage)
            yield f"event: progress\ndata: {{\"stage\": \"{stage}\", \"percent\": {progress}}}\n\n"
            await asyncio.sleep(1)
        
        result = await get_final_result(job_id)
        yield f"event: complete\ndata: {json.dumps(result)}\n\n"
    
    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",  # 禁用 Nginx 缓冲 / Disable Nginx buffering
        },
    )
```

### 17.2 文件上传与流式处理 / File Upload & Stream Processing

```python
# 大文件流式上传（不全量读入内存）
# Large file streaming upload (not fully loaded into memory)
from fastapi import UploadFile, File
import aiofiles
from pathlib import Path

UPLOAD_DIR = Path("/tmp/uploads")
MAX_FILE_SIZE = 100 * 1024 * 1024  # 100MB
CHUNK_SIZE = 64 * 1024  # 64KB


@app.post("/api/v1/upload")
async def upload_file(file: UploadFile = File(...)):
    """ 流式文件上传 """
    """ Streaming file upload """
    # 验证文件类型
    # Validate file type
    allowed_types = {"text/csv", "application/json", "application/x-ndjson"}
    if file.content_type not in allowed_types:
        raise HTTPException(415, f"Unsupported type: {file.content_type}")
    
    # 流式写入磁盘
    # Stream write to disk
    dest = UPLOAD_DIR / f"{uuid4().hex}_{file.filename}"
    total_size = 0
    
    async with aiofiles.open(dest, "wb") as f:
        while chunk := await file.read(CHUNK_SIZE):
            total_size += len(chunk)
            if total_size > MAX_FILE_SIZE:
                await f.close()
                dest.unlink(missing_ok=True)
                raise HTTPException(413, "File too large")
            await f.write(chunk)
    
    return {"filename": dest.name, "size": total_size}


# 流式文件下载
# Streaming file download
@app.get("/api/v1/download/{filename}")
async def download_file(filename: str):
    """ 安全文件下载（防路径穿越） """
    """ Secure file download (prevent path traversal) """
    # 安全检查：防止路径穿越
    # Security: prevent path traversal
    safe_name = Path(filename).name  # 去除路径分隔符 / Strip path separators
    file_path = UPLOAD_DIR / safe_name
    
    if not file_path.exists():
        raise HTTPException(404, "File not found")
    
    async def file_stream():
        async with aiofiles.open(file_path, "rb") as f:
            while chunk := await f.read(CHUNK_SIZE):
                yield chunk
    
    return StreamingResponse(
        file_stream(),
        media_type="application/octet-stream",
        headers={"Content-Disposition": f"attachment; filename={safe_name}"},
    )
```

### 17.3 流式响应性能对比 / Streaming Response Performance Comparison

| 方式 / Method | 内存占用 / Memory | 首字节时间 / TTFB | 适用场景 / Use Case |
|---|---|---|---|
| JSONResponse | 全量 / Full | 慢（等全部完成）/ Slow | 小响应 < 1MB |
| StreamingResponse | 恒定 / Constant | 快 / Fast | 大文件/无限流 / Large/infinite |
| FileResponse | 低 / Low | 快 / Fast | 静态文件 / Static files |
| SSE | 恒定 / Constant | 即时 / Instant | 实时推送 / Real-time push |

## 18. API 版本化与演进 / API Versioning & Evolution

### 18.1 版本化策略 / Versioning Strategies

```python
# 策略 1：URL 路径版本（本项目采用）
# Strategy 1: URL path version (used in this project)
app = FastAPI()

v1_router = APIRouter(prefix="/api/v1")
v2_router = APIRouter(prefix="/api/v2")

@v1_router.post("/mask")
async def mask_v1(data: MaskRequestV1):
    """ V1: 基础脱敏 """
    return await do_mask(data)

@v2_router.post("/mask")
async def mask_v2(data: MaskRequestV2):
    """ V2: 支持上下文感知脱敏 """
    """ V2: Supports context-aware masking """
    return await do_mask_v2(data)

app.include_router(v1_router)
app.include_router(v2_router)


# 策略 2：Header 版本
# Strategy 2: Header version
from fastapi import Header

@app.post("/api/mask")
async def mask_versioned(
    data: dict,
    accept_version: str = Header(default="1.0", alias="Accept-Version"),
):
    if accept_version == "2.0":
        return await do_mask_v2(data)
    return await do_mask_v1(data)
```

### 18.2 向后兼容与废弃 / Backward Compatibility & Deprecation

```python
# 废弃端点标记
# Deprecated endpoint marking
import warnings
from fastapi import Response

def deprecated(sunset_date: str, replacement: str):
    """ 废弃装饰器：添加 Sunset 头 """
    """ Deprecation decorator: adds Sunset header """
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, response: Response, **kwargs):
            response.headers["Deprecation"] = "true"
            response.headers["Sunset"] = sunset_date
            response.headers["Link"] = f'<{replacement}>; rel="successor-version"'
            return await func(*args, **kwargs)
        return wrapper
    return decorator


@v1_router.post("/classify")
@deprecated(sunset_date="2025-12-31", replacement="/api/v2/classify")
async def classify_v1(data: ClassifyRequestV1):
    """ V1 分类（已废弃，使用 V2） """
    """ V1 classify (deprecated, use V2) """
    return await classify(data)
```

### 18.3 API 变更日志管理 / API Changelog Management

```python
# 版本兼容性矩阵
# Version compatibility matrix
API_VERSIONS = {
    "v1": {
        "status": "deprecated",
        "sunset": "2025-12-31",
        "changes": "基础隐私原语 / Basic privacy primitives",
    },
    "v2": {
        "status": "stable",
        "changes": "添加上下文感知 + 异步任务 / Context-aware + async jobs",
    },
    "v3": {
        "status": "beta",
        "changes": "流式分类 + 多模态 / Streaming + multimodal",
    },
}

@app.get("/api/versions")
async def list_versions():
    """ 列出所有 API 版本及状态 """
    """ List all API versions and status """
    return API_VERSIONS
```

### 18.4 版本化最佳实践 / Versioning Best Practices

| 实践 / Practice | 说明 / Description | 本项目 / This Project |
|---|---|---|
| URL 路径版本 / URL path version | 最直观 / Most intuitive | ✅ /api/v1/ |
| 不删除旧版本 / Don't remove old | 给迁移时间 / Give migration time | ✅ 废弃标记 |
| 响应含版本号 / Response has version | 方便调试 / Easy debugging | ✅ X-API-Version |
| 文档同步更新 / Docs sync | OpenAPI 自动 / Auto OpenAPI | ✅ /docs |
| 破坏性变更必须新版本 / Breaking = new version | 严格语义 / Strict semantics | ✅ |

## 19. 可观测性与生产监控 / Observability & Production Monitoring

### 19.1 结构化日志集成 / Structured Logging Integration

```python
# 请求级别结构化日志
# Request-level structured logging
import structlog
from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware
import time
import uuid

logger = structlog.get_logger()


class StructuredLoggingMiddleware(BaseHTTPMiddleware):
    """ 结构化请求日志中间件 """
    """ Structured request logging middleware """
    
    async def dispatch(self, request: Request, call_next):
        request_id = str(uuid.uuid4())[:8]
        start = time.perf_counter()
        
        # 注入 request_id 到上下文
        # Inject request_id into context
        structlog.contextvars.bind_contextvars(
            request_id=request_id,
            method=request.method,
            path=request.url.path,
        )
        
        try:
            response = await call_next(request)
            duration_ms = (time.perf_counter() - start) * 1000
            
            logger.info(
                "request_completed",
                status=response.status_code,
                duration_ms=round(duration_ms, 2),
            )
            
            response.headers["X-Request-ID"] = request_id
            return response
        except Exception as exc:
            duration_ms = (time.perf_counter() - start) * 1000
            logger.error(
                "request_failed",
                error=str(exc),
                duration_ms=round(duration_ms, 2),
            )
            raise
        finally:
            structlog.contextvars.unbind_contextvars(
                "request_id", "method", "path"
            )
```

### 19.2 Prometheus 指标 / Prometheus Metrics

```python
# 自定义 Prometheus 指标
# Custom Prometheus metrics
from prometheus_client import Counter, Histogram, Gauge
from prometheus_fastapi_instrumentator import Instrumentator

# 业务指标
# Business metrics
MASK_REQUESTS = Counter(
    "privacy_mask_requests_total",
    "Total masking requests",
    ["strategy", "status"],
)

CLASSIFICATION_DURATION = Histogram(
    "privacy_classification_duration_seconds",
    "Classification processing time",
    ["layer"],
    buckets=[0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0],
)

BUDGET_REMAINING = Gauge(
    "privacy_budget_remaining",
    "Remaining privacy budget",
    ["namespace"],
)

# 自动插桩 FastAPI
# Auto-instrument FastAPI
instrumentator = Instrumentator(
    should_group_status_codes=True,
    should_ignore_untemplated=True,
    excluded_handlers=["/metrics", "/health"],
)


@app.on_event("startup")
async def setup_metrics():
    instrumentator.instrument(app).expose(
        app, endpoint="/metrics", include_in_schema=False
    )
```

### 19.3 分布式追踪 / Distributed Tracing

```python
# OpenTelemetry 集成
# OpenTelemetry integration
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

# 初始化 Tracer
# Initialize Tracer
provider = TracerProvider()
processor = BatchSpanProcessor(
    OTLPSpanExporter(endpoint="http://jaeger:4317")
)
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)

# 自动插桩
# Auto-instrument
FastAPIInstrumentor.instrument_app(app)

# 手动 Span
# Manual Span
tracer = trace.get_tracer("privacy-agent")

@app.post("/api/v1/classify")
async def classify(data: ClassifyRequest):
    with tracer.start_as_current_span("classify_pipeline") as span:
        span.set_attribute("field_count", len(data.fields))
        
        with tracer.start_as_current_span("layer1_rule_engine"):
            rule_results = await run_rule_engine(data)
        
        with tracer.start_as_current_span("layer2_ner"):
            ner_results = await run_ner(data)
        
        span.set_attribute("classification_count", len(rule_results))
        return merge_results(rule_results, ner_results)
```

### 19.4 健康检查与就绪探针 / Health Check & Readiness Probe

```python
# K8s 健康检查端点
# K8s health check endpoints
@app.get("/health", include_in_schema=False)
async def health():
    """ 存活探针：进程活着就返回 200 """
    """ Liveness probe: returns 200 if process alive """
    return {"status": "healthy"}


@app.get("/readyz", include_in_schema=False)
async def readiness():
    """ 就绪探针：检查依赖服务可用性 """
    """ Readiness probe: checks dependency availability """
    checks = {}
    
    # 检查预算数据库
    # Check budget database
    try:
        await budget_db.ping()
        checks["budget_db"] = "ok"
    except Exception as e:
        checks["budget_db"] = f"error: {e}"
    
    # 检查模型加载状态
    # Check model loading status
    checks["ner_model"] = "loaded" if ner_engine.is_ready else "loading"
    checks["llm_model"] = "loaded" if llm_engine.is_ready else "not_configured"
    
    all_ok = all(v in ("ok", "loaded", "not_configured") for v in checks.values())
    
    return JSONResponse(
        status_code=200 if all_ok else 503,
        content={"status": "ready" if all_ok else "not_ready", "checks": checks},
    )
```

### 19.5 可观测性架构总结 / Observability Architecture Summary

```
┌─────────────────────────────────────────────────────────────┐
│  可观测性三支柱 / Observability Three Pillars              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌────────────┐   ┌──────────────┐   ┌───────────────┐  │
│  │   Logs     │   │   Metrics    │   │    Traces     │  │
│  │ structlog  │   │ Prometheus   │   │ OpenTelemetry │  │
│  │ JSON 格式  │   │ /metrics     │   │ OTLP export   │  │
│  └─────┬──────┘   └──────┬───────┘   └───────┬───────┘  │
│        │              │               │              │
│        ▼              ▼               ▼              │
│  ┌─────────────────────────────────────────────────┐  │
│  │          Grafana / Jaeger / Loki              │  │
│  │          统一可视化 / Unified visualization      │  │
│  └─────────────────────────────────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

| 支柱 / Pillar | 工具 / Tool | 用途 / Purpose | 本项目实现 / Implementation |
|---|---|---|---|
| 日志 / Logs | structlog (JSON) | 事件记录 / Event recording | StructuredLoggingMiddleware |
| 指标 / Metrics | prometheus-client | 聚合监控 / Aggregate monitoring | /metrics + 业务指标 |
| 追踪 / Traces | OpenTelemetry | 请求链路 / Request path | FastAPIInstrumentor |
| 健康 / Health | 自定义 / Custom | K8s 探针 / K8s probes | /health + /readyz |
