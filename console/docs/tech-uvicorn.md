# Uvicorn 技术栈说明 / Uvicorn Technology Stack

## 1. 技术简介 / Introduction

Uvicorn 是一个基于 `uvloop` 和 `httptools` 的高性能 ASGI Web 服务器，是 Python 异步 Web 生态的事实标准运行器。
Uvicorn is a high-performance ASGI web server built on `uvloop` and `httptools`, the de facto standard runner for Python's async web ecosystem.

核心特性 / Core Features：
- **ASGI 协议支持（ASGI Protocol）**：完整实现 ASGI 3.0 规范，支持 HTTP/1.1、WebSocket、生命周期事件。
- **极致性能（Extreme Performance）**：底层使用 uvloop（libuv 的 Python 绑定）替代默认 asyncio 事件循环，吞吐量提升 2-4 倍。
- **热重载（Hot Reload）**：开发模式下文件变更自动重启服务，无需手动操作。
- **多 Worker 支持（Multi-worker）**：通过 `--workers` 参数启动多进程，利用多核 CPU。
- **优雅关闭（Graceful Shutdown）**：收到 SIGTERM 后等待进行中请求完成再退出。
- **零配置启动（Zero-config）**：一行命令即可运行任何 ASGI 应用。

本项目使用版本 / Version Used：`uvicorn[standard] >= 0.27.0`

`[standard]` 额外依赖包含：
- `uvloop`：高性能事件循环（替代 asyncio 默认实现）
- `httptools`：Node.js HTTP 解析器的 Python 绑定
- `websockets`：WebSocket 协议支持
- `watchfiles`：文件监控（热重载用）

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 架构角色 / Architecture Role

```
浏览器 ──HTTP──▶ Uvicorn (port 8080) ──▶ FastAPI App ──httpx──▶ privacy-local-agent (port 8079)
                 ASGI 服务器               Python REST 代理后端
                 ASGI Server               Python REST Proxy Backend
```

Uvicorn 作为 Python REST 代理后端的 HTTP 服务器：
Uvicorn serves as the HTTP server for the Python REST proxy backend:
- 接收前端 HTTP 请求 / Receives frontend HTTP requests
- 将请求交给 FastAPI 路由处理 / Hands requests to FastAPI routing
- FastAPI 通过 httpx 转发到 agent / FastAPI forwards to agent via httpx
- 返回 JSON 响应给前端 / Returns JSON response to frontend

### 2.2 启动方式 / Startup Methods

#### 开发模式（启动脚本）/ Development Mode (Startup Script)

文件 / File：`console/scripts/dev-start.sh`

```bash
# 在虚拟环境中启动 Uvicorn
# Start Uvicorn in virtual environment
(
    source "$BACKEND_VENV/bin/activate"
    cd "$SCRIPT_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080
) &
```

参数说明 / Parameter Notes：
- `app.main:app`：模块路径 `app/main.py` 中的 `app` 对象（FastAPI 实例）
  Module path `app/main.py`'s `app` object (FastAPI instance)
- `--host 127.0.0.1`：仅监听本地回环地址（安全：不暴露到外网）
  Listen on loopback only (security: not exposed to external network)
- `--port 8080`：控制台后端端口 / Console backend port

#### 开发模式（手动 + 热重载）/ Development Mode (Manual + Hot Reload)

文件 / File：`console/backend/run.sh`

```bash
# 带热重载的开发服务器（文件变更自动重启）
# Dev server with hot reload (auto-restart on file changes)
uvicorn app.main:app --host 127.0.0.1 --port 8080 --reload
```

`--reload` 参数启用 watchfiles 监控 `app/` 目录，任何 `.py` 文件变更触发自动重启。
`--reload` enables watchfiles monitoring of `app/` directory, any `.py` change triggers auto-restart.

### 2.3 与 FastAPI 的集成 / Integration with FastAPI

文件 / File：`console/backend/app/main.py`

```python
from fastapi import FastAPI
from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Uvicorn 启动/关闭时触发的生命周期钩子。
    Lifespan hooks triggered by Uvicorn on startup/shutdown."""
    # 启动时：预热 httpx 连接池 / Startup: warm up httpx connection pool
    _ = await agent_client._get_client()
    yield
    # 关闭时：优雅释放连接 / Shutdown: gracefully release connections
    if agent_client._client is not None:
        await agent_client._client.aclose()

app = FastAPI(title="Privacy Test Console", lifespan=lifespan)
```

Uvicorn 在启动时发送 `lifespan.startup` 事件，触发 `yield` 前的代码；
Uvicorn sends `lifespan.startup` event on start, triggering code before `yield`;
收到 SIGTERM 时发送 `lifespan.shutdown`，触发 `yield` 后的清理代码。
on SIGTERM sends `lifespan.shutdown`, triggering cleanup code after `yield`.

### 2.4 生产部署考量 / Production Deployment Considerations

| 场景 / Scenario | 推荐配置 / Recommended Config |
|---|---|
| 本地开发 / Local dev | `uvicorn app.main:app --reload --port 8080` |
| 单实例生产 / Single instance prod | `uvicorn app.main:app --host 0.0.0.0 --port 8080 --workers 4` |
| 容器化 / Containerized | 由 K8s/Docker 管理进程，单 worker 即可 / Process managed by K8s/Docker, single worker suffices |
| 反向代理后 / Behind reverse proxy | Nginx/Caddy 处理 TLS，Uvicorn 仅监听 127.0.0.1 / Nginx/Caddy handles TLS, Uvicorn listens on 127.0.0.1 only |

### 2.5 为何选择 Uvicorn / Why Uvicorn

| 对比维度 / Dimension | Uvicorn | Gunicorn | 传统 WSGI (uWSGI) |
|---|---|---|---|
| 异步支持 / Async support | 原生 ASGI / Native ASGI | 需 worker 插件 / Needs worker plugin | 不支持 / Not supported |
| 性能 / Performance | 极高（uvloop）/ Very high | 中等 / Medium | 中等 / Medium |
| 配置复杂度 / Config complexity | 极低 / Very low | 中等 / Medium | 高 / High |
| FastAPI 官方推荐 / FastAPI official | ✅ 首选 / First choice | 可选 / Optional | 不兼容 / Incompatible |
| WebSocket | 内置 / Built-in | 需额外配置 / Extra config | 不支持 / Not supported |

### 2.6 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 使用 `[standard]` 安装 | 获得 uvloop + httptools 性能加成 / Get uvloop + httptools performance boost |
| 绑定 127.0.0.1 | 控制台仅本地使用，无需暴露 / Console is local-only, no need to expose |
| 单 worker 模式 | 代理转发为 I/O 密集，单进程异步即可饱和 / Proxy is I/O bound, single async process suffices |
| lifespan 管理连接池 | 利用 Uvicorn 生命周期事件优雅管理资源 / Leverage Uvicorn lifecycle events for graceful resource management |

### 2.7 ASGI 协议详解 / ASGI Protocol Details

ASGI（Asynchronous Server Gateway Interface）是 Python 异步 Web 应用的标准接口：

```text
┌─────────────────────────────────────────────────────────────┐
│  HTTP 请求到达 / HTTP Request arrives                        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Uvicorn 解析 HTTP / Uvicorn parses HTTP                    │
│  - httptools 解析请求行/头 / Parse request line/headers      │
│  - 构建 ASGI scope 字典 / Build ASGI scope dict             │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  调用 ASGI App / Invoke ASGI App                            │
│  await app(scope, receive, send)                            │
│  - scope: 请求元数据（method, path, headers）/ Request meta   │
│  - receive: 接收请求体 / Receive request body               │
│  - send: 发送响应 / Send response                           │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  FastAPI 路由处理 / FastAPI routing                          │
│  - 路由匹配 / Route matching                                │
│  - 依赖注入 / Dependency injection                          │
│  - 处理器执行 / Handler execution                           │
│  - 响应序列化 / Response serialization                      │
└─────────────────────────────────────────────────────────────┘
```

**ASGI vs WSGI 对比 / ASGI vs WSGI comparison**：

| 特性 / Feature | ASGI | WSGI |
|---|---|---|
| 并发模型 / Concurrency | 异步 (async/await) | 同步（线程/进程）|
| WebSocket | ✅ 原生支持 / Native | ❌ 不支持 / Not supported |
| HTTP/2 | ✅ 可扩展 / Extensible | ❌ 不支持 / Not supported |
| 生命周期事件 / Lifespan | ✅ startup/shutdown | ❌ 无 / None |
| 代表框架 / Frameworks | FastAPI, Starlette | Django (<3.0), Flask |

### 2.8 事件循环机制 / Event Loop Mechanism

```text
┌─────────────────────────────────────────────────────────────┐
│  asyncio 默认事件循环 / Default asyncio event loop           │
│  - Python 实现 / Python implementation                      │
│  - 基于 selectors 模块 / Based on selectors module          │
│  - 性能基准 / Baseline performance                          │
└─────────────────────────────────────────────────────────────┘
                           │
                           │  uvloop 替换 / Replace with uvloop
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  uvloop 事件循环 / uvloop event loop                        │
│  - libuv 的 Python 绑定（C 实现）/ libuv Python binding (C) │
│  - 比默认 asyncio 快 2-4x / 2-4x faster than default asyncio│
│  - 与 Node.js 相同的事件循环底层 / Same underlying as Node.js│
│  - 自动启用（安装 uvloop 后）/ Auto-enabled after install    │
└─────────────────────────────────────────────────────────────┘
```

**性能对比（典型 JSON API）/ Performance comparison (typical JSON API)**：

| 配置 / Config | 请求/秒 / Req/s | 延迟 P99 / Latency P99 |
|---|---|---|
| asyncio + h11 | ~15,000 | ~8ms |
| uvloop + httptools | ~45,000 | ~3ms |
| 提升 / Improvement | **3x** | **2.7x** |

### 2.9 信号处理与优雅关闭 / Signal Handling & Graceful Shutdown

```text
┌─────────────────────────────────────────────────────────────┐
│  收到 SIGTERM/SIGINT / Receive SIGTERM/SIGINT               │
│  (Ctrl+C 或 K8s Pod 终止) / (Ctrl+C or K8s Pod termination)│
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  停止接受新连接 / Stop accepting new connections             │
│  - 关闭监听 socket / Close listening socket                 │
│  - 新请求被拒绝 / New requests rejected                     │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  等待进行中请求完成 / Wait for in-flight requests            │
│  - 默认超时 30 秒 / Default timeout 30s                     │
│  - 可通过 --timeout-graceful-shutdown 配置 / Configurable    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  触发 lifespan.shutdown / Trigger lifespan.shutdown         │
│  - 执行 yield 后的清理代码 / Execute cleanup after yield     │
│  - 关闭 httpx 连接池 / Close httpx connection pool          │
│  - 释放资源 / Release resources                             │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  进程退出 / Process exits                                   │
│  - 退出码 0（正常）/ Exit code 0 (normal)                   │
└─────────────────────────────────────────────────────────────┘
```

