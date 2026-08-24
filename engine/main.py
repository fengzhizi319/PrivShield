"""REST API 入口模块（应用装配）。

基于 FastAPI 构建，负责应用级装配：生命周期管理、可观测性中间件、
Prometheus metrics 暴露，以及把按域拆分到 ``routers/*`` 的子路由统一挂载。

各端点的请求模型定义在 ``schemas.py``，跨路由共享的服务单例、安全依赖与
异常映射定义在 ``deps.py``；本模块仅做组装，不再内联具体端点实现。

REST API entrypoint built with FastAPI. Endpoint implementations are split into
``routers/*``; request models live in ``schemas.py`` and shared dependencies
(service singleton, security deps, exception mapping) live in ``deps.py``.
"""

# =============================================================================
# Phase 1: Environment bootstrap — load .env files before any other imports
# Phase 1: 环境引导 — 在任何其他导入之前加载 .env 配置文件
# =============================================================================

# Load environment variables from .env / config/env/<profile>.env so that
# downstream imports (e.g. security/config.py) can read settings at module level.
# 从 .env 及 config/env/<profile>.env 加载环境变量，确保下游模块（如 security/config.py）
# 在模块级导入时即可读取到配置值。
from .env_loader import load_env_file
load_env_file()

# =============================================================================
# Phase 2: Standard-library & third-party imports
# Phase 2: 标准库与第三方库导入
# =============================================================================

import os
from contextlib import asynccontextmanager

# FastAPI core framework for building REST API endpoints
# FastAPI 核心框架，用于构建 REST API 端点
from fastapi import FastAPI
# GZip middleware for compressing large HTTP responses to reduce bandwidth
# GZip 中间件，压缩大型 HTTP 响应以减少网络带宽消耗
from fastapi.middleware.gzip import GZipMiddleware
# Starlette base middleware class for implementing custom ASGI middleware
# Starlette 基础中间件类，用于实现自定义 ASGI 中间件
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

# =============================================================================
# Phase 3: Internal package imports — observability, security, routers
# Phase 3: 内部包导入 — 可观测性、安全、路由模块
# =============================================================================

# Structured logging configuration (text/json format, per-module log levels)
# 结构化日志配置（支持 text/json 格式、按模块设置日志级别）
from .observability.logging_config import configure_logging
# Prometheus metrics ASGI app factory — creates a standalone metrics endpoint
# Prometheus 指标 ASGI 应用工厂 — 创建独立的指标暴露端点
from .observability.metrics import make_asgi_app
# Observability middleware: injects request_id, records access logs & metrics
# 可观测性中间件：注入 request_id、记录访问日志与 Prometheus 指标
from .observability.middleware import ObservabilityMiddleware
# OpenTelemetry distributed tracing initializer (optional, no-op if not configured)
# OpenTelemetry 分布式追踪初始化器（可选，未配置时为 no-op）
from .observability.tracing import init_tracing

# Re-export the PrivacyService singleton so that tests and callers can access
# it via ``from engine.main import service``.
# 再导出 PrivacyService 单例，使测试和外部调用方可通过
# ``from engine.main import service`` 获取。
from .deps import service  # noqa: F401

# Medical pipeline router (multi-step classification + privacy processing)
# 医疗流水线路由（多步分类 + 隐私处理组合端点）
from .pipeline import router as pipeline_router

# Domain-specific routers: each module exposes a ``router`` (APIRouter instance)
# 按域拆分的路由：每个模块暴露一个 ``router``（APIRouter 实例）
#   health      — /v1/health, /v1/readyz     健康检查与就绪探针
#   mask        — /v1/privacy/mask/*          字段级脱敏
#   dp          — /v1/privacy/dp/*            差分隐私（Laplace/Gaussian）
#   ldp         — /v1/privacy/ldp/*           本地差分隐私
#   kano        — /v1/privacy/k_anonymize/*   K-匿名化
#   qol         — /v1/privacy/qol/*           查询混淆
#   budget      — /v1/privacy/budget          隐私预算查询
#   profile     — /v1/privacy/profile         隐私参数 profile
#   file        — /v1/privacy/file/*          文件上传与处理
#   ops         — /v1/ops/diagnostics         运维诊断
#   medical     — /v1/medical/*               医疗数据流水线
#   dynclassification — /v1/classify/*        三层漏斗动态分类
from .routers import budget, dp, dynclassification, file, health, kano, ldp, mask, medical, ops, profile, qol