### 2.10 Worker 模型 / Worker Model

```bash
# 单 Worker（默认，本项目使用）/ Single Worker (default, used by this project)
uvicorn app.main:app --host 127.0.0.1 --port 8080

# 多 Worker（生产环境）/ Multi Worker (production)
uvicorn app.main:app --host 0.0.0.0 --port 8080 --workers 4
```

**Worker 模型对比 / Worker model comparison**：

| 模式 / Mode | 进程数 / Processes | 适用场景 / Use case |
|---|---|---|
| 单 Worker | 1 | I/O 密集（代理、API 网关）/ I/O bound (proxy, API gateway) |
| 多 Worker | N | CPU 密集（图像处理、ML 推理）/ CPU bound (image processing, ML) |
| 容器化 | 1（K8s 管理副本）/ 1 (K8s manages replicas) | 云原生部署 / Cloud-native deployment |

**为何本项目使用单 Worker / Why single Worker for this project**：

```text
请求处理流程 / Request processing flow:
前端请求 → FastAPI 路由 → httpx 异步转发 → 等待 agent 响应 → 返回
                              │                    │
                              │                    └─ I/O 等待（非阻塞）/ I/O wait (non-blocking)
                              └─ 事件循环可处理其他请求 / Event loop handles other requests

单进程异步即可处理数百并发连接，无需多进程。
Single async process handles hundreds of concurrent connections, no multi-process needed.
```

### 2.11 性能调优参数 / Performance Tuning Parameters

| 参数 / Parameter | 默认值 / Default | 说明 / Description |
|---|---|---|
| `--workers` | 1 | Worker 进程数（CPU 核心数）/ Worker processes (CPU cores) |
| `--limit-concurrency` | 无限制 / Unlimited | 最大并发连接数 / Max concurrent connections |
| `--limit-max-requests` | 无限制 / Unlimited | 单 Worker 处理 N 请求后重启（防内存泄漏）/ Restart after N requests |
| `--timeout-keep-alive` | 5s | Keep-Alive 连接超时 / Keep-Alive connection timeout |
| `--timeout-graceful-shutdown` | 30s | 优雅关闭超时 / Graceful shutdown timeout |
| `--backlog` | 2048 | TCP 连接队列长度 / TCP connection queue length |
| `--h11-max-incomplete-event-size` | 16KB | 最大请求头大小 / Max request header size |

**本项目推荐配置 / Recommended config for this project**：

```bash
# 开发环境 / Development
uvicorn app.main:app --host 127.0.0.1 --port 8080 --reload

# 生产环境（容器内）/ Production (in container)
uvicorn app.main:app --host 0.0.0.0 --port 8080 \
  --limit-concurrency 1000 \
  --timeout-keep-alive 65 \
  --timeout-graceful-shutdown 30
```

## 3. 日志配置详解 / Logging Configuration Details

### 3.1 Uvicorn 内置日志体系 / Uvicorn Built-in Logging System

Uvicorn 使用 Python 标准 `logging` 模块，定义了两个独立的 Logger：

```text
┌─────────────────────────────────────────────────────────────┐
│  Uvicorn 日志架构 / Uvicorn Logging Architecture              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  "uvicorn" (根 Logger)                                      │
│    ├── "uvicorn.error"   → 服务器生命周期日志               │
│    │     - 启动/关闭信息 / Startup/shutdown info            │
│    │     - 连接建立/断开 / Connection established/closed    │
│    │     - 异常堆栈 / Exception tracebacks                  │
│    │                                                       │
│    └── "uvicorn.access"  → HTTP 访问日志                    │
│          - 每个请求一行 / One line per request              │
│          - 格式: IP:PORT - "METHOD PATH HTTP/1.1" STATUS    │
│          - 类似 Nginx access_log 格式                       │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 日志级别与格式配置 / Log Level & Format Configuration

```python
# 编程式配置日志（适用于生产环境）
# Programmatic log configuration (for production)
import uvicorn

log_config = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "default": {
            # 带颜色的开发格式 / Colored development format
            "format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s",
            "datefmt": "%Y-%m-%d %H:%M:%S",
        },
        "json": {
            # 结构化 JSON 格式（生产环境推荐）/ Structured JSON (production recommended)
            "()": "uvicorn.logging.JSONFormatter",
            "format": "%(asctime)s %(levelname)s %(message)s",
        },
    },
    "handlers": {
        "default": {
            "formatter": "default",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
    },
    "loggers": {
        "uvicorn": {"handlers": ["default"], "level": "INFO"},
        "uvicorn.error": {"level": "INFO"},
        "uvicorn.access": {
            "handlers": ["default"],
            "level": "INFO",
            "propagate": False,  # 不传播到父 Logger / Don't propagate to parent
        },
    },
}

uvicorn.run("app.main:app", log_config=log_config)
```

**命令行日志参数 / CLI Logging Parameters**：

| 参数 / Parameter | 默认值 / Default | 说明 / Description |
|---|---|---|
| `--log-level` | `info` | 全局日志级别 (critical/error/warning/info/debug/trace) |
| `--log-config` | None | 自定义日志配置文件路径（JSON/YAML）/ Custom log config file |
| `--access-log` / `--no-access-log` | 启用 / Enabled | 是否输出 HTTP 访问日志 / Whether to output access log |
| `--use-colors` / `--no-use-colors` | 自动检测 / Auto | 终端彩色输出 / Terminal colored output |

### 3.3 访问日志格式解析 / Access Log Format Analysis

```text
典型访问日志输出 / Typical access log output:

INFO:     127.0.0.1:52314 - "GET /api/health HTTP/1.1" 200 OK
INFO:     127.0.0.1:52314 - "POST /api/proxy HTTP/1.1" 200 OK
INFO:     127.0.0.1:52316 - "GET /api/samples HTTP/1.1" 200 OK

各字段含义 / Field meanings:
┌──────────┬─────────────────┬─────────────────────────────┬──────────┐
│ 级别     │ 客户端地址:端口  │ "方法 路径 协议版本"         │ 状态码   │
│ Level    │ Client:Port     │ "Method Path Protocol"      │ Status   │
└──────────┴─────────────────┴─────────────────────────────┴──────────┘
```

## 4. 编程式启动 API / Programmatic Startup API

### 4.1 Python 代码中启动 Uvicorn / Starting Uvicorn from Python Code

```python
import uvicorn

# 方式 1：简单启动（阻塞当前线程）/ Simple start (blocks current thread)
uvicorn.run(
    "app.main:app",           # 应用导入路径 / Application import path
    host="127.0.0.1",         # 监听地址 / Listen address
    port=8080,                # 监听端口 / Listen port
    reload=True,              # 开发模式热重载 / Dev mode hot reload
    log_level="debug",        # 日志级别 / Log level
)

# 方式 2：传入 ASGI 应用对象（无需字符串导入）/ Pass ASGI app object directly
from app.main import app
uvicorn.run(app, host="0.0.0.0", port=8080)

# 方式 3：多 Worker 模式（内部使用 multiprocessing）
# Multi-worker mode (internally uses multiprocessing)
uvicorn.run(
    "app.main:app",
    host="0.0.0.0",
    port=8080,
    workers=4,                # 启动 4 个子进程 / Spawn 4 child processes
    # 注意：workers > 1 时不能传 app 对象，必须用字符串路径
    # Note: workers > 1 requires string path, not app object
)
```

### 4.2 Config + Server 细粒度控制 / Fine-grained Control with Config + Server

```python
import asyncio
import uvicorn

# 高级用法：分离配置与服务器实例，支持异步启停
# Advanced: separate config from server instance, supports async start/stop
config = uvicorn.Config(
    "app.main:app",
    host="127.0.0.1",
    port=8080,
    log_level="info",
    limit_concurrency=1000,      # 最大并发连接 / Max concurrent connections
    timeout_keep_alive=65,       # Keep-Alive 超时 / Keep-Alive timeout
    timeout_graceful_shutdown=30, # 优雅关闭超时 / Graceful shutdown timeout
)
server = uvicorn.Server(config)

# 在异步上下文中运行（可与 gRPC 服务器共存）
# Run in async context (can coexist with gRPC server)
async def main():
    # 本项目 server.py 即使用此模式同时启动 REST + gRPC
    # This project's server.py uses this pattern to start REST + gRPC together
    await asyncio.gather(
        server.serve(),           # Uvicorn REST 服务 / Uvicorn REST service
        grpc_server.start(),      # gRPC 服务 / gRPC service
    )

asyncio.run(main())
```

### 4.3 本项目中的编程式启动 / Programmatic Startup in This Project

文件 / File：`privacy_local_agent/server.py`

```text
┌─────────────────────────────────────────────────────────────┐
│  server.py 启动流程 / server.py Startup Flow                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. 创建 FastAPI app 实例 / Create FastAPI app instance      │
│  2. 创建 gRPC Server 实例 / Create gRPC Server instance      │
│  3. 配置 uvicorn.Config / Configure uvicorn.Config           │
│  4. asyncio.gather(                                         │
│       uvicorn_server.serve(),    ← REST 服务                │
│       grpc_server.start(),       ← gRPC 服务                │
│     )                                                       │
│  5. 两个服务共享同一事件循环 / Both share same event loop     │
│                                                             │
│  优势：单进程同时提供 REST(8079) + gRPC(50051)              │
│  Advantage: single process serves REST(8079) + gRPC(50051)  │
└─────────────────────────────────────────────────────────────┘
```

## 5. 热重载机制详解 / Hot Reload Mechanism Details

### 5.1 热重载工作原理 / Hot Reload Working Principle

```text
┌─────────────────────────────────────────────────────────────┐
│  --reload 模式进程模型 / --reload Mode Process Model           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  主进程（Reloader）/ Main Process (Reloader)                  │
│    │                                                        │
│    ├── watchfiles 监控线程 / watchfiles monitoring thread    │
│    │     - 使用 Rust 编写的 notify 库 / Uses Rust notify lib│
│    │     - macOS: FSEvents / Linux: inotify / Win: ReadDir  │
│    │     - 监控 *.py 文件变更 / Monitors *.py changes       │
│    │                                                        │
│    └── 子进程（Worker）/ Child Process (Worker)              │
│          - 运行实际 ASGI 应用 / Runs actual ASGI app         │
│          - 文件变更时被杀死并重启 / Killed & restarted on    │
│            file change                                      │
│                                                             │
│  触发重启的事件 / Events Triggering Restart:                  │
│    - .py 文件保存 / .py file saved                          │
│    - 新增/删除 .py 文件 / .py file added/removed            │
│    - 默认忽略: .git, __pycache__, .venv, node_modules       │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 热重载配置选项 / Hot Reload Configuration Options

```bash
# 基础热重载 / Basic hot reload
uvicorn app.main:app --reload

# 指定监控目录（默认当前目录）/ Specify watch directories
uvicorn app.main:app --reload --reload-dir ./app --reload-dir ./lib

# 指定文件扩展名（默认 .py）/ Specify file extensions
uvicorn app.main:app --reload --reload-include "*.py" --reload-include "*.yaml"

# 排除特定文件 / Exclude specific files
uvicorn app.main:app --reload --reload-exclude "*_test.py" --reload-exclude "*.log"

# 重启延迟（防止频繁重启）/ Restart delay (prevent rapid restarts)
uvicorn app.main:app --reload --reload-delay 0.5  # 500ms 防抖
```

| 参数 / Parameter | 默认值 / Default | 说明 / Description |
|---|---|---|
| `--reload` | 关闭 / Off | 启用热重载 / Enable hot reload |
| `--reload-dir` | 当前目录 / CWD | 监控目录（可多次指定）/ Watch dirs (repeatable) |
| `--reload-include` | `*.py` | 触发重载的文件模式 / File patterns triggering reload |
| `--reload-exclude` | 无 / None | 排除的文件模式 / Excluded file patterns |
| `--reload-delay` | 0.25s | 变更后的等待延迟 / Delay after change before restart |

### 5.3 热重载 vs 手动重启对比 / Hot Reload vs Manual Restart

| 维度 / Dimension | --reload 热重载 | 手动重启 / Manual restart |
|---|---|---|
| 重启速度 / Restart speed | ~200-500ms | ~2-5s（含终端操作）/ Including terminal ops |
| 状态保持 / State preservation | 丢失（进程重建）/ Lost (process rebuilt) | 丢失 / Lost |
| 连接池 / Connection pool | 重建 / Rebuilt | 重建 / Rebuilt |
| 适用环境 / Environment | 仅开发 / Dev only | 任何 / Any |
| 生产禁用原因 / Why disabled in prod | 文件监控消耗 inotify 资源 / File watch consumes inotify |

## 6. ASGI 中间件管线 / ASGI Middleware Pipeline

### 6.1 中间件执行模型 / Middleware Execution Model

ASGI 中间件采用"洋葱模型"（Onion Model），请求从外向内穿过每层中间件：

```text
┌─────────────────────────────────────────────────────────────┐
│  请求方向 / Request Direction                                 │
│                                                             │
│  客户端请求 → [CORS] → [TrustedHost] → [自定义] → FastAPI   │
│              │         │              │           │         │
│              │  外层    │   中层       │  内层     │  应用   │
│              │         │              │           │         │
│  客户端响应 ← [CORS] ← [TrustedHost] ← [自定义] ← FastAPI   │
│                                                             │
│  每层中间件可以 / Each middleware layer can:                   │
│    - 修改请求头 / Modify request headers                     │
│    - 短路返回（不传给内层）/ Short-circuit (skip inner)       │
│    - 修改响应体 / Modify response body                       │
│    - 记录耗时 / Record timing                                │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 本项目中间件栈 / Project Middleware Stack

文件 / File：`console/backend/app/main.py`

```python
from fastapi.middleware.cors import CORSMiddleware

# CORS 中间件（最外层，处理跨域预检请求）
# CORS middleware (outermost, handles cross-origin preflight)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],  # Vite 开发服务器 / Vite dev server
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
```

**请求完整生命周期 / Complete Request Lifecycle**：

```text
浏览器发起请求 / Browser initiates request
    │
    ▼
Uvicorn 接收 TCP 连接 / Uvicorn accepts TCP connection
    │
    ▼
httptools 解析 HTTP 报文 / httptools parses HTTP message
    │
    ▼
构建 ASGI scope 字典 / Build ASGI scope dict
    │
    ▼
CORSMiddleware 检查 Origin / CORSMiddleware checks Origin
    │  ├── 预检请求 OPTIONS → 直接返回 200 / Preflight → return 200
    │  └── 正常请求 → 传递给内层 / Normal → pass to inner
    ▼
FastAPI 路由匹配 / FastAPI route matching
    │
    ▼
依赖注入（Depends）/ Dependency injection
    │
    ▼
路由处理器执行 / Route handler execution
    │  └── httpx 异步转发到 agent / Async forward to agent
    ▼
响应序列化（JSON）/ Response serialization
    │
    ▼
CORSMiddleware 添加 CORS 头 / Add CORS headers
    │
    ▼
Uvicorn 发送 HTTP 响应 / Uvicorn sends HTTP response
```

## 7. Gunicorn + Uvicorn 生产部署 / Gunicorn + Uvicorn Production Deployment

### 7.1 为何需要 Gunicorn / Why Gunicorn is Needed

```text
┌─────────────────────────────────────────────────────────────┐
│  Uvicorn 单进程局限 / Uvicorn Single-process Limitations      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Uvicorn --workers N:                                       │
│    - 使用 Python multiprocessing 简单 fork                  │
│    - 无进程监控/自动重启 / No process monitoring/auto-restart│
│    - 无预 fork 模型 / No pre-fork model                     │
│    - Worker 崩溃后不会自动补充 / Crashed workers not replaced│
│                                                             │
│  Gunicorn + UvicornWorker:                                  │
│    - 成熟的进程管理器 / Mature process manager               │
│    - Worker 崩溃自动重启 / Auto-restart crashed workers      │
│    - 优雅重启（零停机）/ Graceful restart (zero downtime)    │
│    - 预 fork 模型（启动即就绪）/ Pre-fork (ready on start)   │
│    - 丰富的信号处理 / Rich signal handling                   │
└─────────────────────────────────────────────────────────────┘
```

### 7.2 Gunicorn 启动命令 / Gunicorn Startup Command

```bash
# 生产环境推荐配置 / Production recommended config
gunicorn app.main:app \
  --worker-class uvicorn.workers.UvicornWorker \
  --workers 4 \
  --bind 0.0.0.0:8080 \
  --timeout 120 \
  --graceful-timeout 30 \
  --keep-alive 65 \
  --max-requests 10000 \
  --max-requests-jitter 1000 \
  --access-logfile - \
  --error-logfile -
```

**参数详解 / Parameter Details**：

| 参数 / Parameter | 说明 / Description |
|---|---|
| `--worker-class uvicorn.workers.UvicornWorker` | 使用 Uvicorn 作为 Worker 实现（ASGI 兼容）|
| `--workers 4` | Worker 进程数（通常 = CPU 核心数 × 2 + 1）|
| `--timeout 120` | Worker 无响应超时（秒）/ Worker silent timeout |
| `--graceful-timeout 30` | 优雅关闭等待时间 / Graceful shutdown wait |
| `--keep-alive 65` | Keep-Alive 连接保持秒数（需 > 负载均衡器超时）|
| `--max-requests 10000` | 单 Worker 处理 N 请求后自动重启（防内存泄漏）|
| `--max-requests-jitter 1000` | 重启抖动（避免所有 Worker 同时重启）|

### 7.3 部署架构对比 / Deployment Architecture Comparison

| 方案 / Approach | 适用场景 / Use Case | 优势 / Pros | 劣势 / Cons |
|---|---|---|---|
| 纯 Uvicorn | 开发/容器化 / Dev/Container | 简单、零依赖 / Simple | 无进程管理 / No process mgmt |
| Gunicorn + Uvicorn | 传统服务器 / Bare metal | 成熟稳定 / Mature | 额外依赖 / Extra dependency |
| K8s + 纯 Uvicorn | 云原生 / Cloud-native | K8s 管理进程 / K8s manages | 需 K8s 基础设施 / Needs K8s infra |
| Docker Compose | 本地集成测试 / Local integration | 一键启动 / One-command | 非生产级 / Not production-grade |

## 8. 常见问题排查 / Common Troubleshooting

### 8.1 端口占用 / Port Already in Use

```bash
# 症状 / Symptom:
# ERROR: [Errno 48] error while attempting to bind on address ('127.0.0.1', 8080)

# 排查：查找占用端口的进程 / Find process using the port
lsof -i :8080
# 或 / Or
ss -tlnp | grep 8080

# 解决：终止旧进程或更换端口 / Kill old process or change port
kill -9 <PID>
uvicorn app.main:app --port 8081
```

### 8.2 模块导入失败 / Module Import Error

```bash
# 症状 / Symptom:
# Error loading ASGI app. Could not import module "app.main"

# 常见原因 / Common causes:
# 1. 工作目录不正确（不在 backend/ 下）/ Wrong CWD (not in backend/)
# 2. 虚拟环境未激活 / Virtual environment not activated
# 3. 依赖未安装 / Dependencies not installed

# 解决 / Solution:
cd console/backend
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --port 8080
```

### 8.3 Worker 超时 / Worker Timeout

```bash
# 症状（Gunicorn 模式）/ Symptom (Gunicorn mode):
# [CRITICAL] WORKER TIMEOUT (pid:12345)

# 原因：请求处理时间超过 --timeout / Request exceeds --timeout
# 解决：增大超时或优化处理逻辑 / Increase timeout or optimize handler
gunicorn app.main:app --timeout 300  # 增大到 5 分钟 / Increase to 5 min
```

### 8.4 内存泄漏检测 / Memory Leak Detection

```bash
# 使用 --limit-max-requests 定期重启 Worker 防止内存累积
# Use --limit-max-requests to periodically restart workers
uvicorn app.main:app --limit-max-requests 50000

# 监控内存使用 / Monitor memory usage
watch -n 5 'ps aux | grep uvicorn | grep -v grep'