# ASGI-level API Key middleware for mounted sub-apps (e.g. /metrics)
# ASGI 层 API Key 中间件，用于挂载的子应用（如 /metrics）
from .security.auth import ApiKeyAuthAsgiMiddleware
# Security settings loader — reads PRIVACY_AUTH_ENABLED etc. from environment
# 安全配置加载器 — 从环境变量读取 PRIVACY_AUTH_ENABLED 等配置
from .security.config import get_security_settings


class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    """Security response headers middleware.

    Injects common security headers into every HTTP response to defend against
    MIME sniffing, clickjacking, and reflected XSS attacks.

    安全响应头中间件。
    自动为所有 HTTP 响应注入通用安全响应头，防范 MIME 嗅探、点击劫持与反射型 XSS 攻击。
    """

    async def dispatch(self, request: Request, call_next) -> Response:
        # Pass the request down the middleware chain and get the response
        # 将请求传递给下游中间件/路由处理函数，获取响应对象
        response: Response = await call_next(request)
        # Prevent browsers from MIME-type sniffing — forces browser to respect
        # the declared Content-Type, mitigating drive-by download attacks.
        # 禁止浏览器进行 MIME 类型嗅探 — 强制浏览器遵守声明的 Content-Type，
        # 防止"顺手牵羊"下载攻击（drive-by download）。
        response.headers["X-Content-Type-Options"] = "nosniff"
        # Completely deny iframe embedding — prevents clickjacking by ensuring
        # the page cannot be rendered inside a <frame>/<iframe>.
        # 完全禁止 iframe 嵌入 — 通过确保页面无法在 <frame>/<iframe> 中渲染来
        # 防止点击劫持攻击。
        response.headers["X-Frame-Options"] = "DENY"
        # Enable browser built-in XSS filter — when a reflected XSS is detected,
        # block the page from rendering instead of sanitizing the script.
        # 启用浏览器内置 XSS 过滤器 — 当检测到反射型 XSS 时，阻止页面渲染而非
        # 尝试清理脚本代码（更安全）。
        response.headers["X-XSS-Protection"] = "1; mode=block"
        return response


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager.

    Startup:  initialize structured logging and optional OpenTelemetry tracing.
    Shutdown: run cleanup logic (currently a no-op placeholder for future use).

    应用生命周期管理器。
    启动阶段：初始化结构化日志与可选的 OpenTelemetry 分布式追踪。
    关闭阶段：执行清理逻辑（当前为占位，供未来扩展如关闭连接池、刷新缓冲区等）。

    Args:
        app: FastAPI application instance / FastAPI 应用实例。
    """
    # --- Startup phase / 启动阶段 ---

    # Configure structured logging: reads format/level/service-name from env vars
    # so that log output is consistent across all modules from the very first line.
    # 配置结构化日志：从环境变量读取日志格式、级别、服务名，
    # 确保所有模块从第一行日志起就保持一致的输出格式。
    configure_logging(
        log_level=os.environ.get("PRIVACY_LOG_LEVEL", "INFO"),
        json_format=os.environ.get("PRIVACY_LOG_FORMAT", "text").lower() == "json",
        service_name=os.environ.get("PRIVACY_SERVICE_NAME", "PrivShield"),
    )

    # Initialize OpenTelemetry distributed tracing (optional).
    # If OTEL_EXPORTER_OTLP_ENDPOINT is not set, this is a no-op.
    # Service name falls back to PRIVACY_SERVICE_NAME for unified log/trace correlation.
    # 初始化 OpenTelemetry 分布式追踪（可选）。
    # 若未设置 OTEL_EXPORTER_OTLP_ENDPOINT 则为 no-op。
    # 服务名回退到 PRIVACY_SERVICE_NAME，以保持日志与追踪的统一关联。
    init_tracing(
        endpoint=os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT"),
        service_name=os.environ.get(
            "OTEL_SERVICE_NAME",
            os.environ.get("PRIVACY_SERVICE_NAME", "PrivShield"),
        ),
    )

    # --- Application running / 应用运行中 ---

    # Yield control back to the ASGI server (uvicorn); the app now serves requests.
    # 将控制权交还给 ASGI 服务器（uvicorn）；应用开始处理请求。
    try:
        yield
    finally:
        # --- Shutdown phase / 关闭阶段 ---
        # Placeholder for future cleanup: close DB connections, flush trace buffers, etc.
        # 占位块，供未来清理逻辑使用：关闭数据库连接、刷新追踪缓冲区等。
        pass


# =============================================================================
# Phase 4: FastAPI application creation & middleware stack assembly
# Phase 4: FastAPI 应用创建与中间件栈装配
# =============================================================================

# Read auth setting once at import time to configure the app before it starts.
# When auth is enabled, disable /docs and /openapi.json to prevent unauthenticated
# callers from discovering the full API surface (attack surface reduction).
# 在导入时一次性读取认证配置，在应用启动前完成配置。
# 当认证启用时，禁用 /docs 和 /openapi.json，防止未认证调用方发现完整 API 表面
# （缩小攻击面）。
_auth_enabled = get_security_settings().auth_enabled
app = FastAPI(
    title="数盾 (PrivShield) 隐私治理边车",
    description="数联天下企业级数据隐私计算、多原语脱敏与三层动态分类分级治理服务 (PrivShield Privacy Governance Sidecar)",
    version="1.8.0",
    lifespan=lifespan,
    # docs_url=None disables Swagger UI; openapi_url=None disables the schema endpoint
    # docs_url=None 禁用 Swagger UI；openapi_url=None 禁用 schema 端点
    docs_url=None if _auth_enabled else "/docs",
    openapi_url=None if _auth_enabled else "/openapi.json",
)

# ---------------------------------------------------------------------------
# Middleware execution order (outermost -> innermost, i.e. request flow):
# ObservabilityMiddleware -> GZipMiddleware -> SecurityHeadersMiddleware -> router
#
# Note: Starlette add_middleware() prepends, so the LAST added runs FIRST on request.
# ---------------------------------------------------------------------------
# 中间件执行顺序（由外到内，即请求流向）：
# ObservabilityMiddleware -> GZipMiddleware -> SecurityHeadersMiddleware -> 路由处理
# 注意：Starlette add_middleware() 是前插的，因此最后添加的中间件在请求时最先执行。
# ---------------------------------------------------------------------------

# Security headers middleware — adds X-Content-Type-Options, X-Frame-Options, etc.
# Added first so it runs closest to the route handler (innermost layer).
# 安全响应头中间件 — 添加 X-Content-Type-Options、X-Frame-Options 等。
# 最先添加，使其最靠近路由处理函数（最内层）。
app.add_middleware(SecurityHeadersMiddleware)

# GZip response compression — reduces bandwidth for responses >= 1KB.
# minimum_size=1000 avoids compressing tiny responses where compression overhead
# would make them larger than the original.
# GZip 响应压缩 — 减少 >= 1KB 响应的网络传输开销。
# minimum_size=1000 避免压缩小型响应（压缩开销可能使压缩后反而更大）。
app.add_middleware(GZipMiddleware, minimum_size=1000)

# Observability middleware (outermost layer):
#   1. Extracts/generates request_id for distributed tracing correlation
#   2. Records structured access logs (method, path, status, duration)
#   3. Updates Prometheus counters/histograms (request count, latency, traffic)
#   Note: /metrics path is excluded internally to avoid self-referencing loops.
# 可观测性中间件（最外层）：
#   1. 提取/生成 request_id，用于分布式追踪关联
#   2. 记录结构化访问日志（方法、路径、状态码、耗时）
#   3. 更新 Prometheus 计数器/直方图（请求数、延迟、流量）
#   注意：/metrics 路径在内部被排除，避免自引用循环。
app.add_middleware(ObservabilityMiddleware)

# ---------------------------------------------------------------------------
# Mounted sub-applications (ASGI mount, bypasses FastAPI dependency injection)
# 挂载的子应用（ASGI 挂载，绕过 FastAPI 依赖注入体系）
# ---------------------------------------------------------------------------

# Prometheus metrics endpoint at /metrics.
# Since app.mount() bypasses FastAPI's dependency system, we wrap the metrics
# ASGI app with ApiKeyAuthAsgiMiddleware to enforce API Key authentication at
# the ASGI layer when PRIVACY_AUTH_ENABLED=true. The middleware also enforces
# the "ops:metrics" scope, preventing low-privilege keys from accessing metrics.
# When auth is disabled, the middleware passes through transparently.
# Prometheus 指标端点 /metrics。
# 由于 app.mount() 绕过 FastAPI 依赖体系，因此用 ApiKeyAuthAsgiMiddleware
# 包裹指标 ASGI 应用，在 ASGI 层强制 API Key 认证（PRIVACY_AUTH_ENABLED=true 时）。
# 该中间件同时强制检查 "ops:metrics" scope，防止低权限 Key 访问指标。
# 认证未启用时，中间件透明透传。
app.mount("/metrics", ApiKeyAuthAsgiMiddleware(make_asgi_app()))

# ---------------------------------------------------------------------------
# Route registration — include domain-specific routers
# 路由注册 — 挂载按域拆分的子路由
# ---------------------------------------------------------------------------

# Dynamic classification router: 3-layer funnel (Rule -> NER -> LLM)
# for field/table/image data classification and security tagging.
# 动态分类路由：三层漏斗（规则 -> NER -> LLM），
# 用于字段/表/图片数据分类分级与安全标记。
app.include_router(dynclassification.router)

# Privacy primitives & operational routers:
#   health    — K8s liveness/readiness probes (/v1/health, /v1/readyz)
#   mask      — Field-name-aware PII masking (/v1/privacy/mask, /v1/privacy/mask_record)
#   dp        — Differential privacy: Laplace/Gaussian count/sum/mean (/v1/privacy/dp/*)
#   ldp       — Local differential privacy (/v1/privacy/ldp/*)
#   kano      — K-anonymity: per-record & dataset-level (/v1/privacy/k_anonymize/*)
#   qol       — Query obfuscation: dummy query injection (/v1/privacy/qol/*)
#   budget    — Privacy budget accounting & tracking (/v1/privacy/budget)
#   profile   — Privacy parameter profile management (/v1/privacy/profile)
#   file      — File upload, parsing & redaction (/v1/privacy/file/*)
#   ops       — Operational diagnostics & engine status (/v1/ops/diagnostics)
#   medical   — Medical data processing pipeline (/v1/medical/*)
#   pipeline  — Combined multi-step classification + privacy pipeline (/v1/pipeline/*)
# 隐私原语与运维路由：
#   health    — K8s 存活/就绪探针
#   mask      — 字段名感知的 PII 脱敏
#   dp        — 差分隐私：Laplace/Gaussian 计数/求和/均值
#   ldp       — 本地差分隐私
#   kano      — K-匿名：记录级与数据集级
#   qol       — 查询混淆：虚拟查询注入
#   budget    — 隐私预算核算与追踪
#   profile   — 隐私参数 profile 管理
#   file      — 文件上传、解析与打码
#   ops       — 运维诊断与引擎状态
#   medical   — 医疗数据处理流水线
#   pipeline  — 多步分类 + 隐私处理组合流水线
app.include_router(health.router)
app.include_router(mask.router)
app.include_router(dp.router)
app.include_router(ldp.router)
app.include_router(kano.router)
app.include_router(qol.router)
app.include_router(budget.router)
app.include_router(profile.router)
app.include_router(file.router)
app.include_router(ops.router)
app.include_router(medical.router)
app.include_router(pipeline_router)


# =============================================================================
# Phase 5: Standalone entry point — ``python -m engine.main``
# Phase 5: 独立入口 — ``python -m engine.main``
# =============================================================================

# This block only executes when the file is run directly (not when imported).
# It parses CLI arguments and starts the Uvicorn ASGI server.
# Note: this entry point binds to 127.0.0.1 by default (local dev only).
# For production, use ``python -m engine.server`` which binds 0.0.0.0.
# 该代码块仅在直接运行文件时执行（导入时不执行）。
# 解析命令行参数并启动 Uvicorn ASGI 服务器。
# 注意：此入口默认绑定 127.0.0.1（仅本地开发）。
# 生产环境请使用 ``python -m engine.server``（默认绑定 0.0.0.0）。

if __name__ == "__main__":
    import argparse

    import uvicorn

    # CLI argument parser for standalone REST server mode
    # 独立 REST 服务器模式的命令行参数解析器
    parser = argparse.ArgumentParser(
        prog="engine.main",
        description="SecretFlow Local Privacy Agent REST server.",
    )
    # Bind to loopback only — safe default for local development
    # 仅绑定回环地址 — 本地开发的安全默认值
    parser.add_argument(
        "--host",
        default="127.0.0.1",
        help="REST server host (default: 127.0.0.1).",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8079,
        help="REST server port (default: 8079).",
    )
    # Parse CLI args and start the ASGI server with the configured app
    # 解析命令行参数，使用配置好的 app 启动 ASGI 服务器
    args = parser.parse_args()
    uvicorn.run(app, host=args.host, port=args.port)