# 或使用 Prometheus 指标（本项目 agent 已集成）
# Or use Prometheus metrics (this project's agent already integrates)
curl http://127.0.0.1:8079/metrics | grep process_resident_memory
```

### 8.5 排查清单 / Troubleshooting Checklist

| 问题 / Issue | 检查项 / Check | 解决方案 / Solution |
|---|---|---|
| 启动失败 / Startup failure | 端口占用、模块路径、依赖 | lsof、cd 正确目录、pip install |
| 请求超时 / Request timeout | agent 是否运行、网络连通 | curl agent 健康检查 / Curl agent health |
| 高延迟 / High latency | 事件循环阻塞（同步代码）| 使用 async/await 或 run_in_executor |
| 内存增长 / Memory growth | 连接池泄漏、大对象缓存 | --limit-max-requests 定期重启 |
| 热重载无效 / Reload not working | 文件不在监控目录 | --reload-dir 指定正确路径 |
| WebSocket 断开 / WS disconnect | keep-alive 超时过短 | 增大 --timeout-keep-alive |

## 9. 事件循环深入 / Event Loop Deep Dive

### 9.1 asyncio 事件循环架构 / asyncio Event Loop Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  Uvicorn 事件循环架构 / Uvicorn Event Loop Architecture         │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌──────────────────────────────────────────────────────┐  │
│  │           asyncio Event Loop (uvloop)              │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │                                                      │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐      │  │
│  │  │ HTTP     │  │ HTTP     │  │ HTTP     │      │  │
│  │  │ Conn 1   │  │ Conn 2   │  │ Conn N   │      │  │
│  │  └────┬─────┘  └────┬─────┘  └────┬─────┘      │  │
│  │       │           │           │              │  │
│  │       ▼           ▼           ▼              │  │
│  │  ┌────────────────────────────────────────┐  │  │
│  │  │        Task Scheduler                  │  │  │
│  │  │  - I/O 多路复用 (epoll/kqueue)         │  │  │
│  │  │  - 协程调度 / Coroutine scheduling     │  │  │
│  │  │  - 定时器 / Timers                     │  │  │
│  │  └────────────────────────────────────────┘  │  │
│  │       │           │           │              │  │
│  │       ▼           ▼           ▼              │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │  │
│  │  │ ASGI App │  │ ASGI App │  │ ASGI App │  │  │
│  │  │ (FastAPI)│  │ (FastAPI)│  │ (FastAPI)│  │  │
│  │  └──────────┘  └──────────┘  └──────────┘  │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                                │
│  单线程并发：所有请求共享一个事件循环
│  Single-threaded concurrency: all requests share one loop
└────────────────────────────────────────────────────────────────┘
```

### 9.2 同步与异步处理对比 / Sync vs Async Processing

```python
# ✅ 正确：异步处理（不阻塞事件循环）
# Correct: async handling (doesn't block event loop)
import httpx
from fastapi import FastAPI

app = FastAPI()

@app.post("/api/mask")
async def mask_data(request: MaskRequest):
    # 异步 HTTP 调用，不阻塞其他请求
    # Async HTTP call, doesn't block other requests
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            "http://127.0.0.1:8079/v1/mask",
            json=request.model_dump(),
            timeout=30.0,
        )
    return resp.json()


# ❌ 错误：同步阻塞（卡死整个事件循环）
# Wrong: sync blocking (freezes entire event loop)
import requests

@app.post("/api/mask-bad")
async def mask_data_bad(request: MaskRequest):
    # 这会阻塞事件循环，所有其他请求都会等待！
    # This blocks the event loop, all other requests wait!
    resp = requests.post(  # ← 同步调用 / Sync call
        "http://127.0.0.1:8079/v1/mask",
        json=request.model_dump(),
    )
    return resp.json()


# ✅ 补救：必须用同步库时，放入线程池
# Fix: when sync library required, use thread pool
import asyncio

@app.post("/api/mask-thread")
async def mask_data_thread(request: MaskRequest):
    loop = asyncio.get_event_loop()
    # 在线程池中执行同步代码 / Run sync code in thread pool
    result = await loop.run_in_executor(
        None,  # 使用默认线程池 / Use default pool
        sync_mask_function,
        request,
    )
    return result
```

### 9.3 事件循环监控 / Event Loop Monitoring

```python
import asyncio
import logging
import time

logger = logging.getLogger(__name__)

class EventLoopMonitor:
    """事件循环延迟监控器 / Event loop latency monitor.

    检测事件循环是否被阻塞 / Detects if event loop is blocked.
    """

    def __init__(self, threshold_ms: float = 100.0):
        self.threshold_ms = threshold_ms
        self._task: asyncio.Task | None = None

    async def start(self):
        self._task = asyncio.create_task(self._monitor())

    async def _monitor(self):
        while True:
            start = time.perf_counter()
            await asyncio.sleep(0.1)  # 每 100ms 检查一次 / Check every 100ms
            elapsed_ms = (time.perf_counter() - start) * 1000

            # 如果实际等待远超 100ms，说明循环被阻塞
            # If actual wait >> 100ms, loop was blocked
            lag = elapsed_ms - 100
            if lag > self.threshold_ms:
                logger.warning(
                    f"Event loop lag detected: {lag:.1f}ms "
                    f"(threshold: {self.threshold_ms}ms)"
                )

    async def stop(self):
        if self._task:
            self._task.cancel()


# 在 FastAPI 生命周期中启用 / Enable in FastAPI lifespan
from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app: FastAPI):
    monitor = EventLoopMonitor(threshold_ms=200)
    await monitor.start()
    yield
    await monitor.stop()
```

### 9.4 本项目事件循环实践 / This Project's Event Loop Practice

| 组件 / Component | 异步策略 / Async Strategy | 说明 / Notes |
|---|---|---|
| Console Backend (FastAPI) | httpx.AsyncClient | 异步转发到 Agent / Async proxy to Agent |
| Privacy Agent (FastAPI) | 混合 / Mixed | 计算密集型用线程池 / CPU-bound uses pool |
| gRPC Server | ThreadPoolExecutor | 同步 servicer + 线程池 / Sync servicer + pool |
| Gateway | asyncio + httpx | 全异步代理 / Fully async proxy |

## 10. 信号处理与优雅关闭 / Signal Handling & Graceful Shutdown

### 10.1 信号处理流程 / Signal Handling Flow

```text
┌────────────────────────────────────────────────────────────────┐
│  Uvicorn 优雅关闭流程 / Uvicorn Graceful Shutdown Flow          │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  SIGTERM / SIGINT 接收 / Received                               │
│       │                                                        │
│       ▼                                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ 1. 停止接受新连接 / Stop accepting new connections    │  │
│  │    - 关闭监听 socket / Close listening socket         │  │
│  │    - 新请求被拒绝 / New requests rejected             │  │
│  └──────────────────────────────────────────────────────┘  │
│       │                                                        │
│       ▼                                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ 2. 等待现有请求完成 / Wait for in-flight requests     │  │
│  │    - 超时: --timeout-graceful-shutdown (default: 30s) │  │
│  │    - 已接收的请求继续处理 / Received reqs continue    │  │
│  └──────────────────────────────────────────────────────┘  │
│       │                                                        │
│       ▼                                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ 3. 触发 ASGI lifespan shutdown / Trigger lifespan     │  │
│  │    - 关闭 httpx 连接池 / Close httpx pool            │  │
│  │    - 刷新日志 / Flush logs                           │  │
│  │    - 关闭 DB 连接 / Close DB connections             │  │
│  └──────────────────────────────────────────────────────┘  │
│       │                                                        │
│       ▼                                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ 4. 强制关闭剩余连接 / Force close remaining conns     │  │
│  │    - 超时未完成的请求被取消 / Timed-out reqs cancelled │  │
│  │    - 进程退出 / Process exits                        │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 10.2 FastAPI Lifespan 事件 / FastAPI Lifespan Events

```python
from contextlib import asynccontextmanager
from fastapi import FastAPI
import httpx
import logging

logger = logging.getLogger(__name__)

@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理 / Application lifespan management.

    启动时初始化资源，关闭时清理资源 /
    Initialize resources on startup, cleanup on shutdown.
    """
    # === 启动阶段 / Startup phase ===
    logger.info("Starting up: initializing resources...")

    # 初始化 HTTP 客户端连接池 / Initialize HTTP client pool
    app.state.http_client = httpx.AsyncClient(
        base_url="http://127.0.0.1:8079",
        timeout=30.0,
        limits=httpx.Limits(max_connections=20),
    )

    logger.info("Startup complete: resources ready")

    yield  # 应用运行中 / Application running

    # === 关闭阶段 / Shutdown phase ===
    logger.info("Shutting down: cleaning up resources...")

    # 关闭 HTTP 客户端 / Close HTTP client
    await app.state.http_client.aclose()

    logger.info("Shutdown complete: all resources released")


app = FastAPI(lifespan=lifespan)
```

### 10.3 K8s 优雅关闭配置 / K8s Graceful Shutdown Config

```yaml
# K8s Deployment 配置 / K8s Deployment config
spec:
  terminationGracePeriodSeconds: 60  # 总优雅期 / Total grace period
  containers:
  - name: console-backend
    lifecycle:
      preStop:
        exec:
          # 先等待负载均衡器摘流 / Wait for LB to drain
          command: ["sh", "-c", "sleep 5"]
    # Uvicorn 优雅关闭超时 / Uvicorn graceful timeout
    # 必须小于 terminationGracePeriodSeconds
    # Must be less than terminationGracePeriodSeconds
    command: [
      "uvicorn", "app.main:app",
      "--timeout-graceful-shutdown", "30",
    ]
```

### 10.4 本项目关闭策略 / This Project's Shutdown Strategy

| 组件 / Component | 关闭行为 / Shutdown Behavior | 超时 / Timeout |
|---|---|---|
| Console Backend | 关闭 httpx 连接池 / Close httpx pool | 30s |
| Privacy Agent REST | 等待当前请求 / Wait current reqs | 30s |
| Privacy Agent gRPC | server.stop(grace=5) | 5s grace |
| Gateway | 关闭所有后端连接 / Close all backend conns | 10s |

## 11. 多进程与 Worker 管理 / Multi-Process & Worker Management

### 11.1 进程模型对比 / Process Model Comparison

```text
┌────────────────────────────────────────────────────────────────┐
│  Uvicorn 进程模型 / Uvicorn Process Models                      │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  模式 1：单进程（本项目使用）/ Mode 1: Single process (used)    │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  uvicorn app.main:app                                   │  │
│  │  └── 1 个事件循环 / 1 event loop                       │  │
│  │      └── N 个并发协程 / N concurrent coroutines         │  │
│  └────────────────────────────────────────────────────────┘  │
│  适用：开发、低并发代理 / Use: dev, low-concurrency proxy      │
│                                                                │
│  模式 2：多 Worker / Mode 2: Multi-worker                       │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  gunicorn -w 4 -k uvicorn.workers.UvicornWorker         │  │
│  │  ├── Worker 1 (PID 101): event loop + coroutines       │  │
│  │  ├── Worker 2 (PID 102): event loop + coroutines       │  │
│  │  ├── Worker 3 (PID 103): event loop + coroutines       │  │
│  │  └── Worker 4 (PID 104): event loop + coroutines       │  │
│  └────────────────────────────────────────────────────────┘  │
│  适用：生产、多核利用 / Use: production, multi-core            │
│                                                                │
│  模式 3：Uvicorn 内置多 Worker / Mode 3: Uvicorn built-in       │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  uvicorn app.main:app --workers 4                       │  │
│  │  ├── Supervisor process (管理) / Management             │  │
│  │  ├── Worker 1: event loop                              │  │
│  │  ├── Worker 2: event loop                              │  │
│  │  ├── Worker 3: event loop                              │  │
│  │  └── Worker 4: event loop                              │  │
│  └────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 11.2 Worker 数量决策 / Worker Count Decision

| 场景 / Scenario | 推荐 Workers | 原因 / Reason |
|---|---|---|
| 本地开发 / Local dev | 1 | 简化调试、热重载 / Simple debug, hot reload |
| 代理转发（I/O 密集）/ Proxy (I/O) | 1-2 | 异步已足够并发 / Async suffices |
| CPU 密集计算 / CPU-bound | CPU 核数 / CPU cores | 利用多核 / Utilize multi-core |
| 生产部署 / Production | 2×CPU + 1 | Gunicorn 经典公式 / Classic formula |
| 本项目 Console Backend | 1 | 纯代理，异步即可 / Pure proxy, async enough |

### 11.3 Gunicorn + Uvicorn 生产配置 / Production Config

```python
# gunicorn.conf.py - 生产环境配置 / Production config
import multiprocessing

# Worker 配置 / Worker config
worker_class = "uvicorn.workers.UvicornWorker"
workers = multiprocessing.cpu_count() * 2 + 1
worker_connections = 1000

# 超时配置 / Timeout config
timeout = 120               # 请求超时 / Request timeout
graceful_timeout = 30       # 优雅关闭超时 / Graceful shutdown
keepalive = 5               # Keep-alive 秒数 / Keep-alive seconds

# 稳定性 / Stability
max_requests = 50000        # 每个 Worker 处理 N 请求后重启 / Restart after N reqs
max_requests_jitter = 5000  # 随机偏移避免同时重启 / Random jitter

# 日志 / Logging
accesslog = "-"             # stdout
errorlog = "-"              # stderr
loglevel = "info"

# 进程管理 / Process management
preload_app = True          # 预加载应用（减少内存）/ Preload (reduce memory)
proc_name = "console-backend"
```

### 11.4 本项目进程管理实践 / This Project's Process Management

| 组件 / Component | 进程模型 / Process Model | 说明 / Notes |
|---|---|---|
| Console Backend | 单进程 Uvicorn | 开发工具，低并发 / Dev tool, low concurrency |
| Privacy Agent REST | 单进程 Uvicorn | 计算用线程池 / Compute uses thread pool |
| Privacy Agent gRPC | ThreadPoolExecutor(10) | 同步 servicer / Sync servicer |
| Gateway | 单进程 asyncio | 纯 I/O 代理 / Pure I/O proxy |
| 生产部署 / Production | Gunicorn + Workers | K8s 多 Pod 替代 / K8s multi-Pod alternative |

## 12. WebSocket 支持详解 / WebSocket Support Details

### 12.1 ASGI WebSocket 协议 / ASGI WebSocket Protocol

Uvicorn 原生支持 WebSocket，通过 ASGI 协议处理：

```
客户端 / Client                    Uvicorn                       应用 / App
    │                              │                              │
    │── HTTP Upgrade ────────▶│                              │
    │                              │── websocket.connect ───▶│
    │                              │                              │
    │◀── 101 Switching ────────│◀── accept() ───────────────│
    │                              │                              │
    │── Frame (text) ─────────▶│── websocket.receive ───▶│
    │                              │                              │
    │◀── Frame (text) ─────────│◀── websocket.send ─────────│
    │                              │                              │
    │── Close Frame ───────────▶│── websocket.disconnect ─▶│
    │                              │                              │
```

### 12.2 FastAPI WebSocket 端点 / FastAPI WebSocket Endpoint

```python
# privacy_local_agent/routers/ws.py
from fastapi import APIRouter, WebSocket, WebSocketDisconnect
import asyncio
import json

router = APIRouter()

# 连接管理器
# Connection manager
class ConnectionManager:
    def __init__(self):
        self.active: list[WebSocket] = []
    
    async def connect(self, ws: WebSocket):
        await ws.accept()
        self.active.append(ws)
    
    def disconnect(self, ws: WebSocket):
        self.active.remove(ws)
    
    async def broadcast(self, message: str):
        for ws in self.active[:]:
            try:
                await ws.send_text(message)
            except Exception:
                self.active.remove(ws)

manager = ConnectionManager()


@router.websocket("/ws/classify")
async def classify_stream(ws: WebSocket):
    """ 实时分类 WebSocket 端点 """
    """ Real-time classification WebSocket endpoint """
    await manager.connect(ws)
    try:
        while True:
            # 接收分类请求
            # Receive classification request
            data = await ws.receive_text()
            request = json.loads(data)
            
            # 发送进度通知
            # Send progress notification
            await ws.send_json({
                "type": "progress",
                "stage": "rule_engine",
                "progress": 0.3,
            })
            
            # 执行分类（异步）
            # Execute classification (async)
            result = await run_classification(request)
            
            # 发送结果
            # Send result
            await ws.send_json({
                "type": "result",
                "data": result,
            })
    except WebSocketDisconnect:
        manager.disconnect(ws)
```

### 12.3 WebSocket 心跳与重连 / WebSocket Heartbeat & Reconnection

```python
# 服务端心跳检测
# Server-side heartbeat detection
@router.websocket("/ws/events")
async def events_stream(ws: WebSocket):
    await ws.accept()
    
    async def heartbeat():
        """ 每 30s 发送 ping """
        """ Send ping every 30s """
        while True:
            try:
                await ws.send_json({"type": "ping"})
                await asyncio.sleep(30)
            except Exception:
                break
    
    heartbeat_task = asyncio.create_task(heartbeat())
    
    try:
        while True:
            data = await ws.receive_text()
            msg = json.loads(data)
            
            # 客户端 pong 响应
            # Client pong response
            if msg.get("type") == "pong":
                continue
            
            # 处理业务消息
            # Handle business messages
            await handle_message(ws, msg)
    except WebSocketDisconnect:
        pass
    finally:
        heartbeat_task.cancel()
```

```typescript
// 客户端自动重连逻辑
// Client-side auto-reconnection logic
class ReconnectingWebSocket {
  private ws: WebSocket | null = null
  private retries = 0
  private maxRetries = 5
  
  constructor(private url: string) {
    this.connect()
  }
  
  private connect() {
    this.ws = new WebSocket(this.url)
    
    this.ws.onopen = () => {
      this.retries = 0  // 重置重试计数 / Reset retry count
      console.log('WebSocket connected')
    }
    
    this.ws.onclose = () => {
      if (this.retries < this.maxRetries) {
        // 指数退避重连
        // Exponential backoff reconnection
        const delay = Math.min(1000 * 2 ** this.retries, 30000)
        setTimeout(() => this.connect(), delay)
        this.retries++
      }
    }
    
    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.type === 'ping') {
        this.ws?.send(JSON.stringify({ type: 'pong' }))
        return
      }
      this.handleMessage(msg)
    }
  }
  
  private handleMessage(msg: any) {
    // 业务处理 / Business logic
  }
}
```

### 12.4 WebSocket 性能调优 / WebSocket Performance Tuning

| 参数 / Parameter | 默认值 / Default | 建议 / Recommended | 说明 / Description |
|---|---|---|---|
| ws_max_size | 16MB | 1MB | 最大消息大小 / Max message size |
| ws_ping_interval | 20s | 30s | Ping 间隔 / Ping interval |
| ws_ping_timeout | 20s | 10s | Ping 超时 / Ping timeout |
| 连接数限制 / Conn limit | 无 / None | 1000 | 单进程最大 WS 连接 / Max WS per process |

## 13. TLS/HTTPS 配置详解 / TLS/HTTPS Configuration Details

### 13.1 基本 TLS 配置 / Basic TLS Configuration

```bash
# 开发环境自签名证书
# Development self-signed certificate
openssl req -x509 -newkey rsa:2048 \
  -keyout key.pem -out cert.pem \
  -days 365 -nodes \
  -subj "/CN=localhost"

# 启动 HTTPS 服务
# Start HTTPS server
uvicorn app.main:app \
  --host 0.0.0.0 \
  --port 8443 \
  --ssl-certfile cert.pem \
  --ssl-keyfile key.pem
```

### 13.2 mTLS 双向认证 / mTLS Mutual Authentication

```python
# 编程式 mTLS 配置
# Programmatic mTLS configuration
import ssl
import uvicorn

def create_ssl_context() -> ssl.SSLContext:
    """ 创建 mTLS SSL 上下文 """
    """ Create mTLS SSL context """
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    
    # 服务器证书
    # Server certificate
    ctx.load_cert_chain(
        certfile="certs/server-cert.pem",
        keyfile="certs/server-key.pem",
    )
    
    # 要求客户端证书（双向 TLS）
    # Require client certificate (mutual TLS)
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.load_verify_locations(cafile="certs/ca-cert.pem")
    
    # 仅允许 TLS 1.3
    # Only allow TLS 1.3
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    
    return ctx


if __name__ == "__main__":
    config = uvicorn.Config(
        "app.main:app",
        host="0.0.0.0",
        port=8443,
        ssl_certfile="certs/server-cert.pem",
        ssl_keyfile="certs/server-key.pem",
        ssl_ca_certs="certs/ca-cert.pem",
        ssl_cert_reqs=ssl.CERT_REQUIRED,  # mTLS
    )
    server = uvicorn.Server(config)
    server.run()
```

### 13.3 证书轮换与热加载 / Certificate Rotation & Hot Reload

```python
# 证书轮换方案（不重启服务）
# Certificate rotation (without restart)
import ssl
import asyncio
from pathlib import Path

class CertWatcher:
    """ 监控证书文件变化并热重载 """
    """ Watch certificate file changes and hot reload """
    
    def __init__(self, cert_path: str, key_path: str, check_interval: int = 60):
        self.cert_path = Path(cert_path)
        self.key_path = Path(key_path)
        self.check_interval = check_interval
        self._last_mtime = 0.0
    
    async def watch(self):
        """ 定期检查证书更新 """
        """ Periodically check for certificate updates """
        while True:
            current_mtime = self.cert_path.stat().st_mtime
            if current_mtime > self._last_mtime:
                self._last_mtime = current_mtime
                await self._reload_certs()
            await asyncio.sleep(self.check_interval)
    
    async def _reload_certs(self):
        """ 重载证书（需要服务器支持） """
        """ Reload certificates (requires server support) """
        print(f"Certificate updated: {self.cert_path}")
        # 注意：Uvicorn 本身不支持热重载证书
        # Note: Uvicorn itself doesn't support hot cert reload
        # 生产环境使用 Envoy/Nginx 前置代理
        # Production uses Envoy/Nginx as front proxy
```

### 13.4 生产环境 TLS 架构 / Production TLS Architecture

```
┌─────────────────────────────────────────────────────┐
│  生产环境 TLS 终结架构                            │
│  Production TLS Termination Architecture            │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Client ──TLS──▶ Ingress/LB ──HTTP──▶ Uvicorn     │
│                   (TLS 终结)         (plain)        │
│                                                     │
│  Client ──mTLS──▶ Envoy Sidecar ──HTTP──▶ Uvicorn │
│                    (mTLS 终结)       (plain)        │
│                                                     │
│  Client ──TLS────────────────────▶ Uvicorn         │
│              (直连，开发环境)        (TLS)           │
│                                                     │
└─────────────────────────────────────────────────────┘
```

### 13.5 TLS 配置对比 / TLS Configuration Comparison

| 场景 / Scenario | 配置 / Config | 说明 / Description |
|---|---|---|
| 本地开发 / Local dev | 无 TLS / No TLS | http://localhost:8079 |
| K8s Ingress | Ingress TLS 终结 | cert-manager 自动证书 / Auto cert |
| Service Mesh | Envoy mTLS | Istio 自动注入 / Auto inject |
| 裸机部署 / Bare metal | Uvicorn --ssl | 自管证书 / Self-managed cert |
| Docker Compose | Nginx 前置 | 挂载证书 volume / Mount cert volume |

## 14. 性能基准与调优 / Performance Benchmarks & Tuning

### 14.1 基准测试方法 / Benchmarking Methodology

```bash
# 使用 wrk 进行 HTTP 基准测试
# HTTP benchmark using wrk

# 基本 GET 请求
# Basic GET request
wrk -t4 -c100 -d30s http://localhost:8079/health

# POST 请求（带 JSON body）
# POST request (with JSON body)
wrk -t4 -c100 -d30s -s mask.lua http://localhost:8079/api/v1/mask

# mask.lua 内容 / mask.lua content:
# wrk.method = "POST"
# wrk.headers["Content-Type"] = "application/json"
# wrk.body = '{"data": {"name": "test"}, "strategy": "partial"}'
```

```bash
# 使用 hey 进行更简单的基准测试
# Simpler benchmark using hey

# 100 并发，10000 请求
# 100 concurrent, 10000 requests
hey -n 10000 -c 100 \
  -m POST \
  -H "Content-Type: application/json" \
  -d '{"data":{"name":"test"}}' \
  http://localhost:8079/api/v1/mask
```

### 14.2 典型性能数据 / Typical Performance Data

```
环境 / Environment: MacBook Pro M1, Python 3.11, Uvicorn 0.27

端点 / Endpoint              | RPS     | P50    | P99    | 说明 / Notes
---------------------------|---------|--------|--------|-------------
GET /health                | ~15000  | 2ms    | 8ms    | 纯内存 / Pure memory
POST /api/v1/mask (small)  | ~3000   | 15ms   | 45ms   | 5 字段 / 5 fields
POST /api/v1/mask (large)  | ~500    | 80ms   | 200ms  | 100 字段 / 100 fields
POST /api/v1/dp/count      | ~2500   | 20ms   | 60ms   | 计算密集 / Compute heavy
POST /api/v1/classify      | ~800    | 50ms   | 150ms  | 规则引擎 / Rule engine
```

### 14.3 事件循环优化 / Event Loop Optimization

```python
# 避免阻塞事件循环的常见模式
# Common patterns that block the event loop

# ✘ 错误：同步阻塞调用
# ✘ Wrong: Synchronous blocking call
@app.post("/slow")
async def slow_endpoint():
    time.sleep(5)  # 阻塞整个事件循环！/ Blocks entire event loop!
    return {"status": "done"}

# ✔ 正确：使用 run_in_executor
# ✔ Correct: Use run_in_executor
import asyncio
from functools import partial

@app.post("/correct")
async def correct_endpoint():
    loop = asyncio.get_event_loop()
    # CPU 密集型任务放入线程池
    # CPU-intensive task in thread pool
    result = await loop.run_in_executor(
        None,  # 默认线程池 / Default thread pool
        partial(heavy_computation, data),
    )
    return {"result": result}

# ✔ 正确：异步 I/O
# ✔ Correct: Async I/O
@app.post("/async-io")
async def async_io_endpoint():
    async with httpx.AsyncClient() as client:
        resp = await client.get("http://other-service/data")
    return resp.json()
```

### 14.4 连接池与 Keep-Alive 调优 / Connection Pool & Keep-Alive Tuning

```python
# Uvicorn 服务器端 Keep-Alive 配置
# Uvicorn server-side Keep-Alive configuration
config = uvicorn.Config(
    "app.main:app",
    # Keep-Alive 超时（秒）
    # Keep-Alive timeout (seconds)
    timeout_keep_alive=75,  # 默认 5s，生产建议 75s
    
    # 最大并发连接数（受 OS fd 限制）
    # Max concurrent connections (limited by OS fd)
    limit_concurrency=1000,
    
    # 最大请求行大小
    # Max request line size
    limit_max_request_line=8190,
    
    # 最大请求头大小
    # Max request header size
    limit_max_request_headers=32768,
    
    # 背压控制：等待队列大小
    # Backpressure: waiting queue size
    backlog=2048,
)
```

### 14.5 性能调优检查清单 / Performance Tuning Checklist

| 优化项 / Optimization | 影响 / Impact | 建议 / Recommendation |
|---|---|---|
| 避免阻塞调用 / Avoid blocking | 高 / High | 所有 I/O 用 async / All I/O async |
| CPU 任务线程池 / CPU thread pool | 高 / High | run_in_executor |
| Keep-Alive 复用 / Keep-Alive reuse | 中 / Medium | timeout_keep_alive=75 |
| 连接池大小 / Pool size | 中 / Medium | 根据并发调整 / Adjust by concurrency |
| JSON 序列化 / JSON serialization | 低 / Low | orjson 替代 / orjson alternative |
| 日志异步 / Async logging | 低 / Low | QueueHandler |
| 工作进程数 / Worker count | 高 / High | CPU核数 * 2 / CPU cores * 2 |

## 15. 容器化部署实践 / Containerized Deployment Practices

Uvicorn 在容器环境中的部署需要考虑镜像构建、资源限制、健康检查、优雅关闭等多个方面。合理的容器化实践能确保服务在 Kubernetes 等编排平台上稳定运行。

Deploying Uvicorn in container environments requires considering image building, resource limits, health checks, and graceful shutdown. Proper containerization practices ensure stable operation on orchestration platforms like Kubernetes.

### 15.1 多阶段 Dockerfile / Multi-stage Dockerfile

```dockerfile
# Dockerfile - Uvicorn 服务容器化 / Uvicorn service containerization
# 多阶段构建减小镜像体积 / Multi-stage build reduces image size

# === 构建阶段 / Build stage ===
FROM python:3.11-slim AS builder

WORKDIR /app

# 安装构建依赖 / Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# 先复制依赖文件，利用 Docker 层缓存
# Copy dependency files first for Docker layer caching
COPY requirements-core.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements-core.txt

# === 运行阶段 / Runtime stage ===
FROM python:3.11-slim AS runtime

# 安全：创建非 root 用户 / Security: create non-root user
RUN groupadd -r appuser && useradd -r -g appuser -d /app -s /sbin/nologin appuser

WORKDIR /app

# 从构建阶段复制依赖 / Copy deps from builder
COPY --from=builder /install /usr/local

# 复制应用代码 / Copy application code
COPY privacy_local_agent/ ./privacy_local_agent/
COPY pyproject.toml .

# 设置环境变量 / Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PRIVACY_REST_HOST=0.0.0.0 \
    PRIVACY_REST_PORT=8079 \
    PRIVACY_GRPC_HOST=0.0.0.0 \
    PRIVACY_GRPC_PORT=50051 \
    PRIVACY_LOG_FORMAT=json

# 切换到非 root 用户 / Switch to non-root user
USER appuser

# 暴露端口 / Expose ports
EXPOSE 8079 50051

# 健康检查 / Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8079/health')" || exit 1

# 启动命令 / Start command
# 使用 exec form 确保信号正确传递 / Use exec form for proper signal handling
CMD ["python", "-m", "uvicorn", "privacy_local_agent.main:app", \
     "--host", "0.0.0.0", "--port", "8079", \
     "--workers", "2", "--loop", "uvloop", "--http", "httptools"]
```

### 15.2 Kubernetes 部署配置 / Kubernetes Deployment Config

```yaml
# deploy/k8s/uvicorn-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: privacy-agent
  labels:
    app: privacy-agent
    component: uvicorn
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1          # 滚动更新最多多 1 个 Pod / Max 1 extra Pod
      maxUnavailable: 0    # 不允许不可用 / Zero downtime
  selector:
    matchLabels:
      app: privacy-agent
  template:
    metadata:
      labels:
        app: privacy-agent
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8079"
        prometheus.io/path: "/metrics"
    spec:
      # 优雅关闭等待时间 / Graceful shutdown period
      terminationGracePeriodSeconds: 60
      
      containers:
        - name: privacy-agent
          image: PrivShield:0.1.0
          ports:
            - containerPort: 8079
              name: http
            - containerPort: 50051
              name: grpc
          
          # 资源限制 / Resource limits
          resources:
            requests:
              cpu: "250m"
              memory: "256Mi"
            limits:
              cpu: "1000m"
              memory: "512Mi"
          
          # 存活探针 / Liveness probe
          livenessProbe:
            httpGet:
              path: /health
              port: 8079
            initialDelaySeconds: 10
            periodSeconds: 15
            timeoutSeconds: 3
            failureThreshold: 3
          
          # 就绪探针 / Readiness probe
          readinessProbe:
            httpGet:
              path: /health
              port: 8079
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 2
            failureThreshold: 2
          
          # 启动探针（慢启动保护）/ Startup probe (slow start protection)
          startupProbe:
            httpGet:
              path: /health
              port: 8079
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 12  # 最多等 60s / Wait up to 60s
          
          # 生命周期钩子 / Lifecycle hooks
          lifecycle:
            preStop:
              exec:
                # 优雅关闭：先停止接收新请求，等待现有请求完成
                # Graceful: stop accepting new requests, wait for existing
                command: ["sh", "-c", "sleep 10 && kill -SIGTERM 1"]
          
          env:
            - name: PRIVACY_LOG_FORMAT
              value: "json"
            - name: PRIVACY_LOG_LEVEL
              value: "INFO"
            - name: OTEL_EXPORTER_OTLP_ENDPOINT
              value: "http://otel-collector:4317"
```

### 15.3 容器内优雅关闭 / Graceful Shutdown in Containers

```python
"""容器环境优雅关闭处理 / Graceful shutdown handling in containers

Kubernetes 关闭流程 / Kubernetes shutdown flow:
1. Pod 标记为 Terminating / Pod marked Terminating
2. 从 Service endpoints 移除 / Removed from Service endpoints
3. 执行 preStop hook / Execute preStop hook
4. 发送 SIGTERM / Send SIGTERM
5. 等待 terminationGracePeriodSeconds / Wait grace period
6. 发送 SIGKILL / Send SIGKILL
"""
import asyncio
import signal
import logging
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI

logger = logging.getLogger(__name__)

# 关闭状态跟踪 / Shutdown state tracking
_shutdown_event = asyncio.Event()
_active_requests = 0


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理 / Application lifecycle management"""
    # === 启动阶段 / Startup phase ===
    logger.info("Application starting up...")
    
    # 注册信号处理 / Register signal handlers
    loop = asyncio.get_event_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, _handle_shutdown_signal, sig)
    
    yield
    
    # === 关闭阶段 / Shutdown phase ===
    logger.info("Application shutting down...")
    
    # 1. 停止接受新连接 / Stop accepting new connections
    _shutdown_event.set()
    
    # 2. 等待现有请求完成（最多 30s）/ Wait for existing requests (max 30s)
    for _ in range(300):  # 30s * 10 checks/s
        if _active_requests == 0:
            break
        await asyncio.sleep(0.1)
    
    if _active_requests > 0:
        logger.warning(f"Force shutdown with {_active_requests} active requests")
    
    # 3. 清理资源 / Cleanup resources
    logger.info("Shutdown complete")


def _handle_shutdown_signal(sig: signal.Signals):
    """处理关闭信号 / Handle shutdown signal"""
    logger.info(f"Received signal {sig.name}, initiating graceful shutdown")
    _shutdown_event.set()


app = FastAPI(lifespan=lifespan)


@app.middleware("http")
async def track_requests(request, call_next):
    """跟踪活跃请求数 / Track active request count"""
    global _active_requests
    
    # 如果正在关闭，拒绝新请求 / If shutting down, reject new requests
    if _shutdown_event.is_set():
        from fastapi.responses import JSONResponse
        return JSONResponse(
            status_code=503,
            content={"detail": "Service shutting down"},
            headers={"Retry-After": "30"},
        )
    
    _active_requests += 1
    try:
        response = await call_next(request)
        return response
    finally:
        _active_requests -= 1
```

### 15.4 容器化检查清单 / Containerization Checklist

| 检查项 / Check Item | 状态 / Status | 说明 / Description |
|---|---|---|
| 非 root 用户 / Non-root user | ✅ 必须 / Required | 安全最佳实践 / Security best practice |
| 多阶段构建 / Multi-stage build | ✅ 推荐 / Recommended | 减小镜像体积 / Reduce image size |
| 健康检查 / Health check | ✅ 必须 / Required | K8s 探针依赖 / K8s probe dependency |
| 优雅关闭 / Graceful shutdown | ✅ 必须 / Required | 防止请求丢失 / Prevent request loss |
| JSON 日志 / JSON logging | ✅ 推荐 / Recommended | 便于日志采集 / Easy log collection |
| 资源限制 / Resource limits | ✅ 必须 / Required | 防止资源耗尽 / Prevent exhaustion |
| 只读文件系统 / Read-only FS | ✅ 推荐 / Recommended | 安全加固 / Security hardening |
| 最小基础镜像 / Minimal base | ✅ 推荐 / Recommended | 减少攻击面 / Reduce attack surface |

## 16. 可观测性集成 / Observability Integration

可观测性三大支柱（日志、指标、追踪）是生产环境运维的基础。Uvicorn 作为 ASGI 服务器，需要与 OpenTelemetry、Prometheus 等工具无缝集成，提供全方位的服务可见性。

The three pillars of observability (logs, metrics, traces) are the foundation of production operations. Uvicorn, as an ASGI server, needs seamless integration with OpenTelemetry, Prometheus, and similar tools for comprehensive service visibility.

### 16.1 结构化日志配置 / Structured Logging Configuration

```python
"""生产级结构化日志配置 / Production-grade structured logging configuration"""
import logging
import json
import sys
from datetime import datetime, timezone
from contextvars import ContextVar

# 请求上下文 / Request context
request_id_var: ContextVar[str] = ContextVar('request_id', default='-')


class JSONFormatter(logging.Formatter):
    """结构化 JSON 日志格式器 / Structured JSON log formatter
    
    输出示例 / Output example:
    {
      "timestamp": "2024-06-15T10:30:00.123Z",
      "level": "INFO",
      "logger": "uvicorn.access",
      "message": "GET /api/v1/mask 200",
      "request_id": "req-abc123",
      "service": "privacy-agent",
      "extra": {...}
    }
    """
    
    def __init__(self, service_name: str = "privacy-agent"):
        super().__init__()
        self.service_name = service_name
    
    def format(self, record: logging.LogRecord) -> str:
        log_entry = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
            "request_id": request_id_var.get(),
            "service": self.service_name,
        }
        
        # 添加异常信息 / Add exception info
        if record.exc_info:
            log_entry["exception"] = self.formatException(record.exc_info)
        
        # 添加额外字段 / Add extra fields
        if hasattr(record, 'extra_data'):
            log_entry["extra"] = record.extra_data
        
        return json.dumps(log_entry, ensure_ascii=False, default=str)


def configure_logging(level: str = "INFO", format_type: str = "json"):
    """配置日志系统 / Configure logging system
    
    Args:
        level: 日志级别 / Log level
        format_type: 'json' 或 'text' / 'json' or 'text'
    """
    root_logger = logging.getLogger()
    root_logger.setLevel(getattr(logging, level.upper()))
    
    # 清除默认 handler / Clear default handlers
    root_logger.handlers.clear()
    
    # 控制台输出 / Console output
    handler = logging.StreamHandler(sys.stdout)
    
    if format_type == "json":
        handler.setFormatter(JSONFormatter())
    else:
        handler.setFormatter(logging.Formatter(
            "%(asctime)s | %(levelname)-8s | %(name)s | %(message)s"
        ))
    
    root_logger.addHandler(handler)
    
    # 降低第三方库日志级别 / Reduce third-party log levels
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
```

### 16.2 Prometheus 指标集成 / Prometheus Metrics Integration

```python
"""Prometheus 指标集成 / Prometheus metrics integration"""
from prometheus_client import (
    Counter, Histogram, Gauge, Info,
    generate_latest, CONTENT_TYPE_LATEST,
)
from fastapi import FastAPI, Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
import time

# === 指标定义 / Metric definitions ===

# 请求计数器 / Request counter
REQUEST_COUNT = Counter(
    'http_requests_total',
    '总请求数 / Total HTTP requests',
    ['method', 'endpoint', 'status_code']
)

# 请求延迟 / Request latency
REQUEST_LATENCY = Histogram(
    'http_request_duration_seconds',
    '请求延迟分布 / Request latency distribution',
    ['method', 'endpoint'],
    buckets=[0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
)

# 活跃连接数 / Active connections
ACTIVE_CONNECTIONS = Gauge(
    'http_active_connections',
    '当前活跃连接数 / Current active connections'
)

# 服务信息 / Service info
SERVICE_INFO = Info(
    'privacy_agent',
    '服务版本信息 / Service version info'
)
SERVICE_INFO.info({
    'version': '0.1.0',
    'runtime': 'uvicorn',
    'python_version': '3.11',
})


class PrometheusMiddleware(BaseHTTPMiddleware):
    """指标采集中间件 / Metrics collection middleware"""
    
    async def dispatch(self, request: Request, call_next):
        # 跳过指标端点本身 / Skip metrics endpoint itself
        if request.url.path == "/metrics":
            return await call_next(request)
        
        method = request.method
        endpoint = request.url.path
        
        ACTIVE_CONNECTIONS.inc()
        start_time = time.perf_counter()
        
        try:
            response = await call_next(request)
            status_code = response.status_code
        except Exception:
            status_code = 500
            raise
        finally:
            duration = time.perf_counter() - start_time
            ACTIVE_CONNECTIONS.dec()
            
            # 记录指标 / Record metrics
            REQUEST_COUNT.labels(method, endpoint, str(status_code)).inc()
            REQUEST_LATENCY.labels(method, endpoint).observe(duration)
        
        return response


# 指标暴露端点 / Metrics exposure endpoint
def setup_metrics(app: FastAPI):
    """配置指标端点 / Configure metrics endpoint"""
    app.add_middleware(PrometheusMiddleware)
    
    @app.get("/metrics", include_in_schema=False)
    async def metrics():
        return Response(
            content=generate_latest(),
            media_type=CONTENT_TYPE_LATEST,
        )
```

### 16.3 OpenTelemetry 分布式追踪 / OpenTelemetry Distributed Tracing

```python
"""分布式追踪集成 / Distributed tracing integration"""
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import Resource
from fastapi import FastAPI


def setup_tracing(app: FastAPI, service_name: str = "privacy-agent"):
    """配置分布式追踪 / Configure distributed tracing
    
    Args:
        app: FastAPI 应用 / FastAPI application
        service_name: 服务名称（显示在追踪系统中）/ Service name in trace UI
    """
    # 创建资源标识 / Create resource identifier
    resource = Resource.create({
        "service.name": service_name,
        "service.version": "0.1.0",
        "deployment.environment": "production",
    })
    
    # 配置 TracerProvider / Configure TracerProvider
    provider = TracerProvider(resource=resource)
    
    # OTLP 导出器（发送到 Jaeger/Tempo）/ OTLP exporter (to Jaeger/Tempo)
    otlp_exporter = OTLPSpanExporter(
        endpoint="http://otel-collector:4317",
        insecure=True,
    )
    
    # 批量导出（减少网络开销）/ Batch export (reduce network overhead)
    provider.add_span_processor(
        BatchSpanProcessor(
            otlp_exporter,
            max_queue_size=2048,
            max_export_batch_size=512,
            export_timeout_millis=30000,
        )
    )
    
    trace.set_tracer_provider(provider)
    
    # 自动检测 FastAPI / Auto-instrument FastAPI
    FastAPIInstrumentor.instrument_app(
        app,
        excluded_urls="health,metrics",  # 排除健康检查 / Exclude health checks
    )
```

### 16.4 可观测性架构总览 / Observability Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    可观测性架构 / Observability Architecture          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌───────────┐   ┌───────────┐   ┌───────────┐  │
│  │  Logs     │   │  Metrics  │   │  Traces   │  │
│  │  日志     │   │  指标     │   │  追踪     │  │
│  └─────┬─────┘   └─────┬─────┘   └─────┬─────┘  │
│        │               │               │         │
│        ▼               ▼               ▼         │
│  ┌─────────────────────────────────────────┐  │
│  │        OpenTelemetry Collector          │  │
│  └────────┬──────────┬──────────┬──────────┘  │
│           │          │          │              │
│           ▼          ▼          ▼              │
│  ┌─────────┐ ┌──────────┐ ┌─────────┐    │
│  │  Loki   │ │Prometheus│ │ Jaeger/ │    │
│  │         │ │          │ │ Tempo   │    │
│  └────┬────┘ └────┬─────┘ └────┬────┘    │
│       │          │            │            │
│       └──────────┼────────────┘            │
│                  ▼                          │
│         ┌──────────────┐                  │
│         │   Grafana    │                  │
│         │  统一看板    │                  │
│         └──────────────┘                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 17. 高可用架构设计 / High Availability Architecture Design

高可用（HA）架构确保服务在部分组件故障时仍能正常运行。对于 Uvicorn 服务，HA 涉及多实例部署、负载均衡、故障转移、熔断降级等多个层面。

High Availability (HA) architecture ensures services remain operational despite partial component failures. For Uvicorn services, HA involves multi-instance deployment, load balancing, failover, circuit breaking, and graceful degradation.

### 17.1 多实例部署模式 / Multi-instance Deployment Patterns

```python
"""多实例部署与负载均衡 / Multi-instance deployment & load balancing

部署拓扑 / Deployment topology:

                    ┌─────────────┐
                    │   Ingress   │
                    │  / LB       │
                    └──────┬──────┘
                           │
              ┌───────────┼───────────┐
              │            │            │
              ▼            ▼            ▼
     ┌────────────┐ ┌────────────┐ ┌────────────┐
     │  Pod 1     │ │  Pod 2     │ │  Pod 3     │
     │  Uvicorn   │ │  Uvicorn   │ │  Uvicorn   │
     │  x2 workers│ │  x2 workers│ │  x2 workers│
     └────────────┘ └────────────┘ └────────────┘
              │            │            │
              └───────────┼───────────┘
                           │
                    ┌──────┴──────┐
                    │  Shared     │
                    │  State      │
                    │  (Redis/DB) │
                    └─────────────┘
"""
import asyncio
import httpx
from dataclasses import dataclass
from enum import Enum


class InstanceHealth(Enum):
    """实例健康状态 / Instance health status"""
    HEALTHY = "healthy"
    DEGRADED = "degraded"
    UNHEALTHY = "unhealthy"


@dataclass
class ServiceInstance:
    """服务实例 / Service instance"""
    host: str
    port: int
    health: InstanceHealth = InstanceHealth.HEALTHY
    active_connections: int = 0
    error_count: int = 0
    last_health_check: float = 0.0
    
    @property
    def address(self) -> str:
        return f"{self.host}:{self.port}"


class LoadBalancer:
    """负载均衡器 / Load balancer
    
    支持多种策略 / Supports multiple strategies:
    - Round Robin: 轮询 / Round robin
    - Least Connections: 最少连接 / Least connections
    - Weighted: 加权 / Weighted
    """
    
    def __init__(self, instances: list[ServiceInstance], strategy: str = "least_conn"):
        self.instances = instances
        self.strategy = strategy
        self._rr_index = 0
    
    def select_instance(self) -> ServiceInstance | None:
        """选择目标实例 / Select target instance"""
        healthy = [i for i in self.instances if i.health == InstanceHealth.HEALTHY]
        
        if not healthy:
            # 降级：尝试 degraded 实例 / Degrade: try degraded instances
            healthy = [i for i in self.instances if i.health != InstanceHealth.UNHEALTHY]
        
        if not healthy:
            return None
        
        if self.strategy == "round_robin":
            instance = healthy[self._rr_index % len(healthy)]
            self._rr_index += 1
            return instance
        
        elif self.strategy == "least_conn":
            return min(healthy, key=lambda i: i.active_connections)
        
        return healthy[0]


class CircuitBreaker:
    """熔断器 / Circuit breaker
    
    状态机 / State machine:
    CLOSED ──(失败率>阈值)──▶ OPEN ──(超时)──▶ HALF_OPEN
      ▲                                        │
      └────────(探测成功)──────────────────┘
    
    CLOSED: 正常通过请求 / Normal pass-through
    OPEN: 拒绝所有请求 / Reject all requests
    HALF_OPEN: 允许少量探测 / Allow limited probes
    """
    
    def __init__(self, failure_threshold: int = 5, recovery_timeout: float = 30.0):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self._state = "CLOSED"
        self._failure_count = 0
        self._last_failure_time = 0.0
    
    @property
    def state(self) -> str:
        if self._state == "OPEN":
            # 检查是否可以进入 HALF_OPEN / Check if can enter HALF_OPEN
            import time
            if time.time() - self._last_failure_time > self.recovery_timeout:
                self._state = "HALF_OPEN"
        return self._state
    
    def record_success(self):
        """记录成功 / Record success"""
        self._failure_count = 0
        self._state = "CLOSED"
    
    def record_failure(self):
        """记录失败 / Record failure"""
        import time
        self._failure_count += 1
        self._last_failure_time = time.time()
        
        if self._failure_count >= self.failure_threshold:
            self._state = "OPEN"
    
    def allow_request(self) -> bool:
        """是否允许请求通过 / Whether to allow request"""
        state = self.state
        if state == "CLOSED":
            return True
        elif state == "HALF_OPEN":
            return True  # 允许探测 / Allow probe
        return False  # OPEN: 拒绝 / Reject
```

### 17.2 健康检查与故障检测 / Health Check & Failure Detection

```python
"""健康检查与故障检测 / Health check & failure detection"""
import asyncio
import time
import httpx
import logging

logger = logging.getLogger(__name__)


class HealthChecker:
    """健康检查器 / Health checker
    
    检查维度 / Check dimensions:
    1. Liveness: 进程是否存活 / Is process alive
    2. Readiness: 是否能处理请求 / Can it handle requests
    3. Dependency: 依赖服务是否正常 / Are dependencies healthy
    """
    
    def __init__(self, instances: list[ServiceInstance], interval: float = 10.0):
        self.instances = instances
        self.interval = interval
        self._running = False
    
    async def start(self):
        """启动健康检查循环 / Start health check loop"""
        self._running = True
        while self._running:
            await self._check_all()
            await asyncio.sleep(self.interval)
    
    def stop(self):
        self._running = False
    
    async def _check_all(self):
        """检查所有实例 / Check all instances"""
        async with httpx.AsyncClient(timeout=5.0) as client:
            tasks = [self._check_instance(client, inst) for inst in self.instances]
            await asyncio.gather(*tasks, return_exceptions=True)
    
    async def _check_instance(self, client: httpx.AsyncClient, instance: ServiceInstance):
        """检查单个实例 / Check single instance"""
        try:
            # Liveness 检查 / Liveness check
            resp = await client.get(f"http://{instance.address}/health")
            
            if resp.status_code == 200:
                data = resp.json()
                
                # 检查依赖状态 / Check dependency status
                if data.get("dependencies", {}).get("all_healthy", True):
                    instance.health = InstanceHealth.HEALTHY
                else:
                    instance.health = InstanceHealth.DEGRADED
                    logger.warning(f"Instance {instance.address} degraded: {data}")
            else:
                instance.health = InstanceHealth.UNHEALTHY
                instance.error_count += 1
                logger.error(f"Instance {instance.address} unhealthy: HTTP {resp.status_code}")
        
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            instance.health = InstanceHealth.UNHEALTHY
            instance.error_count += 1
            logger.error(f"Instance {instance.address} unreachable: {e}")
        
        instance.last_health_check = time.time()
```

### 17.3 高可用检查清单 / High Availability Checklist

| 层面 / Layer | 检查项 / Check Item | 状态 / Status | 说明 / Description |
|---|---|---|---|
| 部署 / Deploy | 多副本 / Multi-replica | ✅ | ≥ 3 个 Pod / ≥ 3 Pods |
| 部署 / Deploy | 反亲和性 / Anti-affinity | ✅ | 分散在不同节点 / Spread across nodes |
| 部署 / Deploy | 滚动更新 / Rolling update | ✅ | 零停机 / Zero downtime |
| 网络 / Network | 负载均衡 / Load balancing | ✅ | K8s Service / Ingress |
| 网络 / Network | 连接超时 / Connection timeout | ✅ | 防止慢连接 / Prevent slow connections |
| 容错 / Fault | 熔断器 / Circuit breaker | ✅ | 防止级联失败 / Prevent cascade |
| 容错 / Fault | 重试策略 / Retry policy | ✅ | 指数退避 / Exponential backoff |
| 容错 / Fault | 降级方案 / Degradation | ✅ | 返回缓存/默认值 / Cache/default |
| 状态 / State | 无状态设计 / Stateless | ✅ | 状态外置 / External state |
| 状态 / State | 分布式预算 / Distributed budget | ✅ | SQLite/Redis |
